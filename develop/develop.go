package develop

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/hannibal/develop/fs"
	"github.com/jackc/hannibal/server"

	_ "github.com/jackc/hannibal/embed/statik"
)

type Config struct {
	ProjectPath   string
	ListenAddress string
}

func Develop(config *Config) {
	log := *current.Logger(context.Background())

	db.RequireCorrectVersion(context.Background())

	watcher, err := fs.NewWatcher()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start file system watcher")
	}

	sqlPath := filepath.Join(config.ProjectPath, "sql")
	err = watcher.Add(sqlPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to watch sql directory")
	}

	templatePath := filepath.Join(config.ProjectPath, "template")
	err = watcher.Add(templatePath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to watch template directory")
	}

	configPath := filepath.Join(config.ProjectPath, "config")
	err = watcher.Add(configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to watch config directory")
	}

	host := &server.Host{
		HTTPListenAddr: config.ListenAddress,
	}

	err = host.Load(context.Background(), config.ProjectPath)
	if err != nil {
		log.Error().Err(err).Msg("failed to load project")
	}

	log.Info().Str("addr", host.HTTPListenAddr).Msg("Starting HTTP server")

	interruptChan := make(chan os.Signal, 1)
	shutdownSignals := []os.Signal{os.Interrupt}
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
		os.Exit(0)
	}()

	go func() {
		err = host.ListenAndServe()
		if err != nil {
			log.Fatal().Err(err).Msg("could not start server")
		}
	}()
	// End HTTP Server

	for {
		select {
		case event := <-watcher.Events:
			log.Info().Str("name", event.Name).Str("op", event.Op.String()).Msg("file change detected")

			err := host.Load(context.Background(), config.ProjectPath)
			if err != nil {
				log.Error().Err(err).Msg("reload failed")
			} else {
				log.Info().Msg("reload succeeded")
			}
		case err := <-watcher.Errors:
			log.Fatal().Err(err).Msg("file system watcher error")
		}
	}
}
