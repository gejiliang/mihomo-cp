package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/gejiliang/mihomo-cp/internal/handler"
	"github.com/gejiliang/mihomo-cp/internal/middleware"
	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
)

// Config holds the HTTP server configuration.
type Config struct {
	Host string
	Port int
}

// Server wraps the standard library HTTP server and mux.
type Server struct {
	httpServer  *http.Server
	mux         *http.ServeMux
	db          *store.DB
	authHandler *handler.AuthHandler
	authSvc     *service.AuthService
}

// New creates a new Server from the given Config and database.
func New(cfg Config, db *store.DB) *Server {
	mux := http.NewServeMux()

	// Generate a random JWT secret on each startup (tokens won't survive restarts — OK for MVP).
	secret := generateSecret()

	authSvc := service.NewAuthService(secret, 1, 24*7) // 1h access, 7d refresh
	userStore := store.NewUserStore(db)
	authHandler := handler.NewAuthHandler(userStore, authSvc)

	s := &Server{
		mux:         mux,
		db:          db,
		authSvc:     authSvc,
		authHandler: authHandler,
		httpServer: &http.Server{
			Addr:    fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Handler: middleware.Logging(mux),
		},
	}
	s.registerRoutes()
	return s
}

// registerRoutes wires up all HTTP routes.
func (s *Server) registerRoutes() {
	authMiddleware := middleware.Auth(s.authSvc)

	// Public routes.
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"status":"ok"}}`))
	})
	s.mux.HandleFunc("POST /api/auth/login", s.authHandler.Login)
	s.mux.HandleFunc("POST /api/auth/refresh", s.authHandler.Refresh)

	// Protected routes.
	s.mux.Handle("GET /api/auth/me", authMiddleware(http.HandlerFunc(s.authHandler.Me)))
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

// generateSecret generates a random 32-byte hex string for use as a JWT secret.
func generateSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("failed to generate JWT secret: %v", err))
	}
	return hex.EncodeToString(b)
}
