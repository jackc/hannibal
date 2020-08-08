package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/hannibal/appconf"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/pgtype"
)

var allowedInArgs = []string{
	"query_args",
}

var allowedOutArgs = []string{
	"status",
	"resp_body",
}

type PGFuncHandler struct {
	SQL         string
	FuncInArgs  []string
	QueryParams []appconf.RouteParams
}

func (h *PGFuncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var queryArgs map[string]interface{}
	if len(h.QueryParams) != 0 {
		queryArgs = make(map[string]interface{}, len(h.QueryParams))
		for _, qp := range h.QueryParams {
			queryArgs[qp.Name] = r.URL.Query().Get(qp.Name)
		}
	}

	sqlArgs := make([]interface{}, 0, len(h.FuncInArgs))
	for _, ia := range h.FuncInArgs {
		switch ia {
		case "query_args":
			sqlArgs = append(sqlArgs, queryArgs)
		}
	}

	var status pgtype.Int2
	var respBody []byte

	err := db.App(ctx).QueryRow(ctx, h.SQL, sqlArgs...).Scan(
		&status,
		&respBody,
	)
	if err != nil {
		panic(err)
	}

	w.Header().Add("Content-Type", "application/json")

	if status.Status == pgtype.Present {
		w.WriteHeader(int(status.Int))
	}

	if respBody != nil {
		w.Write(respBody)
	}
}

func NewPGFuncHandler(name string, proargmodes []string, proargnames []string) (*PGFuncHandler, error) {
	if name == "" {
		return nil, errors.New("name cannot be empty")
	}

	if len(proargmodes) == 0 {
		return nil, errors.New("proargmodes cannot be empty")
	}

	if len(proargnames) == 0 {
		return nil, errors.New("proargnames cannot be empty")
	}

	if len(proargmodes) != len(proargnames) {
		return nil, errors.New("proargmodes and proargnames are not the same length")
	}

	inArgMap := make(map[string]struct{})
	outArgMap := make(map[string]struct{})

	for i, m := range proargmodes {
		switch m {
		case "i":
			inArgMap[proargnames[i]] = struct{}{}
		case "o":
			outArgMap[proargnames[i]] = struct{}{}
		default:
			return nil, fmt.Errorf("unknown proargmode: %s", m)
		}
	}

	if _, hasStatus := outArgMap["status"]; !hasStatus {
		if _, hasRespBody := outArgMap["resp_body"]; !hasRespBody {
			return nil, errors.New("missing status and resp_body args")
		}
	}

	inArgs := make([]string, 0, len(inArgMap))
	// Allowed input arguments in order.
	for _, a := range allowedInArgs {
		if _, ok := inArgMap[a]; ok {
			inArgs = append(inArgs, a)
			delete(inArgMap, a)
		}
	}

	// inArgMap should be empty
	if len(inArgMap) > 0 {
		for k, _ := range inArgMap {
			return nil, fmt.Errorf("unknown arg: %s", k)
		}
	}

	sb := &strings.Builder{}

	sb.WriteString("select ")
	for i, arg := range allowedOutArgs {
		if i > 0 {
			sb.WriteString(", ")
		}
		if _, ok := outArgMap[arg]; ok {
			delete(outArgMap, arg)
			sb.WriteString(arg)
		} else {
			fmt.Fprintf(sb, "null as %s", arg)
		}
	}

	fmt.Fprintf(sb, " from %s(", name)
	for i, arg := range inArgs {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(sb, "%s => $%d", arg, i+1)
	}
	sb.WriteString(")")

	// outArgMap should be empty
	if len(outArgMap) > 0 {
		for k, _ := range outArgMap {
			return nil, fmt.Errorf("unknown arg: %s", k)
		}
	}

	h := &PGFuncHandler{
		SQL:        sb.String(),
		FuncInArgs: inArgs,
	}

	return h, nil
}
