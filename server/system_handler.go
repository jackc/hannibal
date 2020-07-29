package server

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi"
	"github.com/jackc/hannibal/system"
	"github.com/jackc/pgx/v4"
)

type SystemHandler struct {
	reloadMutex *sync.RWMutex

	router chi.Router
}

func NewSystemHandler(ctx context.Context, reloadMutex *sync.RWMutex) (*SystemHandler, error) {
	sh := &SystemHandler{
		reloadMutex: reloadMutex,
		router:      chi.NewRouter(),
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
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Forbidden"))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, err)
			w.Write([]byte("Internal server error"))
		}
		return
	}

	file, _, err := req.FormFile("pkg")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
		return
	}
	defer file.Close()

	hashDigest := sha512.New512_256()
	_, err = io.Copy(hashDigest, file)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
		return
	}
	pkgDigest := hashDigest.Sum(nil)

	expectedPkgDigest, err := hex.DecodeString(req.FormValue("digest"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
		return
	}

	if bytes.Compare(pkgDigest, expectedPkgDigest) != 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Incorrect package digest"))
		return
	}

	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
		return
	}

	signature, err := hex.DecodeString(req.FormValue("signature"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
		return
	}

	ok, err := system.ValidateDeployment(ctx, userID, pkgDigest, signature)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
		return
	}
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Incorrect package signature"))
		return
	}

	n, _ := ioutil.ReadAll(file)
	fmt.Fprintf(w, "Hey -- %d, I read %d bytes of a correctly digested and signed file", userID, len(n))
	// ah.router.ServeHTTP(w, req)
}
