package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/pgtype"
	pgtypeuuid "github.com/jackc/pgtype/ext/gofrs-uuid"
	shopspring "github.com/jackc/pgtype/ext/shopspring-numeric"
	"github.com/jackc/pgtype/pgxtype"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/jackc/pgxutil"
	"github.com/rs/zerolog"
)

var shutdownSignals = []os.Signal{os.Interrupt}

type Config struct {
	ListenAddress        string
	DatabaseURL          string
	DatabaseSystemSchema string
	DatabaseAppSchema    string
}

func Serve(config *Config) {
	log := zerolog.New(os.Stdout).With().
		Timestamp().
		Logger()
	current.SetLogger(&log)

	dbconfig, err := pgxpool.ParseConfig(config.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to parse database connection string")
	}
	dbconfig.AfterConnect = AfterConnect(config.DatabaseSystemSchema, config.DatabaseAppSchema)

	dbpool, err := pgxpool.ConnectConfig(context.Background(), dbconfig)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}

	r := BaseMux(log)

	appHandler, err := NewAppHandler(dbpool, config.DatabaseSystemSchema)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create app handler")
	}

	err = appHandler.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load app handler")
	}

	r.Mount("/", appHandler)

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

func AfterConnect(systemSchema, appSchema string) func(context.Context, *pgx.Conn) error {
	return func(ctx context.Context, conn *pgx.Conn) error {
		searchPath, err := pgxutil.SelectString(ctx, conn, "show search_path")
		if err != nil {
			return fmt.Errorf("failed to get search_path: %v", err)
		}

		searchPath = fmt.Sprintf("%s, %s, %s", appSchema, searchPath, systemSchema)
		_, err = conn.Exec(ctx, fmt.Sprintf("set search_path = %s", db.QuoteSchema(searchPath)))
		if err != nil {
			return fmt.Errorf("failed to set search_path: %v", err)
		}

		err = registerDataTypes(ctx, conn, systemSchema)
		if err != nil {
			return fmt.Errorf("failed to register data types: %v", err)
		}

		return nil
	}
}

func registerDataTypes(ctx context.Context, conn *pgx.Conn, systemSchema string) error {
	conn.ConnInfo().RegisterDataType(pgtype.DataType{
		Value: &pgtypeuuid.UUID{},
		Name:  "uuid",
		OID:   pgtype.UUIDOID,
	})
	conn.ConnInfo().RegisterDataType(pgtype.DataType{
		Value: &shopspring.Numeric{},
		Name:  "numeric",
		OID:   pgtype.NumericOID,
	})

	dataTypeNames := []string{
		"handler_param",
		"_handler_param",
		"handler",
		"_handler",
		"get_handler_result_row_param",
		"_get_handler_result_row_param",
		"get_handler_result_row",
		"_get_handler_result_row",
	}

	for _, typeName := range dataTypeNames {
		dataType, err := pgxtype.LoadDataType(ctx, conn, conn.ConnInfo(), fmt.Sprintf("%s.%s", db.QuoteSchema(systemSchema), typeName))
		if err != nil {
			return err
		}
		conn.ConnInfo().RegisterDataType(dataType)
	}

	return nil
}
