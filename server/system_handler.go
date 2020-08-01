package server

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-chi/chi"
	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/hannibal/deploy"
	"github.com/jackc/hannibal/reload"
	"github.com/jackc/hannibal/system"
	"github.com/jackc/pgx/v4"
)

type SystemHandler struct {
	deployMutex sync.Mutex

	router       chi.Router
	reloadSystem *reload.System
	appPath      string
}

func NewSystemHandler(rs *reload.System, appPath string) (*SystemHandler, error) {
	sh := &SystemHandler{
		router:       chi.NewRouter(),
		reloadSystem: rs,
		appPath:      appPath,
	}

	sh.router.Post("/deploy", sh.deploy)

	return sh, nil
}

func (sh *SystemHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	sh.router.ServeHTTP(w, req)
}

func (sh *SystemHandler) deploy(w http.ResponseWriter, req *http.Request) {
	sh.deployMutex.Lock()
	defer sh.deployMutex.Unlock()

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

	nextPath := filepath.Join(sh.appPath, "next")
	currentPath := filepath.Join(sh.appPath, "current")
	previousPath := filepath.Join(sh.appPath, "previous")

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

	dbconfig := db.GetConfig(ctx)
	sqlPath := filepath.Join(nextPath, "sql")
	nextSchema := fmt.Sprintf("%s_next", dbconfig.AppSchema)
	err = db.InstallCodePackage(context.Background(), dbconfig.SysConnString, nextSchema, sqlPath)
	if err != nil {
		current.Logger(ctx).Error().Caller().Err(err).Send()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = sh.reloadSystem.Reload(ctx, func() error {
		_, err := db.Sys(ctx).Exec(ctx, fmt.Sprintf("drop schema if exists %s cascade", db.QuoteSchema(dbconfig.AppSchema)))
		if err != nil {
			return err
		}

		_, err = db.Sys(ctx).Exec(ctx, fmt.Sprintf("alter schema %s rename to %s", db.QuoteSchema(nextSchema), db.QuoteSchema(dbconfig.AppSchema)))
		if err != nil {
			return err
		}

		return nil
	})
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
