package server

import "net/http"

type reverseProxy struct {
}

func (*reverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello, world"))
}
