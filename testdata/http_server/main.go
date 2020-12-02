package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/reverse_proxy/hello", HelloHandler)
	http.HandleFunc("/reverse_proxy/cookie_session", CookieSessionHandler)
	http.ListenAndServe(fmt.Sprintf(":%s", os.Args[1]), nil)
}

func HelloHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello via reverse proxy!"))
}

func CookieSessionHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Write([]byte(r.Header.Get("X-Hannibal-Cookie-Session")))
		return
	case http.MethodPost:
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "failed to read request body: %v", err)
			return
		}
		w.Header().Set("X-Hannibal-Cookie-Session", string(body))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}
