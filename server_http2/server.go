package main

import (
	"fmt"
	"golang.org/x/net/http2"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, HTTP/2!")
}

func main() {
	server := &http.Server{
		Addr:    ":8443",
		Handler: http.HandlerFunc(handler),
	}

	http2.ConfigureServer(server, &http2.Server{})

	fmt.Println("Starting HTTP/2 server on :8443")
	if err := server.ListenAndServeTLS("server.crt", "server.key"); err != nil {
		fmt.Println("Error starting HTTP/2 server:", err)
	}
}
