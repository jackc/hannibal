package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/reverse_proxy/hello", HelloHandler)
	http.ListenAndServe(fmt.Sprintf(":%s", os.Args[1]), nil)
}

func HelloHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello via reverse proxy!"))
}
