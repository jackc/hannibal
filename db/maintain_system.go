package db

import (
	"context"
	"fmt"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/jackc/pgxutil"
	"github.com/jackc/tern/migrate"

	_ "github.com/jackc/hannibal/embed/statik"
	"github.com/rakyll/statik/fs"
)

// Init creates the initial database structure.
func InitDB(ctx context.Context) error {
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
		return fmt.Errorf("database already initialized: system schema %s already exists", dbconfig.SysSchema)
	}

	_, err = conn.Exec(ctx, fmt.Sprintf("create schema %s", dbconfig.SysSchema))
	if err != nil {
		return fmt.Errorf("failed to create schema %s: %w", dbconfig.SysSchema, err)
	}

	return upgradeDB(ctx, conn)
}

// UpgradeDB upgrades the system database structure.
func UpgradeDB(ctx context.Context) error {
	dbconfig := GetConfig(ctx)

	conn, err := pgx.Connect(ctx, dbconfig.SysConnString)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	return upgradeDB(ctx, conn)
}

type DBStatus struct {
	CurrentVersion int32
	DesiredVersion int32
}

func GetDBStatus(ctx context.Context) (*DBStatus, error) {
	dbconfig := GetConfig(ctx)

	conn, err := pgx.Connect(ctx, dbconfig.SysConnString)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	return getDBStatus(ctx, conn)
}

func getDBStatus(ctx context.Context, conn *pgx.Conn) (*DBStatus, error) {
	migrator, err := newSystemMigrator(ctx, conn)
	if err != nil {
		return nil, err
	}

	dbStatus := &DBStatus{
		DesiredVersion: int32(len(migrator.Migrations)),
	}

	dbStatus.CurrentVersion, err = migrator.GetCurrentVersion(ctx)
	if err != nil {
		return nil, err
	}

	return dbStatus, nil
}

func upgradeDB(ctx context.Context, conn *pgx.Conn) error {
	migrator, err := newSystemMigrator(ctx, conn)
	if err != nil {
		return err
	}

	err = migrator.Migrate(ctx)
	if err != nil {
		return err
	}

	return nil
}

func newSystemMigrator(ctx context.Context, conn *pgx.Conn) (*migrate.Migrator, error) {
	dbconfig := GetConfig(ctx)

	statikFS, err := fs.New()
	if err != nil {
		return nil, err
	}

	migrator, err := migrate.NewMigratorEx(ctx, conn, fmt.Sprintf("%s.schema_version", dbconfig.SysSchema), &migrate.MigratorOptions{MigratorFS: statikFS})
	if err != nil {
		return nil, err
	}
	migrator.OnStart = func(ctx context.Context, sequence int32, name string, sql string) {
		current.Logger(ctx).Info().Int32("sequence", sequence).Str("name", name).Msg("beginning migration")
	}
	migrator.Data = map[string]interface{}{
		"hannibalSchema": dbconfig.SysSchema,
	}
	err = migrator.LoadMigrations("/system_migrations")
	if err != nil {
		return nil, err
	}

	return migrator, nil
}

// RequireCorrectVersion checks that the database is at the correct version. If not it logs a fatal error and
// terminates the program.
func RequireCorrectVersion(ctx context.Context) {
	conn, err := Sys(ctx).(*pgxpool.Pool).Acquire(ctx)
	if err != nil {
		current.Logger(ctx).Fatal().Err(err).Msg("could not acquire database connection to check database status")
	}
	defer conn.Release()

	dbStatus, err := getDBStatus(ctx, conn.Conn())
	if err != nil {
		current.Logger(ctx).Fatal().Err(err).Msg("failed to check database status")
	}

	if dbStatus.CurrentVersion < dbStatus.DesiredVersion {
		current.Logger(ctx).Fatal().Int32("currentVersion", dbStatus.CurrentVersion).Int32("desiredVersion", dbStatus.DesiredVersion).Msg("database needs to be upgraded")
	} else if dbStatus.CurrentVersion > dbStatus.DesiredVersion {
		current.Logger(ctx).Fatal().Int32("currentVersion", dbStatus.CurrentVersion).Int32("desiredVersion", dbStatus.DesiredVersion).Msg("database has later version than hannibal")
	}
}

func QuoteSchema(s string) string {
	return pgx.Identifier{s}.Sanitize()
}
