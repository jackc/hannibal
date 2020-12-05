package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/", HelloHandler)
	err := http.ListenAndServe(fmt.Sprintf(":%s", os.Args[1]), nil)
	if err != http.ErrServerClosed {
		log.Fatalf("could not start HTTP server: %v", err)
	}
}

func HelloHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello, world!"))
}
