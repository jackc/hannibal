package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/hannibal/db"
	"github.com/jackc/pgtype"
)

var allowedInArgs = []string{
	"args",
	"cookie_session",
}

var allowedOutArgs = []string{
	"status",
	"resp_body",
	"template",
	"template_data",
	"cookie_session",
}

type PGFuncHandler struct {
	SQL          string
	FuncInArgs   []string
	Params       []*RequestParam
	RootTemplate *template.Template
	Host         *Host
}

func extractRawArgs(r *http.Request) (map[string]interface{}, error) {
	rawArgs := make(map[string]interface{})
	for key, values := range r.URL.Query() {
		rawArgs[key] = values[0]
	}

	if r.Header.Get("Content-Type") == "application/json" {
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		err := decoder.Decode(&rawArgs)
		if err != nil {
			return nil, err
		}
	}

	return rawArgs, nil
}

func (h *PGFuncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rawArgs, err := extractRawArgs(r)
	if err != nil {
		panic(err)
	}

	errors := make(map[string]string)

	var queryArgs map[string]interface{}
	if len(h.Params) != 0 {
		queryArgs = make(map[string]interface{}, len(h.Params))
		for _, qp := range h.Params {
			if value, err := qp.Parse(rawArgs[qp.Name]); err == nil {
				queryArgs[qp.Name] = value
			} else {
				errors[qp.Name] = err.Error()
			}
		}
	}

	if len(errors) != 0 {
		response, err := json.Marshal(map[string]interface{}{"errors": errors})
		if err != nil {
			panic(err) // cannot happen
		}

		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write(response)
		return
	}

	// Read the cookie session. Ignore any errors and treat as missing.
	var requestCookieSession []byte
	if cookie, err := r.Cookie("hannibal-session"); err == nil {
		h.Host.secureCookie.Decode("hannibal-session", cookie.Value, &requestCookieSession)
	}

	sqlArgs := make([]interface{}, 0, len(h.FuncInArgs))
	for _, ia := range h.FuncInArgs {
		switch ia {
		case "args":
			sqlArgs = append(sqlArgs, queryArgs)
		case "cookie_session":
			sqlArgs = append(sqlArgs, requestCookieSession)
		}
	}

	var status pgtype.Int2
	var respBody []byte
	var templateName pgtype.Text
	var templateData map[string]interface{}
	var responseCookieSession []byte

	err = db.App(ctx).QueryRow(ctx, h.SQL, sqlArgs...).Scan(
		&status,
		&respBody,
		&templateName,
		&templateData,
		&responseCookieSession,
	)
	if err != nil {
		panic(err)
	}

	if status.Status == pgtype.Present {
		w.WriteHeader(int(status.Int))
	}

	if respBody != nil {
		w.Header().Add("Content-Type", "application/json")
		w.Write(respBody)
		return
	}

	if responseCookieSession != nil {
		encoded, err := h.Host.secureCookie.Encode("hannibal-session", responseCookieSession)
		if err != nil {
			panic(err)
		}

		cookie := &http.Cookie{
			Name:     "hannibal-session",
			Value:    encoded,
			Path:     "/",
			Secure:   false, // TODO - false in dev mode -- configurable in production
			HttpOnly: true,
		}
		http.SetCookie(w, cookie)

	} else if responseCookieSession != nil {
		cookie := &http.Cookie{
			Name:     "hannibal-session",
			Value:    "",
			Path:     "/",
			Secure:   false, // TODO - false in dev mode -- configurable in production
			HttpOnly: true,
			Expires:  time.Unix(0, 0),
		}
		http.SetCookie(w, cookie)
	}

	if templateName.Status == pgtype.Present {
		w.Header().Add("Content-Type", "text/html")
		tmpl := h.RootTemplate.Lookup(templateName.String)
		if tmpl == nil {
			panic("template not found: " + templateName.String)
		}
		err := tmpl.Execute(w, templateData)
		if err != nil {
			panic(err)
		}
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
		case "b":
			inArgMap[proargnames[i]] = struct{}{}
			outArgMap[proargnames[i]] = struct{}{}
		default:
			return nil, fmt.Errorf("unknown proargmode: %s", m)
		}
	}

	if _, hasStatus := outArgMap["status"]; !hasStatus {
		if _, hasRespBody := outArgMap["resp_body"]; !hasRespBody {
			if _, hasTemplate := outArgMap["template"]; !hasTemplate {
				return nil, errors.New("missing status, resp_body, and template out arguments")
			}
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
