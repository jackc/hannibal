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
	reloadMutex *sync.RWMutex

	router chi.Router
}

func NewAppHandler(ctx context.Context, reloadMutex *sync.RWMutex) (*AppHandler, error) {
	ah := &AppHandler{reloadMutex: reloadMutex}

	return ah, nil
}

func (ah *AppHandler) Load(ctx context.Context) error {
	ah.reloadMutex.Lock()
	defer ah.reloadMutex.Unlock()

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
