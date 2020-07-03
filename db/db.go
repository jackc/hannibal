package db

import (
	"context"
	"fmt"

	"github.com/jackc/foobarbuilder/current"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgxutil"
)

const foobarbuilderSchema = "foobarbuilder"

func MaintainSystem(ctx context.Context, connConfig *pgx.ConnConfig) error {

	conn, err := pgx.ConnectConfig(ctx, connConfig)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	systemSchemaExists, err := pgxutil.SelectBool(ctx, conn, "select exists(select 1 from pg_catalog.pg_namespace where nspname = $1)", foobarbuilderSchema)
	if err != nil {
		return err
	}

	if systemSchemaExists {
		current.Logger(ctx).Debug().Str("schema", foobarbuilderSchema).Msg("schema already exists")
	} else {
		_, err = conn.Exec(ctx, fmt.Sprintf("create schema %s", foobarbuilderSchema))
		if err != nil {
			return fmt.Errorf("failed to create schema %s: %w", foobarbuilderSchema, err)
		}
		current.Logger(ctx).Info().Str("schema", foobarbuilderSchema).Msg("created schema")
	}

	// TODO - migrations embedded in binary and automatically applied

	return nil
}
