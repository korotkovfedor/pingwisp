package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/korotkovfedor/pingwisp/internal"
)

func main() {
	listenAddr := flag.String("listen", "127.0.0.1:8080", "HTTP listen address (host:port)")
	flag.Parse()

	if err := internal.Bootstrap(*listenAddr); err != nil {
		slog.Error("Application exited with error", "error", err)
		os.Exit(1)
	}
}
