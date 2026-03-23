package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Config holds the HTTP server configuration.
type Config struct {
	Host string
	Port int
}

// Server wraps the standard library HTTP server and mux.
type Server struct {
	httpServer *http.Server
	mux        *http.ServeMux
}

// New creates a new Server from the given Config.
func New(cfg Config) *Server {
	mux := http.NewServeMux()
	s := &Server{
		mux: mux,
		httpServer: &http.Server{
			Addr:    fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Handler: mux,
		},
	}
	s.registerRoutes()
	return s
}

// registerRoutes wires up all HTTP routes.
func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
}

// Start begins listening and serving HTTP requests. It blocks until the server
// is closed or encounters an error other than http.ErrServerClosed.
func (s *Server) Start() error {
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully stops the server using the provided context.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
