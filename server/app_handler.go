package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/pgxutil"
)

func NewAppHandler(ctx context.Context) (http.Handler, error) {
	var handlers []Handler
	err := pgxutil.SelectAllStruct(context.Background(), db.Sys(ctx), &handlers,
		fmt.Sprintf("select * from %s.get_handlers()", db.QuoteSchema(db.GetConfig(ctx).SysSchema)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to read handlers: %v", err)
	}

	router := chi.NewRouter()
	for _, h := range handlers {
		fmt.Println(h)
		// TODO - params need to be parsed into something and used in PGJSONHandler.ServeHTTP
		jh := &PGJSONHandler{
			SQL:    h.SQL,
			Params: make([]PGJSONHandlerParam, len(h.Params)),
		}
		for i := range h.Params {
			jh.Params[i].Name = h.Params[i].Name
		}

		router.Method(h.Method, h.Pattern, jh)
	}

	return router, nil
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

type PGJSONHandlerParam struct {
	Name string
}

type PGJSONHandler struct {
	SQL    string
	Params []PGJSONHandlerParam
}

func (h *PGJSONHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	args := make([]interface{}, len(h.Params))
	for i := range h.Params {
		args[i] = r.URL.Query().Get(h.Params[i].Name)
	}

	buf, err := pgxutil.SelectByteSlice(ctx, db.App(ctx), h.SQL, args...)
	if err != nil {
		panic(err)
	}

	w.Header().Add("Content-Type", "application/json")

	w.Write(buf)
}
