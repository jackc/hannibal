package db

import (
	"context"
	"fmt"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/tern/migrate"

	_ "github.com/jackc/hannibal/embed/statik"
)

// MigrateDB migrates the application.
func MigrateDB(ctx context.Context, migrationPath string) error {
	dbconfig := GetConfig(ctx)

	conn, err := pgx.Connect(ctx, dbconfig.SysConnString)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	return migrateDB(ctx, conn, migrationPath)
}

func migrateDB(ctx context.Context, conn *pgx.Conn, migrationPath string) error {
	migrator, err := newAppMigrator(ctx, conn, migrationPath)
	if err != nil {
		return err
	}

	err = migrator.Migrate(ctx)
	if err != nil {
		return err
	}

	return nil
}

func newAppMigrator(ctx context.Context, conn *pgx.Conn, migrationPath string) (*migrate.Migrator, error) {
	dbconfig := GetConfig(ctx)

	migrator, err := migrate.NewMigrator(ctx, conn, fmt.Sprintf("%s.app_schema_version", QuoteSchema(dbconfig.SysSchema)))
	if err != nil {
		return nil, err
	}

	migrator.OnStart = func(ctx context.Context, sequence int32, name string, sql string) {
		current.Logger(ctx).Info().Int32("sequence", sequence).Str("name", name).Msg("beginning migration")
	}
	migrator.Data = map[string]interface{}{
		"hannibalSchema": dbconfig.SysSchema,
	}
	err = migrator.LoadMigrations(migrationPath)
	if err != nil {
		return nil, err
	}

	return migrator, nil
}
