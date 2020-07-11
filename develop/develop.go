package develop

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/jackc/foobarbuilder/current"
	"github.com/jackc/foobarbuilder/db"
	"github.com/jackc/foobarbuilder/develop/fs"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/jackc/tern/migrate"
	"github.com/rs/zerolog"

	_ "github.com/jackc/foobarbuilder/embed/statik"
	statikfs "github.com/rakyll/statik/fs"
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

	// install sql code on startup
	err = installSQL(sqlPath, dbconfig.ConnConfig)
	if err != nil {
		log.Error().Err(err).Msg("failed to install sql")
	} else {
		log.Info().Msg("updated sql")
	}

	for {
		select {
		case event := <-watcher.Events:
			log.Info().Str("name", event.Name).Str("op", event.Op.String()).Msg("file change detected")
			err := installSQL(sqlPath, dbconfig.ConnConfig)
			if err != nil {
				log.Error().Err(err).Msg("failed to install sql")
			} else {
				log.Info().Msg("updated sql")
			}
		case err := <-watcher.Errors:
			log.Fatal().Err(err).Msg("file system watcher error")
		}
	}
}

func installSQL(sqlPath string, connConfig *pgx.ConnConfig) error {
	cps, err := migrate.LoadCodePackageSource(sqlPath)
	if err != nil {
		return err
	}

	statikFS, err := statikfs.New()
	if err != nil {
		return err
	}

	file, err := statikFS.Open("/app_setup.sql")
	if err != nil {
		return err
	}

	buf, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	appSetupName := "app_setup.sql"

	cps.Schema = "app"
	cps.Manifest = append([]string{appSetupName}, cps.Manifest...)
	cps.SourceCode[appSetupName] = string(buf)

	conn, err := pgx.ConnectConfig(context.Background(), connConfig)
	if err != nil {
		return err
	}

	codePackage, err := cps.Compile()
	if err != nil {
		return err
	}

	err = codePackage.Install(context.Background(), conn, map[string]interface{}{})
	if err != nil {
		return err
	}

	return nil
}
