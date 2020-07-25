package db

import (
	"context"
	"fmt"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgxutil"
	"github.com/jackc/tern/migrate"

	_ "github.com/jackc/hannibal/embed/statik"
	"github.com/rakyll/statik/fs"
)

func MaintainSystem(ctx context.Context) error {
	dbconfig := GetConfig(ctx)

	conn, err := pgx.Connect(ctx, dbconfig.SysConnString)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, fmt.Sprintf("set search_path = %s", QuoteSchema(dbconfig.SysSchema)))
	if err != nil {
		return fmt.Errorf("failed to set search_path: %v", err)
	}

	systemSchemaExists, err := pgxutil.SelectBool(ctx, conn, "select exists(select 1 from pg_catalog.pg_namespace where nspname = $1)", dbconfig.SysSchema)
	if err != nil {
		return err
	}

	if systemSchemaExists {
		current.Logger(ctx).Debug().Str("schema", dbconfig.SysSchema).Msg("schema already exists")
	} else {
		_, err = conn.Exec(ctx, fmt.Sprintf("create schema %s", dbconfig.SysSchema))
		if err != nil {
			return fmt.Errorf("failed to create schema %s: %w", dbconfig.SysSchema, err)
		}
		current.Logger(ctx).Info().Str("schema", dbconfig.SysSchema).Msg("created schema")
	}

	statikFS, err := fs.New()
	if err != nil {
		return err
	}

	migrator, err := migrate.NewMigratorEx(ctx, conn, fmt.Sprintf("%s.schema_version", dbconfig.SysSchema), &migrate.MigratorOptions{MigratorFS: statikFS})
	if err != nil {
		return err
	}
	migrator.OnStart = func(ctx context.Context, sequence int32, name string, sql string) {
		current.Logger(ctx).Info().Int32("sequence", sequence).Str("name", name).Msg("beginning migration")
	}
	migrator.Data = map[string]interface{}{
		"hannibalSchema": dbconfig.SysSchema,
	}
	err = migrator.LoadMigrations("/system_migrations")
	if err != nil {
		return err
	}

	err = migrator.Migrate(ctx)
	if err != nil {
		return err
	}

	return nil
}

func QuoteSchema(s string) string {
	return pgx.Identifier{s}.Sanitize()
}
