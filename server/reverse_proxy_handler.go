package server

import (
	"net/http"
	"net/http/httputil"
)

type reverseProxy struct {
	rp *httputil.ReverseProxy
}

func (rp *reverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rp.rp.ServeHTTP(w, r)
}
