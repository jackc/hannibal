package db

import (
	"context"
	"fmt"

	"github.com/jackc/foobarbuilder/current"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgxutil"
	"github.com/jackc/tern/migrate"

	_ "github.com/jackc/foobarbuilder/embed/statik"
	"github.com/rakyll/statik/fs"
)

const FoobarbuilderSchema = "foobarbuilder"

func MaintainSystem(ctx context.Context, connConfig *pgx.ConnConfig) error {

	conn, err := pgx.ConnectConfig(ctx, connConfig)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	systemSchemaExists, err := pgxutil.SelectBool(ctx, conn, "select exists(select 1 from pg_catalog.pg_namespace where nspname = $1)", FoobarbuilderSchema)
	if err != nil {
		return err
	}

	if systemSchemaExists {
		current.Logger(ctx).Debug().Str("schema", FoobarbuilderSchema).Msg("schema already exists")
	} else {
		_, err = conn.Exec(ctx, fmt.Sprintf("create schema %s", FoobarbuilderSchema))
		if err != nil {
			return fmt.Errorf("failed to create schema %s: %w", FoobarbuilderSchema, err)
		}
		current.Logger(ctx).Info().Str("schema", FoobarbuilderSchema).Msg("created schema")
	}

	statikFS, err := fs.New()
	if err != nil {
		return err
	}

	migrator, err := migrate.NewMigratorEx(ctx, conn, fmt.Sprintf("%s.schema_version", FoobarbuilderSchema), &migrate.MigratorOptions{MigratorFS: statikFS})
	if err != nil {
		return err
	}
	migrator.OnStart = func(ctx context.Context, sequence int32, name string, sql string) {
		current.Logger(ctx).Info().Int32("sequence", sequence).Str("name", name).Msg("beginning migration")
	}
	migrator.Data = map[string]interface{}{
		"foobarbuilderSchema": FoobarbuilderSchema,
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
