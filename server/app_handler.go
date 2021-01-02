package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi"
	"github.com/gorilla/csrf"
	"github.com/jackc/hannibal/appconf"
	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/hannibal/srvman"
)

func routeHasOnePath(r appconf.Route) bool {
	count := 0
	if r.GetPath != "" {
		count++
	}
	if r.PostPath != "" {
		count++
	}
	if r.PutPath != "" {
		count++
	}
	if r.PatchPath != "" {
		count++
	}
	if r.DeletePath != "" {
		count++
	}
	if r.Path != "" {
		count++
	}
	return count == 1
}

func routeHasOneHandler(r appconf.Route) bool {
	count := 0
	if r.Func != "" {
		count++
	}
	if r.ReverseProxy != "" {
		count++
	}
	return count == 1
}

func routeName(r appconf.Route) string {
	if r.GetPath != "" {
		return fmt.Sprintf("GET %s", r.GetPath)
	}
	if r.PostPath != "" {
		return fmt.Sprintf("POST %s", r.PostPath)
	}
	if r.PutPath != "" {
		return fmt.Sprintf("PUT %s", r.PutPath)
	}
	if r.PatchPath != "" {
		return fmt.Sprintf("PATCH %s", r.PatchPath)
	}
	if r.DeletePath != "" {
		return fmt.Sprintf("DELETE %s", r.DeletePath)
	}
	return r.Path
}

func NewAppHandler(ctx context.Context, dbconn db.DBConn, schema string, appConfig *appconf.Config, serviceGroup *srvman.Group, tmpl *template.Template, host *Host, publicPath string) (http.Handler, error) {
	csrfFunc, err := makeCSRFFunc(ctx, dbconn, schema, appConfig.CSRFProtection, tmpl, host)
	if err != nil {
		return nil, err
	}

	router := chi.NewRouter()
	for _, r := range appConfig.Routes {
		if !routeHasOnePath(r) {
			return nil, fmt.Errorf("route must have exactly one of path, get, post, put, patch, and delete")
		}

		if !routeHasOneHandler(r) {
			return nil, fmt.Errorf("route %s: must have exactly one of func and reverse-proxy", routeName(r))
		}

		var handler http.Handler

		if r.Func != "" {
			inArgs, outArgs, err := getSQLFuncArgs(ctx, dbconn, schema, r.Func)
			if err != nil {
				return nil, fmt.Errorf("route %s: %v", routeName(r), err)
			}

			pgFuncHandler, err := NewPGFuncHandler(r.Func, inArgs, outArgs)
			if err != nil {
				return nil, fmt.Errorf("route %s: failed to build handler for function %s: %v", routeName(r), r.Func, err)
			}

			pgFuncHandler.Params = make([]*RequestParam, len(r.Params))
			for i, qp := range r.Params {
				var err error
				pgFuncHandler.Params[i], err = requestParamFromAppConfig(qp)
				if err != nil {
					return nil, fmt.Errorf("route %s: failed to convert request param %s: %v", routeName(r), qp.Name, err)
				}
			}

			if r.DigestPassword != nil {
				pgFuncHandler.DigestPassword = &DigestPassword{
					PasswordParam: r.DigestPassword.PasswordParam,
					DigestParam:   r.DigestPassword.DigestParam,
				}
			}

			if r.CheckPasswordDigest != nil {
				inArgs, _, err := getSQLFuncArgs(ctx, dbconn, schema, r.CheckPasswordDigest.GetPasswordDigestFunc)
				if err != nil {
					return nil, fmt.Errorf("route %s: %v", routeName(r), err)
				}

				pgFuncHandler.CheckPasswordDigest, err = newCheckPasswordDigest(r.CheckPasswordDigest.GetPasswordDigestFunc, inArgs)
				pgFuncHandler.CheckPasswordDigest.PasswordParam = r.CheckPasswordDigest.PasswordParam
				pgFuncHandler.CheckPasswordDigest.ResultParam = r.CheckPasswordDigest.ResultParam
			}

			pgFuncHandler.RootTemplate = tmpl
			pgFuncHandler.Host = host
			handler = pgFuncHandler
		} else if r.ReverseProxy != "" {
			var httpAddress string
			if service := serviceGroup.GetService(r.ReverseProxy); service != nil {
				httpAddress = service.HTTPAddress
			} else {
				httpAddress = r.ReverseProxy
			}

			dstURL, err := url.Parse(httpAddress)
			if err != nil {
				return nil, fmt.Errorf("route %s: %v", routeName(r), err)
			}
			rp := &reverseProxy{
				rp: httputil.NewSingleHostReverseProxy(dstURL),
			}
			originalDirector := rp.rp.Director
			rp.rp.Director = func(r *http.Request) {
				if cookie, err := r.Cookie("hannibal-session"); err == nil {
					var requestCookieSession []byte
					host.secureCookie.Decode("hannibal-session", cookie.Value, &requestCookieSession)
					r.Header.Add("X-Hannibal-Cookie-Session", string(requestCookieSession))
				}
				originalDirector(r)
			}

			rp.rp.ModifyResponse = func(resp *http.Response) error {
				responseCookieSession := resp.Header.Get("X-Hannibal-Cookie-Session")
				if responseCookieSession != "" {
					resp.Header.Del("X-Hannibal-Cookie-Session")
					cookie := &http.Cookie{
						Name:     "hannibal-session",
						Path:     "/",
						Secure:   false, // TODO - false in dev mode -- configurable in production
						HttpOnly: true,
					}

					encoded, err := host.secureCookie.Encode("hannibal-session", []byte(responseCookieSession))
					if err != nil {
						panic(err)
					}
					cookie.Value = encoded

					resp.Header.Add("Set-Cookie", cookie.String())
				}

				return nil
			}

			handler = rp
		} else {
			panic("no handler config") // This should be unreachable due to routeHasOneHandler check above.
		}

		if csrfFunc != nil && !r.DisableCSRFProtection {
			handler = csrfFunc(handler)
		}

		if r.GetPath != "" {
			router.Method(http.MethodGet, r.GetPath, handler)
		} else if r.PostPath != "" {
			router.Method(http.MethodPost, r.PostPath, handler)
		} else if r.PutPath != "" {
			router.Method(http.MethodPut, r.PutPath, handler)
		} else if r.PatchPath != "" {
			router.Method(http.MethodPatch, r.PatchPath, handler)
		} else if r.DeletePath != "" {
			router.Method(http.MethodDelete, r.DeletePath, handler)
		} else {
			router.Handle(r.Path, handler)
		}
	}

	router.NotFound(NewPublicFileHandler(publicPath).ServeHTTP)

	return router, nil
}

func makeCSRFFunc(ctx context.Context, dbconn db.DBConn, schema string, csrfProtectionConfig *appconf.CSRFProtection, tmpl *template.Template, host *Host) (func(http.Handler) http.Handler, error) {
	if csrfProtectionConfig == nil {
		csrfProtectionConfig = &appconf.CSRFProtection{}
	}

	if csrfProtectionConfig.Disable {
		return nil, nil
	}

	options := []csrf.Option{}
	if csrfProtectionConfig.CookieName != nil {
		options = append(options, csrf.CookieName(*csrfProtectionConfig.CookieName))
	}
	if csrfProtectionConfig.Domain != nil {
		options = append(options, csrf.Domain(*csrfProtectionConfig.Domain))
	}
	if csrfProtectionConfig.ErrorFunc != nil {
		errorFunc := *csrfProtectionConfig.ErrorFunc
		inArgs, outArgs, err := getSQLFuncArgs(ctx, dbconn, schema, errorFunc)
		if err != nil {
			return nil, err
		}

		h, err := NewPGFuncHandler(errorFunc, inArgs, outArgs)
		if err != nil {
			return nil, fmt.Errorf("failed to build handler for function %s: %v", errorFunc, err)
		}

		h.RootTemplate = tmpl
		h.Host = host

		options = append(options, csrf.ErrorHandler(h))
	}
	if csrfProtectionConfig.FieldName != nil {
		options = append(options, csrf.FieldName(*csrfProtectionConfig.FieldName))
	}
	if csrfProtectionConfig.HTTPOnly != nil {
		options = append(options, csrf.HttpOnly(*csrfProtectionConfig.HTTPOnly))
	}
	if csrfProtectionConfig.MaxAge != nil {
		options = append(options, csrf.MaxAge(*csrfProtectionConfig.MaxAge))
	}
	if csrfProtectionConfig.Path != nil {
		options = append(options, csrf.Path(*csrfProtectionConfig.Path))
	} else {
		options = append(options, csrf.Path("/"))
	}
	if csrfProtectionConfig.RequestHeader != nil {
		options = append(options, csrf.RequestHeader(*csrfProtectionConfig.RequestHeader))
	}
	if csrfProtectionConfig.SameSite != nil {
		ssLowerStr := strings.ToLower(*csrfProtectionConfig.SameSite)
		var ssm csrf.SameSiteMode
		switch ssLowerStr {
		case "none":
			ssm = csrf.SameSiteNoneMode
		case "lax":
			ssm = csrf.SameSiteLaxMode
		case "strict":
			ssm = csrf.SameSiteStrictMode
		default:
			return nil, fmt.Errorf("bad csrf-protection.same-site value: %s", *csrfProtectionConfig.SameSite)
		}

		options = append(options, csrf.SameSite(ssm))
	}
	if csrfProtectionConfig.Secure != nil {
		options = append(options, csrf.Secure(*csrfProtectionConfig.Secure))
	}
	if len(csrfProtectionConfig.TrustedOrigins) > 0 {
		options = append(options, csrf.TrustedOrigins(csrfProtectionConfig.TrustedOrigins))
	}

	csrfKey := sha256.Sum256([]byte(current.SecretKeyBase(ctx) + "CSRF key"))
	csrfFunc := csrf.Protect(csrfKey[:], options...)

	return csrfFunc, nil
}

const (
	RequestParamTypeText = iota + 1
	RequestParamTypeInt
	RequestParamTypeBigint
	RequestParamTypeArray
	RequestParamTypeObject
	RequestParamTypeFile
)

type RequestParam struct {
	Name         string
	Type         int8
	ArrayElement *RequestParam
	ObjectFields []*RequestParam
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
	case "array":
		rp.Type = RequestParamTypeArray
	case "object":
		rp.Type = RequestParamTypeObject
	case "file":
		rp.Type = RequestParamTypeFile
	default:
		return nil, fmt.Errorf("param %s has unknown type: %s", acrp.Name, acrp.Type)
	}

	if acrp.TrimSpace == nil {
		rp.TrimSpace = true
	} else {
		rp.TrimSpace = *acrp.TrimSpace
	}

	if acrp.ArrayElement != nil {
		var err error
		rp.ArrayElement, err = requestParamFromAppConfig(acrp.ArrayElement)
		if err != nil {
			return nil, err
		}
	}

	return rp, nil
}

type arrayElementError struct {
	Index int
	Err   error
}

type arrayElementErrors []arrayElementError

func (e arrayElementErrors) Error() string {
	sb := &strings.Builder{}
	for i, ee := range e {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(sb, "Element %d: %v", ee.Index, ee.Err)
	}
	return sb.String()
}

type objectErrors map[string]error

func (e objectErrors) Error() string {
	sb := &strings.Builder{}
	for k, v := range e {
		if sb.Len() > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(sb, "%s: %v", k, v)
	}
	return sb.String()
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
		if jn, ok := value.(json.Number); ok {
			value = string(jn)
		}

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
		if jn, ok := value.(json.Number); ok {
			value = string(jn)
		}

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
				return nil, fmt.Errorf("%s: cannot convert %v to bigint", rp.Name, value)
			}
		default:
			return nil, fmt.Errorf("%s: cannot convert %v to bigint", rp.Name, value)
		}
		return num, nil

	case RequestParamTypeArray:
		switch value := value.(type) {
		case []interface{}:
			if rp.ArrayElement != nil {
				parsedArray := make([]interface{}, len(value))
				var errors arrayElementErrors
				for i := range parsedArray {
					var err error
					parsedArray[i], err = rp.ArrayElement.Parse(value[i])
					if err != nil {
						errors = append(errors, arrayElementError{Index: i, Err: err})
					}
				}
				if errors != nil {
					return nil, errors
				}
				return parsedArray, nil
			} else {
				return value, nil
			}
		default:
			return nil, fmt.Errorf("%s: cannot convert %v to array", rp.Name, value)
		}

	case RequestParamTypeObject:
		switch value := value.(type) {
		case map[string]interface{}:
			if rp.ObjectFields != nil {
				parsedObject := make(map[string]interface{}, len(rp.ObjectFields))
				var errors objectErrors
				for _, f := range rp.ObjectFields {
					var err error
					parsedObject[f.Name], err = f.Parse(value[f.Name])
					if err != nil {
						if errors == nil {
							errors = make(objectErrors)
						}
						errors[f.Name] = err
					}
				}
				if errors != nil {
					return nil, errors
				}
				return parsedObject, nil
			} else {
				return value, nil
			}
		default:
			return nil, fmt.Errorf("%s: cannot convert %v to object", rp.Name, value)
		}

	case RequestParamTypeFile:
		if _, ok := value.(*uploadedFile); ok {
			return value, nil
		}
		return nil, fmt.Errorf("%s: %v is not a file", rp.Name, value)

	default:
		return nil, fmt.Errorf("unknown param type %v", rp.Type)
	}
}

type DigestPassword struct {
	PasswordParam string
	DigestParam   string
}

type CheckPasswordDigest struct {
	PasswordParam string
	ResultParam   string
	SQL           string
	FuncInArgs    []string
}

func newCheckPasswordDigest(name string, inArgMap map[string]struct{}) (*CheckPasswordDigest, error) {
	if name == "" {
		return nil, errors.New("name cannot be empty")
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

	fmt.Fprintf(sb, "select %s(", name)
	for i, arg := range inArgs {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(sb, "%s => $%d", arg, i+1)
	}
	sb.WriteString(")")

	cpd := &CheckPasswordDigest{
		SQL:        sb.String(),
		FuncInArgs: inArgs,
	}

	return cpd, nil
}

func getSQLFuncArgs(ctx context.Context, dbconn db.DBConn, schema string, name string) (inArgs map[string]struct{}, outArgs map[string]struct{}, err error) {
	var proargmodes []string
	var proargnames []string

	err = dbconn.QueryRow(
		ctx,
		"select proargmodes::text[], proargnames from pg_proc where proname = $1 and pronamespace = ($2::text)::regnamespace",
		name,
		schema,
	).Scan(&proargmodes, &proargnames)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to introspect function %s: %v", name, err)
	}

	inArgs = make(map[string]struct{})
	outArgs = make(map[string]struct{})

	for i, n := range proargnames {
		var mode string
		if len(proargmodes) <= i {
			mode = "i"
		} else {
			mode = proargmodes[i]
		}

		switch mode {
		case "i":
			inArgs[n] = struct{}{}
		case "o":
			outArgs[n] = struct{}{}
		case "b":
			inArgs[n] = struct{}{}
			outArgs[n] = struct{}{}
		default:
			return nil, nil, fmt.Errorf("unknown proargmode: %s", n)
		}
	}

	return inArgs, outArgs, nil
}
