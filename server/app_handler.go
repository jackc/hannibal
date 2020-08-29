package server

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi"
	"github.com/jackc/hannibal/appconf"
	"github.com/jackc/hannibal/db"
)

func NewAppHandler(ctx context.Context, dbconn db.DBConn, schema string, routes []appconf.Route, tmpl *template.Template, host *Host, publicPath string) (http.Handler, error) {
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

		h.Params = make([]*RequestParam, len(r.Params))
		for i, qp := range r.Params {
			var err error
			h.Params[i], err = requestParamFromAppConfig(qp)
			if err != nil {
				return nil, fmt.Errorf("failed to convert request param %s: %v", qp.Name, err)
			}
		}

		h.RootTemplate = tmpl
		h.Host = host

		if r.Method != "" {
			router.Method(r.Method, r.Path, h)
		} else {
			router.Handle(r.Path, h)
		}
	}

	router.NotFound(NewPublicFileHandler(publicPath).ServeHTTP)

	return router, nil
}

const (
	RequestParamTypeText = iota + 1
	RequestParamTypeInt
	RequestParamTypeBigint
)

type RequestParam struct {
	Name         string
	Type         int8
	TrimSpace    bool
	Required     bool
	NullifyEmpty bool
}

func requestParamFromAppConfig(acrp *appconf.RequestParam) (*RequestParam, error) {
	rp := &RequestParam{
		Name:         acrp.Name,
		Required:     acrp.Required,
		NullifyEmpty: acrp.NullifyEmpty,
	}

	switch acrp.Type {
	case "text", "varchar", "":
		rp.Type = RequestParamTypeText
	case "int", "int4", "integer":
		rp.Type = RequestParamTypeInt
	case "bigint", "int8":
		rp.Type = RequestParamTypeBigint
	default:
		return nil, fmt.Errorf("param %s has unknown type: %s", acrp.Name, acrp.Type)
	}

	if acrp.TrimSpace == nil {
		rp.TrimSpace = true
	} else {
		rp.TrimSpace = *acrp.TrimSpace
	}

	return rp, nil
}

func (rp *RequestParam) Parse(value interface{}) (interface{}, error) {
	if rp.TrimSpace {
		if s, ok := value.(string); ok {
			value = strings.TrimSpace(s)
		}
	}

	if rp.NullifyEmpty {
		if value == "" {
			value = nil
		}
	}

	if value == nil {
		if rp.Required {
			return nil, errors.New("missing")
		}
		return nil, nil
	}

	switch rp.Type {
	case RequestParamTypeText:
		var s string
		switch value := value.(type) {
		case string:
			s = value
		default:
			s = fmt.Sprint(value)
		}
		return s, nil
	case RequestParamTypeInt:
		var num int32
		switch value := value.(type) {
		case string:
			var err error
			n, err := strconv.ParseInt(value, 10, 32)
			if err != nil {
				if errors.Is(err, strconv.ErrSyntax) {
					return nil, errors.New("not a number")
				} else if errors.Is(err, strconv.ErrRange) {
					return nil, errors.New("out of range")
				} else {
					return nil, err
				}
			}
			num = int32(n)
		case float64:
			num = int32(value)
			if float64(num) != value {
				return nil, fmt.Errorf("%s: cannot convert %v to int", rp.Name, value)
			}
		default:
			return nil, fmt.Errorf("%s: cannot convert %v to int", rp.Name, value)
		}
		return num, nil
	case RequestParamTypeBigint:
		var num int64
		switch value := value.(type) {
		case string:
			var err error
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				if errors.Is(err, strconv.ErrSyntax) {
					return nil, errors.New("not a number")
				} else if errors.Is(err, strconv.ErrRange) {
					return nil, errors.New("out of range")
				} else {
					return nil, err
				}
			}
			num = int64(n)
		case float64:
			num = int64(value)
			if float64(num) != value {
				return nil, fmt.Errorf("%s: cannot convert %v to int", rp.Name, value)
			}
		default:
			return nil, fmt.Errorf("%s: cannot convert %v to int", rp.Name, value)
		}
		return num, nil
	default:
		return nil, fmt.Errorf("unknown param type %v", rp.Type)
	}
}
