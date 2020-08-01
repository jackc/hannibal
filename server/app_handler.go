package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/go-chi/chi"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/pgxutil"
)

type AppHandler struct {
	reloadMutex sync.RWMutex
	router      chi.Router
}

func NewAppHandler() *AppHandler {
	return &AppHandler{router: chi.NewRouter()}
}

func (ah *AppHandler) LockForReload(ctx context.Context) error {
	ah.reloadMutex.Lock()
	return nil
}

func (ah *AppHandler) UnlockAndReload(ctx context.Context) error {
	defer ah.reloadMutex.Unlock()
	return ah.load(ctx)
}

// load loads the routes. It requires the reloadMutex already be locked.
func (ah *AppHandler) load(ctx context.Context) error {
	var handlers []Handler
	err := pgxutil.SelectAllStruct(context.Background(), db.Sys(ctx), &handlers,
		fmt.Sprintf("select * from %s.get_handlers()", db.QuoteSchema(db.GetConfig(ctx).SysSchema)),
	)
	if err != nil {
		return fmt.Errorf("failed to read handlers: %v", err)
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

	ah.router = router

	return nil
}

func (ah *AppHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ah.reloadMutex.RLock()
	defer ah.reloadMutex.RUnlock()

	ah.router.ServeHTTP(w, req)
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
