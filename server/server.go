package server

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/hannibal/reload"
)

var shutdownSignals = []os.Signal{os.Interrupt}

type Config struct {
	ListenAddress string
	AppPath       string
}

func Serve(config *Config) {
	db.RequireCorrectVersion(context.Background())

	log := *current.Logger(context.Background())

	r := BaseMux(log)

	reloadSystem := &reload.System{}

	appHandler := NewAppHandler()
	reloadSystem.Register(appHandler)

	r.Mount("/", appHandler)

	systemHandler, err := NewSystemHandler(reloadSystem, config.AppPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create system handler")
	}
	r.Mount("/hannibal-system", systemHandler)

	err = reloadSystem.Reload(context.Background(), func() error { return nil })
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load app")
	}

	server := &http.Server{
		Addr:    config.ListenAddress,
		Handler: r,
	}
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, shutdownSignals...)
	go func() {
		s := <-interruptChan
		signal.Reset() // Only listen for one interrupt. If another interrupt signal is received allow it to terminate the program.
		log.Info().Str("signal", s.String()).Msg("shutdown signal received")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		server.SetKeepAlivesEnabled(false)
		err := server.Shutdown(ctx)
		if err != nil {
			log.Error().Err(err).Msg("graceful HTTP server shutdown failed")
		}
	}()

	err = server.ListenAndServe()
	if err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("could not start HTTP server")
	}

}
