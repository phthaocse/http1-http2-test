package main

import (
	"fmt"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, HTTP/2 without TLS!")
}

func main() {
	handler := http.HandlerFunc(handler)

	server := &http.Server{
		Addr:    ":8181",
		Handler: h2c.NewHandler(handler, &http2.Server{}),
	}

	fmt.Println("Starting HTTP/2 server without TLS on :8181")
	if err := server.ListenAndServe(); err != nil {
		fmt.Println("Error starting HTTP/2 server:", err)
	}
}
