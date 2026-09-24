package internal

import (
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/handlers"
)

func Bootstrap() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /ping", handlers.HandlePing)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	slog.Info("Serving at http://localhost:8080")
	log.Fatal(server.ListenAndServe())
}
