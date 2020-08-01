package develop

import (
	"context"
	"net/http"
	"path/filepath"
	"sync"

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
	dbconfig := db.GetConfig(context.Background())

	watcher, err := fs.NewWatcher()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start file system watcher")
	}

	sqlPath := filepath.Join(config.ProjectPath, "sql")
	err = watcher.Add(sqlPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to watch sql directory")
	}

	// install sql code on startup
	err = db.InstallCodePackage(context.Background(), dbconfig.SysConnString, dbconfig.AppSchema, sqlPath)
	if err != nil {
		log.Error().Err(err).Msg("failed to install sql")
	} else {
		log.Info().Msg("updated sql")
	}

	// HTTP Server
	r := server.BaseMux(log)

	reloadMutex := &sync.RWMutex{}

	appHandler, err := server.NewAppHandler(context.Background(), reloadMutex)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create app handler")
	}

	err = appHandler.Load(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load app handler")
	}

	r.Mount("/", appHandler)

	server := &http.Server{
		Addr:    config.ListenAddress,
		Handler: r,
	}

	go func() {
		err = server.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("could not start HTTP server")
		}
	}()
	// End HTTP Server

	for {
		select {
		case event := <-watcher.Events:
			log.Info().Str("name", event.Name).Str("op", event.Op.String()).Msg("file change detected")
			err := db.InstallCodePackage(context.Background(), dbconfig.SysConnString, dbconfig.AppSchema, sqlPath)
			if err != nil {
				log.Error().Err(err).Msg("failed to install sql")
			} else {
				log.Info().Msg("updated sql")
			}

			err = appHandler.Load(context.Background())
			if err != nil {
				log.Error().Err(err).Msg("failed to reload app handler")
			} else {
				log.Info().Msg("reloaded app handler")
			}
		case err := <-watcher.Errors:
			log.Fatal().Err(err).Msg("file system watcher error")
		}
	}
}
