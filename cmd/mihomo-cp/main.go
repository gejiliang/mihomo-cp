package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/server"
)

func main() {
	host := flag.String("host", "0.0.0.0", "address to listen on")
	port := flag.Int("port", 8080, "port to listen on")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	srv := server.New(server.Config{
		Host: *host,
		Port: *port,
	})

	slog.Info("starting server", "host", *host, "port", *port)

	go func() {
		if err := srv.Start(); err != nil {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "err", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}
