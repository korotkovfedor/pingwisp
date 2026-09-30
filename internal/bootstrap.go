package internal

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/checker"
	"github.com/korotkovfedor/pingwisp/internal/handlers"
	"github.com/korotkovfedor/pingwisp/internal/poller"
)

func Bootstrap() {
	ctx := context.Background()

	httpChecker := checker.NewHTTP(&http.Client{})
	poller := poller.New(httpChecker)
	go poller.Run(ctx)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           handlers.NewRouter(poller),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	slog.Info("Serving at http://localhost:8080")
	log.Fatal(server.ListenAndServe())
}
