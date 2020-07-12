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
	"github.com/jackc/pgxutil"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

var shutdownSignals = []os.Signal{os.Interrupt}

type Config struct {
	ListenAddress string
	DatabaseURL   string
}

type HandlerParam struct {
	Name        string
	ParamTypeID string
	Required    bool
}

type Handler struct {
	Method  string
	Pattern string
	SQL     string
	Params  []HandlerParam
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

	var handlers []Handler
	err = pgxutil.SelectAllStruct(context.Background(), dbpool, &handlers, fmt.Sprintf("select * from %s.get_handlers()", db.HannibalSchema))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to read handlers")
	}

	for _, h := range handlers {
		fmt.Println(h)
		// TODO - params need to be parsed into something and used in PGJSONHandler.ServeHTTP
		jh := &PGJSONHandler{
			DB:     dbpool,
			SQL:    h.SQL,
			Params: make([]PGJSONHandlerParam, len(h.Params)),
		}
		for i := range h.Params {
			jh.Params[i].Name = h.Params[i].Name
		}

		fmt.Println(jh)
		r.Method(h.Method, h.Pattern, jh)
	}

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

type PGJSONHandlerParam struct {
	Name string
}

type PGJSONHandler struct {
	DB     *pgxpool.Pool
	SQL    string
	Params []PGJSONHandlerParam
}

func (h *PGJSONHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	args := make([]interface{}, len(h.Params))
	for i := range h.Params {
		args[i] = r.URL.Query().Get(h.Params[i].Name)
	}

	buf, err := pgxutil.SelectByteSlice(r.Context(), h.DB, h.SQL, args...)
	if err != nil {
		panic(err)
	}

	w.Header().Add("Content-Type", "application/json")

	w.Write(buf)
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
