package main

import (
	"net/http"
)

func main() {
	http.HandleFunc("/reverse_proxy/hello", HelloHandler)
	http.ListenAndServe(":3456", nil)
}

func HelloHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello via reverse proxy!"))
}
