package server

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi"
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

	fmt.Fprintln(w, "it got here")

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
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	pkg, _, err := req.FormFile("pkg")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			http.Error(w, "Missing pkg file upload", http.StatusBadRequest)
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}
	defer pkg.Close()

	signature, err := hex.DecodeString(req.FormValue("signature"))
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	publicKeys, err := system.GetDeployPublicKeysForUserID(ctx, userID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = deploy.Unpack(pkg, signature, sh.appPath, publicKeys)
	if err != nil {
		if errors.Is(err, deploy.ErrInvalidSignature) {
			http.Error(w, "Invalid signature", http.StatusBadRequest)
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}
}
