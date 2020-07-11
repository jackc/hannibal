package develop

import (
	"context"
	"os"
	"path/filepath"

	"github.com/jackc/foobarbuilder/current"
	"github.com/jackc/foobarbuilder/db"
	"github.com/jackc/foobarbuilder/develop/fs"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
)

type Config struct {
	ProjectPath   string
	ListenAddress string
	DatabaseURL   string
}

func Develop(config *Config) {
	log := zerolog.New(os.Stdout).With().
		Timestamp().
		Logger()
	current.SetLogger(&log)

	dbconfig, err := pgxpool.ParseConfig(config.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to parse database connection string")
	}

	err = db.MaintainSystem(context.Background(), dbconfig.ConnConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to maintain system")
	}

	watcher, err := fs.NewWatcher()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start file system watcher")
	}

	sqlPath := filepath.Join(config.ProjectPath, "sql")
	err = watcher.Add(sqlPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to watch sql directory")
	}

	for {
		select {
		case event := <-watcher.Events:
			log.Info().Str("name", event.Name).Str("op", event.Op.String()).Msg("file change detected")
		case err := <-watcher.Errors:
			log.Fatal().Err(err).Msg("file system watcher error")
		}
	}
}
