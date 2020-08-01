package develop

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/hannibal/develop/fs"
	"github.com/jackc/hannibal/reload"
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

	reloadSystem := &reload.System{}

	appHandler := server.NewAppHandler()
	reloadSystem.Register(appHandler)

	err = reloadSystem.Reload(context.Background(), func() error { return nil })
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load app handler")
	}

	r.Mount("/", appHandler)

	server := &http.Server{
		Addr:    config.ListenAddress,
		Handler: r,
	}

	log.Info().Str("addr", server.Addr).Msg("Starting HTTP server")

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

			err := reloadSystem.Reload(context.Background(), func() error {
				err := db.InstallCodePackage(context.Background(), dbconfig.SysConnString, dbconfig.AppSchema, sqlPath)
				if err != nil {
					log.Error().Err(err).Msg("failed to install sql")
				} else {
					log.Info().Msg("updated sql")
				}
				return err
			})
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
