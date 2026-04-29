package main

import (
	"cmp"
	"log"
	"net/http"
	"os"
	"time"
)

// here we are going to implement the main function for the server
func main() {
	s := NewStore()
	mux := NewMux(s)

	addr := ":" + cmp.Or(os.Getenv("PORT"), "3000")
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}
