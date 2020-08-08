package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/jackc/hannibal/appconf"
	"github.com/jackc/hannibal/db"
)

func NewAppHandler(ctx context.Context, dbconn db.DBConn, schema string, routes []appconf.Route) (http.Handler, error) {
	router := chi.NewRouter()
	for _, r := range routes {
		var proargmodes []string
		var proargnames []string

		err := dbconn.QueryRow(
			ctx,
			"select proargmodes::text[], proargnames from pg_proc where proname = $1 and pronamespace = ($2::text)::regnamespace",
			r.Func,
			schema,
		).Scan(&proargmodes, &proargnames)
		if err != nil {
			return nil, fmt.Errorf("failed to introspect function %s: %v", r.Func, err)
		}

		h, err := NewPGFuncHandler(r.Func, proargmodes, proargnames)
		if err != nil {
			return nil, fmt.Errorf("failed to build handler for function %s: %v", r.Func, err)
		}

		h.QueryParams = r.QueryParams

		if r.Method != "" {
			router.Method(r.Method, r.Path, h)
		} else {
			router.Handle(r.Path, h)
		}
	}

	return router, nil
}
