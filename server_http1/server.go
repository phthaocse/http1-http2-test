package main

import (
	"fmt"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, HTTP/1.1!")
}

func main() {
	http.HandleFunc("/", handler)
	fmt.Println("Starting HTTP/1.1 server with TLS on :8080")
	if err := http.ListenAndServeTLS(":8080", "server.crt", "server.key", nil); err != nil {
		fmt.Println("Error starting HTTP/1.1 server:", err)
	}
}
