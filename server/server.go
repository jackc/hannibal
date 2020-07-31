package server

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
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

	r := BaseMux(log)

	reloadMutex := &sync.RWMutex{}

	appHandler, err := NewAppHandler(context.Background(), reloadMutex)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create app handler")
	}

	err = appHandler.Load(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load app handler")
	}

	r.Mount("/", appHandler)

	systemHandler, err := NewSystemHandler(context.Background(), reloadMutex, config.AppPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create system handler")
	}
	r.Mount("/hannibal-system", systemHandler)

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
