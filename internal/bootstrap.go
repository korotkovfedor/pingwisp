package internal

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/handlers"
	"github.com/korotkovfedor/pingwisp/internal/poller"
)

func Bootstrap() {
	ctx := context.Background()

	poller := poller.New()
	go poller.Run(ctx)

	mux := http.NewServeMux()

	mux.Handle("POST /targets", handlers.NewCreateTarget(poller))
	mux.Handle("GET /targets", handlers.NewGetTargets(poller))
	mux.Handle("GET /targets/{id}", handlers.NewGetTarget(poller))
	mux.Handle("DELETE /targets/{id}", handlers.NewDeleteTarget(poller))

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	slog.Info("Serving at http://localhost:8080")
	log.Fatal(server.ListenAndServe())
}
