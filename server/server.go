package server

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
)

var shutdownSignals = []os.Signal{os.Interrupt}

type Config struct {
	ListenAddress string
	AppPath       string
}

func Serve(config *Config) {
	db.RequireCorrectVersion(context.Background())
	log := *current.Logger(context.Background())

	host := &Host{
		HTTPListenAddr: config.ListenAddress,
		AppPath:        config.AppPath,
	}

	err := host.Load(context.Background(), filepath.Join(host.AppPath, "current"))
	if err != nil {
		log.Error().Err(err).Msg("unable to load app")
	}

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, shutdownSignals...)
	go func() {
		s := <-interruptChan
		signal.Reset() // Only listen for one interrupt. If another interrupt signal is received allow it to terminate the program.
		log.Info().Str("signal", s.String()).Msg("shutdown signal received")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		err := host.Shutdown(ctx)
		if err != nil {
			log.Error().Err(err).Msg("graceful shutdown failed")
		}
	}()

	log.Info().Str("addr", host.HTTPListenAddr).Msg("Starting HTTP server")

	err = host.ListenAndServe()
	if err != nil {
		log.Fatal().Err(err).Msg("unable to start")
	}
}
