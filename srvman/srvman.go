// Package srvman handles service management.
package srvman

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

// Color represents the color of a blue / green deploy system
type Color int

const (
	_ Color = iota
	ColorBlue
	ColorGreen
)

func (c Color) String() string {
	switch c {
	case ColorBlue:
		return "blue"
	case ColorGreen:
		return "green"
	default:
		return fmt.Sprintf("invalid Color: %d", c)
	}
}

// Next returns the next color. e.g. if c is ColorBlue then Next will return ColorGreen and vice-versa. If c is the
// zero value Next will return ColorBlue.
func (c Color) Next() Color {
	if c == ColorBlue {
		return ColorGreen
	}
	return ColorBlue
}

var bluegreenRegexp = regexp.MustCompile(`\[\[bluegreen\.[a-zA-Z0-9_-]+\]\]`)

func interpolateColorVariables(s string, args map[string]interface{}) (string, error) {
	var err error
	s = bluegreenRegexp.ReplaceAllStringFunc(s, func(match string) string {
		key := match[12 : len(match)-2]

		if args == nil {
			err = fmt.Errorf("missing key: %s", key)
			return ""
		}

		value, ok := args[key]
		if !ok {
			err = fmt.Errorf("missing key: %s", key)
			return ""
		}

		return fmt.Sprint(value)
	})
	if err != nil {
		return "", err
	}

	return s, nil
}

type ServiceConfig struct {
	Name        string
	Cmd         string
	Args        []string
	HTTPAddress string

	HealthCheck *HealthCheck

	// MaxStartupDuration is how long to wait for the first health check to succeed before considering the service to
	// have failed to start.
	MaxStartupDuration time.Duration

	Blue  map[string]interface{}
	Green map[string]interface{}

	Logger *zerolog.Logger
}

// HealthCheck checks that a service is healthy.
type HealthCheck struct {
	TCPConnect string
}

// Check runs the health check.
func (hc *HealthCheck) Check(ctx context.Context) error {
	if hc.TCPConnect != "" {
		err := hc.checkTCPConnect(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (hc *HealthCheck) checkTCPConnect(ctx context.Context) error {
	dialer := &net.Dialer{}
	for {
		conn, err := dialer.DialContext(ctx, "tcp", hc.TCPConnect)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		if ctx.Err() != nil {
			return err
		}

		time.Sleep(50 * time.Millisecond)
	}
}

// Service is a manager for a process. It is managed by a Group and should not be directly used.
type Service struct {
	config *ServiceConfig

	name        string
	cmd         *exec.Cmd
	HTTPAddress string
	healthCheck *HealthCheck

	started bool

	stoppedChan chan struct{}
	cleanupChan chan struct{}

	startupChan chan error
	waitChan    chan error

	logger *zerolog.Logger
}

// start starts the service. It must only be called once.
func (s *Service) start() error {
	if s.started {
		return fmt.Errorf("start has already been called")
	}
	s.started = true

	err := s.cmd.Start()
	if err != nil {
		return err
	}

	if s.config.Logger != nil {
		logger := s.config.Logger.With().Str("service", s.config.Name).Int("pid", s.cmd.Process.Pid).Logger()
		s.logger = &logger
	} else {
		s.logger = zerolog.Ctx(context.Background()) // get a disabled logger
	}

	s.logger.Info().Msg("started process")

	go func() { s.waitChan <- s.cmd.Wait() }()
	go s.waitForStartup()

	select {
	case err := <-s.waitChan:
		s.logger.Error().Err(err).Int("exitCode", s.cmd.ProcessState.ExitCode()).Msg("process immediately exited")
		if err != nil {
			return err
		}
		return fmt.Errorf("process immediately exited with exit code 0")
	case err := <-s.startupChan:
		if err == nil {
			s.logger.Info().Msg("service ready")
		} else {
			s.logger.Error().Err(err).Msg("startup health check failed")
			killErr := s.cmd.Process.Kill()
			if killErr != nil {
				s.logger.Error().Err(err).Msg("kill process failed")
			}
			return err
		}
	}

	go s.monitor()

	return nil
}

// stop stops the service and waits until the process has ended.
func (s *Service) stop() error {
	close(s.stoppedChan)
	<-s.cleanupChan
	return nil
}

func (s *Service) waitForStartup() {
	if s.healthCheck == nil {
		s.startupChan <- nil
		return
	}

	maxStartupDuration := s.config.MaxStartupDuration
	if maxStartupDuration == 0 {
		maxStartupDuration = time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), maxStartupDuration)
	defer cancel()

	s.startupChan <- s.healthCheck.Check(ctx)
}

func (s *Service) monitor() {
	for {
		select {
		// Process has unexpectedly died.
		case err := <-s.waitChan:
			_ = err
		// restart ... but how to handle stop while restarting

		// TODO - health check timer
		// case ...

		// stop() has been called.
		case <-s.stoppedChan:
			s.logger.Info().Msg("sending kill signal to process")
			killErr := s.cmd.Process.Kill()
			if killErr != nil {
				s.logger.Error().Err(killErr).Msg("kill process failed")
			}
			s.logger.Info().Msg("waiting for process to terminate")
			<-s.waitChan
			s.logger.Info().Msg("process terminated")
			close(s.cleanupChan)
			return
		}
	}
}

// Group represents a managed group of services.
type Group struct {
	ServiceConfigs []*ServiceConfig

	color    Color
	services []*Service
}

// Start starts all services from g.ServiceConfigs. If any fail to start it will terminate any that did successfully
// start and return an error. Start must only be called once for any group. Group must not be modified after Start is
// called.
func (g *Group) Start(ctx context.Context, color Color) error {
	g.color = color

	// Create but do not start services.
	g.services = make([]*Service, 0, len(g.ServiceConfigs))
	for _, sc := range g.ServiceConfigs {
		s, err := newService(sc, color)
		if err != nil {
			return fmt.Errorf("bad service config for %s: %v ", sc.Name, err)
		}
		g.services = append(g.services, s)
	}

	var errgrp errgroup.Group
	for _, s := range g.services {
		errgrp.Go(s.start)
	}

	err := errgrp.Wait()
	if err != nil {
		// If an error occurred while starting any of the services, shutdown any processes that did successfully start.
		for _, s := range g.services {
			if s.cmd.Process != nil {
				s.cmd.Process.Kill()
			}
		}
		return err
	}

	return nil
}

func (g *Group) Stop(ctx context.Context) error {
	errChan := make(chan error)
	for _, s := range g.services {
		go func(s *Service) {
			err := s.stop()
			errChan <- err
		}(s)
	}

	var firstErr error
	for range g.services {
		err := <-errChan
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return nil
}

// GetService gets the service by name. It will return nil if no service by that name exists.
func (g *Group) GetService(name string) *Service {
	for _, s := range g.services {
		if s.name == name {
			return s
		}
	}

	return nil
}

// newService makes a new service from sc and color, but does not start it.
func newService(sc *ServiceConfig, color Color) (*Service, error) {
	s := &Service{
		config: sc,
		name:   sc.Name,
	}

	var colorConf map[string]interface{}
	switch color {
	case ColorBlue:
		colorConf = sc.Blue
	case ColorGreen:
		colorConf = sc.Green
	default:
		return nil, fmt.Errorf("unknown color: %v", color)
	}

	cmdPath, err := interpolateColorVariables(sc.Cmd, colorConf)
	if err != nil {
		return nil, err
	}
	cmdArgs := make([]string, len(sc.Args))
	for i := range sc.Args {
		arg, err := interpolateColorVariables(sc.Args[i], colorConf)
		if err != nil {
			return nil, err
		}
		cmdArgs[i] = arg
	}

	s.cmd = exec.Command(cmdPath, cmdArgs...)
	s.cmd.Stdout = os.Stdout
	s.cmd.Stderr = os.Stderr

	s.HTTPAddress, err = interpolateColorVariables(sc.HTTPAddress, colorConf)
	if err != nil {
		return nil, err
	}

	if sc.HealthCheck != nil {
		hc := &HealthCheck{}
		hc.TCPConnect, err = interpolateColorVariables(sc.HealthCheck.TCPConnect, colorConf)
		if err != nil {
			return nil, err
		}

		s.healthCheck = hc
	}

	if sc.Logger != nil {
		logger := sc.Logger.With().Str("service", sc.Name).Logger()
		s.logger = &logger
	} else {
		s.logger = zerolog.Ctx(context.Background()) // get a disabled logger
	}

	s.stoppedChan = make(chan struct{})
	s.cleanupChan = make(chan struct{})
	s.startupChan = make(chan error, 1)
	s.waitChan = make(chan error, 1)

	return s, nil
}
