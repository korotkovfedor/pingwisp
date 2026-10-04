package main

import (
	"log/slog"
	"os"

	"github.com/korotkovfedor/pingwisp/internal"
)

func main() {
	if err := internal.Bootstrap(); err != nil {
		slog.Error("Application exited with error", "error", err)
		os.Exit(1)
	}
}
