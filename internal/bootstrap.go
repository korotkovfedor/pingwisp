package internal

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/checker"
	"github.com/korotkovfedor/pingwisp/internal/handlers"
	"github.com/korotkovfedor/pingwisp/internal/poller"
)

func Bootstrap() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpChecker := checker.NewHTTP(&http.Client{})
	poller := poller.New(httpChecker)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           handlers.NewRouter(poller),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	pollerDone := make(chan struct{})
	go func() {
		defer close(pollerDone)
		poller.Run(ctx)
	}()

	shutdownDone := make(chan error, 1)
	go func() {
		<-ctx.Done()
		slog.Info("Shutting down...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		shutdownErr := server.Shutdown(shutdownCtx)
		shutdownCancel()
		if shutdownErr != nil {
			shutdownErr = errors.Join(shutdownErr, server.Close())
		}

		<-pollerDone
		shutdownDone <- shutdownErr
	}()

	slog.Info("Serving at http://localhost:8080")
	serveErr := server.ListenAndServe()
	stop()
	shutdownErr := <-shutdownDone

	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	if err := errors.Join(serveErr, shutdownErr); err != nil {
		return err
	}

	slog.Info("Server exited")
	return nil
}
