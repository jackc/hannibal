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
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Authorization header is missing or incorrectly formatted"))
		return
	}

	userID, err := system.AuthenticateUserByAPIKeyString(ctx, authorizationHeaderParts[1])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Authorization header contains incorrect API key"))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal server error"))
		}
		return
	}

	pkg, _, err := req.FormFile("pkg")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Missing pkg file upload"))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal server error"))
		}
		return
	}
	defer pkg.Close()

	signature, err := hex.DecodeString(req.FormValue("signature"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
		return
	}

	publicKeys, err := system.GetDeployPublicKeysForUserID(ctx, userID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
		return
	}

	err = deploy.Unpack(pkg, signature, sh.appPath, publicKeys)
	if err != nil {
		if errors.Is(err, deploy.ErrInvalidSignature) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid signature"))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal server error"))
		}
		return
	}
}
