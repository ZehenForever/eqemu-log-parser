package main

import (
	"flag"
	"log"
	"net/http"
	"time"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8787", "listen address")
	flag.Parse()

	srv := NewServer()

	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("eqloghub listening on http://%s", *listen)
	log.Fatal(httpServer.ListenAndServe())
}
