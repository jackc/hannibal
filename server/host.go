package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gorilla/securecookie"
	"github.com/jackc/hannibal/appconf"
	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/hannibal/deploy"
	"github.com/jackc/hannibal/system"
	"github.com/jackc/pgx/v4"
)

type Host struct {
	HTTPListenAddr string
	AppPath        string

	httpServer   *http.Server
	deployMutex  sync.Mutex
	installMutex sync.RWMutex
	appHandler   http.Handler

	secureCookie *securecookie.SecureCookie
}

func (h *Host) ListenAndServe() error {
	log := *current.Logger(context.Background())

	cookieHashKey := sha256.Sum256([]byte(current.SecretKeyBase(context.Background()) + "cookie hash key"))
	cookieBlockKey := sha256.Sum256([]byte(current.SecretKeyBase(context.Background()) + "cookie block key"))
	h.secureCookie = securecookie.New(cookieHashKey[:], cookieBlockKey[:16])

	r := BaseMux(log)

	if h.appHandler == nil {
		h.appHandler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`No project loaded`))
		})
	}

	r.Mount("/", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		h.installMutex.RLock()
		defer h.installMutex.RUnlock()

		h.appHandler.ServeHTTP(w, req)
	}))

	if h.AppPath != "" {
		r.Post("/hannibal-system/deploy", h.handleDeploy)
	}

	h.httpServer = &http.Server{
		Addr:    h.HTTPListenAddr,
		Handler: r,
	}

	err := h.httpServer.ListenAndServe()
	if err != http.ErrServerClosed {
		return fmt.Errorf("could not start HTTP server: %v", err)
	}

	return nil
}

func (h *Host) Shutdown(ctx context.Context) error {
	h.httpServer.SetKeepAlivesEnabled(false)
	err := h.httpServer.Shutdown(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (h *Host) Load(ctx context.Context, projectPath string) error {
	h.installMutex.Lock()
	defer h.installMutex.Unlock()

	h.appHandler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`Failed to load project`))
	})

	configPath := filepath.Join(projectPath, "config")
	appConfig, err := appconf.Load(configPath)
	if err != nil {
		return err
	}

	dbconfig := db.GetConfig(ctx)
	sqlPath := filepath.Join(projectPath, "sql")

	err = db.InstallCodePackage(ctx, dbconfig.SysConnString, dbconfig.AppSchema, sqlPath)
	if err != nil {
		return err
	}

	rootTmpl, err := loadTemplates(filepath.Join(projectPath, "template"))
	if err != nil {
		return err
	}

	newAppHandler, err := NewAppHandler(ctx, db.App(ctx), dbconfig.AppSchema, appConfig, rootTmpl, h, filepath.Join(projectPath, "public"))
	if err != nil {
		return err
	}

	h.appHandler = newAppHandler
	return nil
}

func (h *Host) handleDeploy(w http.ResponseWriter, req *http.Request) {
	h.deployMutex.Lock()
	defer h.deployMutex.Unlock()

	ctx := req.Context()

	authorizationHeaderParts := strings.SplitN(req.Header.Get("Authorization"), " ", 2)
	if len(authorizationHeaderParts) != 2 || authorizationHeaderParts[0] != "hannibal" {
		http.Error(w, "Authorization header is missing or incorrectly formatted", http.StatusForbidden)
		return
	}

	userID, err := system.AuthenticateUserByAPIKeyString(ctx, authorizationHeaderParts[1])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Authorization header contains incorrect API key", http.StatusForbidden)
		} else {
			current.Logger(ctx).Error().Caller().Err(err).Send()
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	pkg, _, err := req.FormFile("pkg")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			http.Error(w, "Missing pkg file upload", http.StatusBadRequest)
		} else {
			current.Logger(ctx).Error().Caller().Err(err).Send()
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}
	defer pkg.Close()

	signature, err := hex.DecodeString(req.FormValue("signature"))
	if err != nil {
		current.Logger(ctx).Error().Caller().Err(err).Send()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	publicKeys, err := system.GetDeployPublicKeysForUserID(ctx, userID)
	if err != nil {
		current.Logger(ctx).Error().Caller().Err(err).Send()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	nextPath := filepath.Join(h.AppPath, "next")
	currentPath := filepath.Join(h.AppPath, "current")
	previousPath := filepath.Join(h.AppPath, "previous")

	err = os.RemoveAll(nextPath)
	if err != nil {
		current.Logger(ctx).Error().Caller().Err(err).Send()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = os.Mkdir(nextPath, 0777)
	if err != nil {
		current.Logger(ctx).Error().Caller().Err(err).Send()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = deploy.Unpack(pkg, signature, nextPath, publicKeys)
	if err != nil {
		if errors.Is(err, deploy.ErrInvalidSignature) {
			http.Error(w, "Invalid signature", http.StatusBadRequest)
		} else {
			current.Logger(ctx).Error().Caller().Err(err).Send()
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	configPath := filepath.Join(nextPath, "config")
	appConfig, err := appconf.Load(configPath)
	if err != nil {
		current.Logger(ctx).Error().Caller().Err(err).Send()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	dbconfig := db.GetConfig(ctx)
	sqlPath := filepath.Join(nextPath, "sql")
	nextSchema := fmt.Sprintf("%s_next", dbconfig.AppSchema)
	err = db.InstallCodePackage(ctx, dbconfig.SysConnString, nextSchema, sqlPath)
	if err != nil {
		current.Logger(ctx).Error().Caller().Err(err).Send()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	rootTmpl, err := loadTemplates(filepath.Join(nextPath, "template"))
	if err != nil {
		current.Logger(ctx).Error().Caller().Err(err).Send()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	newAppHandler, err := NewAppHandler(ctx, db.App(ctx), nextSchema, appConfig, rootTmpl, h, filepath.Join(currentPath, "public"))
	if err != nil {
		current.Logger(ctx).Error().Caller().Err(err).Send()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.installMutex.Lock()
	defer h.installMutex.Unlock()

	h.appHandler = newAppHandler

	_, err = db.Sys(ctx).Exec(ctx, fmt.Sprintf("drop schema if exists %s cascade", db.QuoteSchema(dbconfig.AppSchema)))
	if err != nil {
		current.Logger(ctx).Error().Caller().Err(err).Send()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	_, err = db.Sys(ctx).Exec(ctx, fmt.Sprintf("alter schema %s rename to %s", db.QuoteSchema(nextSchema), db.QuoteSchema(dbconfig.AppSchema)))
	if err != nil {
		current.Logger(ctx).Error().Caller().Err(err).Send()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = os.RemoveAll(previousPath)
	if err != nil {
		current.Logger(ctx).Error().Caller().Err(err).Send()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = os.Rename(currentPath, previousPath)
	if err != nil {
		// On the first deploy currentPath will not exist.
		if !errors.Is(err, os.ErrNotExist) {
			current.Logger(ctx).Error().Caller().Err(err).Send()
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	err = os.Rename(nextPath, currentPath)
	if err != nil {
		current.Logger(ctx).Error().Caller().Err(err).Send()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	current.Logger(ctx).Info().Msg("Successful deploy")
}

func loadTemplates(rootPath string) (*template.Template, error) {
	rootTmpl := template.New("root")

	walkFunc := func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("failed to walk for %s: %v", path, walkErr)
		}

		if info.Mode().IsRegular() {
			tmplSrc, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}

			tmplName := path[len(rootPath)+1:]
			_, err = rootTmpl.New(tmplName).Parse(string(tmplSrc))
			if err != nil {
				return fmt.Errorf("failed to parse for %s: %v", path, err)
			}
		}

		return nil
	}

	err := filepath.Walk(rootPath, walkFunc)
	if err != nil {
		return nil, err
	}

	return rootTmpl, nil
}
