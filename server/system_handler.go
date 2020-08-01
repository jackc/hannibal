package server

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-chi/chi"
	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/deploy"
	"github.com/jackc/hannibal/system"
	"github.com/jackc/pgx/v4"
)

type SystemHandler struct {
	reloadMutex *sync.RWMutex

	router  chi.Router
	appPath string
}

func NewSystemHandler(ctx context.Context, reloadMutex *sync.RWMutex, appPath string) (*SystemHandler, error) {
	sh := &SystemHandler{
		reloadMutex: reloadMutex,
		router:      chi.NewRouter(),
		appPath:     appPath,
	}

	sh.router.Post("/deploy", sh.deploy)

	return sh, nil
}

func (sh *SystemHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	sh.reloadMutex.RLock()
	defer sh.reloadMutex.RUnlock()

	sh.router.ServeHTTP(w, req)
}

func (sh *SystemHandler) deploy(w http.ResponseWriter, req *http.Request) {
	sh.reloadMutex.RUnlock()     // Cannot get the write lock when we already have the read lock.
	defer sh.reloadMutex.RLock() // Reacquire read lock so ServeHTTP can successfully unlock.

	sh.reloadMutex.Lock()
	defer sh.reloadMutex.Unlock()

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

	// TODO - deploy database code -- then once successfully deployed

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
}
