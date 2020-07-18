package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/pgtype"
	pgtypeuuid "github.com/jackc/pgtype/ext/gofrs-uuid"
	shopspring "github.com/jackc/pgtype/ext/shopspring-numeric"
	"github.com/jackc/pgtype/pgxtype"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

var shutdownSignals = []os.Signal{os.Interrupt}

type Config struct {
	ListenAddress string
	DatabaseURL   string
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
	dbconfig.AfterConnect = RegisterDataTypes

	dbpool, err := pgxpool.ConnectConfig(context.Background(), dbconfig)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)

	r.Use(hlog.NewHandler(log))
	r.Use(hlog.RequestIDHandler("request_id", "x-request-id"))
	r.Use(hlog.MethodHandler("method"))
	r.Use(hlog.URLHandler("url"))
	r.Use(hlog.RemoteAddrHandler("remote_ip"))
	r.Use(hlog.AccessHandler(func(r *http.Request, status, size int, duration time.Duration) {
		hlog.FromRequest(r).Info().
			Int("status", status).
			Int("size", size).
			Dur("duration", duration).
			Msg("HTTP request")
	}))

	r.Use(middleware.Recoverer)

	appHandler, err := NewAppHandler(dbpool)
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

func RegisterDataTypes(ctx context.Context, conn *pgx.Conn) error {
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
		dataType, err := pgxtype.LoadDataType(ctx, conn, conn.ConnInfo(), fmt.Sprintf("%s.%s", db.HannibalSchema, typeName))
		if err != nil {
			return err
		}
		conn.ConnInfo().RegisterDataType(dataType)
	}

	return nil
}
