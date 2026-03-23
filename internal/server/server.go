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
	httpServer          *http.Server
	mux                 *http.ServeMux
	db                  *store.DB
	authHandler         *handler.AuthHandler
	authSvc             *service.AuthService
	proxyHandler        *handler.ProxyHandler
	proxyGroupHandler   *handler.ProxyGroupHandler
	ruleHandler         *handler.RuleHandler
	ruleProviderHandler *handler.RuleProviderHandler
	systemConfigHandler *handler.SystemConfigHandler
	publishHandler      *handler.PublishHandler
	runtimeHandler      *handler.RuntimeHandler
	importHandler       *handler.ImportHandler
	settingsHandler     *handler.SettingsHandler
}

// New creates a new Server from the given Config and database.
func New(cfg Config, db *store.DB) *Server {
	mux := http.NewServeMux()

	// Generate a random JWT secret on each startup (tokens won't survive restarts — OK for MVP).
	secret := generateSecret()

	// Stores
	userStore := store.NewUserStore(db)
	proxyStore := store.NewProxyStore(db)
	proxyGroupStore := store.NewProxyGroupStore(db)
	ruleStore := store.NewRuleStore(db)
	ruleProviderStore := store.NewRuleProviderStore(db)
	configStore := store.NewConfigStore(db)
	publishStore := store.NewPublishStore(db)
	settingsStore := store.NewSettingsStore(db)

	// Services
	authSvc := service.NewAuthService(secret, 1, 24*7) // 1h access, 7d refresh
	configSvc := service.NewConfigService()
	validator := service.NewValidator()
	importSvc := service.NewImportService()

	// MihomoClient — read settings for ext_controller URL and secret.
	// Default to localhost:9090 if settings cannot be read yet (first boot).
	mihomoBaseURL := "http://127.0.0.1:9090"
	mihomoSecret := ""
	if st, err := settingsStore.Get(); err == nil {
		if st.ExtController != "" {
			mihomoBaseURL = "http://" + st.ExtController
		}
		mihomoSecret = st.ExtSecret
	}
	mihomoClient := service.NewMihomoClient(mihomoBaseURL, mihomoSecret)

	publishSvc := service.NewPublishService(publishStore, configSvc, validator, mihomoClient)

	// Handlers
	authHandler := handler.NewAuthHandler(userStore, authSvc)
	proxyHandler := handler.NewProxyHandler(proxyStore, proxyGroupStore)
	proxyGroupHandler := handler.NewProxyGroupHandler(proxyGroupStore, ruleStore)
	ruleHandler := handler.NewRuleHandler(ruleStore)
	ruleProviderHandler := handler.NewRuleProviderHandler(ruleProviderStore, mihomoClient)
	systemConfigHandler := handler.NewSystemConfigHandler(configStore)
	publishHandler := handler.NewPublishHandler(
		proxyStore, proxyGroupStore, ruleStore, ruleProviderStore,
		configStore, publishStore, settingsStore,
		configSvc, validator, publishSvc,
	)
	runtimeHandler := handler.NewRuntimeHandler(mihomoClient)
	importHandler := handler.NewImportHandler(
		importSvc, proxyStore, proxyGroupStore, ruleStore, ruleProviderStore, configStore, settingsStore,
	)
	settingsHandler := handler.NewSettingsHandler(settingsStore, userStore, authSvc)

	s := &Server{
		mux:                 mux,
		db:                  db,
		authSvc:             authSvc,
		authHandler:         authHandler,
		proxyHandler:        proxyHandler,
		proxyGroupHandler:   proxyGroupHandler,
		ruleHandler:         ruleHandler,
		ruleProviderHandler: ruleProviderHandler,
		systemConfigHandler: systemConfigHandler,
		publishHandler:      publishHandler,
		runtimeHandler:      runtimeHandler,
		importHandler:       importHandler,
		settingsHandler:     settingsHandler,
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
	auth := middleware.Auth(s.authSvc)

	// Helper: wrap a HandlerFunc with auth middleware.
	protected := func(h http.HandlerFunc) http.Handler {
		return auth(http.HandlerFunc(h))
	}
	// Helper: wrap a Handler with auth + admin-only middleware.
	adminOnly := func(h http.HandlerFunc) http.Handler {
		return auth(middleware.RequireAdmin(http.HandlerFunc(h)))
	}

	// ── Public routes ──────────────────────────────────────────────────────────
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"status":"ok"}}`))
	})
	s.mux.HandleFunc("POST /api/auth/login", s.authHandler.Login)
	s.mux.HandleFunc("POST /api/auth/refresh", s.authHandler.Refresh)

	// ── Auth ──────────────────────────────────────────────────────────────────
	s.mux.Handle("GET /api/auth/me", protected(s.authHandler.Me))

	// ── Proxies ───────────────────────────────────────────────────────────────
	s.mux.Handle("GET /api/proxies", protected(s.proxyHandler.List))
	s.mux.Handle("POST /api/proxies", protected(s.proxyHandler.Create))
	s.mux.Handle("POST /api/proxies/reorder", protected(s.proxyHandler.Reorder))
	s.mux.Handle("GET /api/proxies/{id}", protected(s.proxyHandler.Get))
	s.mux.Handle("PUT /api/proxies/{id}", protected(s.proxyHandler.Update))
	s.mux.Handle("DELETE /api/proxies/{id}", protected(s.proxyHandler.Delete))
	s.mux.Handle("POST /api/proxies/{id}/copy", protected(s.proxyHandler.Copy))
	s.mux.Handle("GET /api/proxies/{id}/refs", protected(s.proxyHandler.Refs))

	// ── Proxy Groups ──────────────────────────────────────────────────────────
	s.mux.Handle("GET /api/proxy-groups", protected(s.proxyGroupHandler.List))
	s.mux.Handle("POST /api/proxy-groups", protected(s.proxyGroupHandler.Create))
	s.mux.Handle("POST /api/proxy-groups/reorder", protected(s.proxyGroupHandler.Reorder))
	s.mux.Handle("GET /api/proxy-groups/{id}", protected(s.proxyGroupHandler.Get))
	s.mux.Handle("PUT /api/proxy-groups/{id}", protected(s.proxyGroupHandler.Update))
	s.mux.Handle("DELETE /api/proxy-groups/{id}", protected(s.proxyGroupHandler.Delete))
	s.mux.Handle("POST /api/proxy-groups/{id}/copy", protected(s.proxyGroupHandler.Copy))
	s.mux.Handle("GET /api/proxy-groups/{id}/refs", protected(s.proxyGroupHandler.Refs))

	// ── Rules ─────────────────────────────────────────────────────────────────
	s.mux.Handle("GET /api/rules", protected(s.ruleHandler.List))
	s.mux.Handle("POST /api/rules", protected(s.ruleHandler.Create))
	s.mux.Handle("POST /api/rules/reorder", protected(s.ruleHandler.Reorder))
	s.mux.Handle("GET /api/rules/{id}", protected(s.ruleHandler.Get))
	s.mux.Handle("PUT /api/rules/{id}", protected(s.ruleHandler.Update))
	s.mux.Handle("DELETE /api/rules/{id}", protected(s.ruleHandler.Delete))

	// ── Rule Providers ────────────────────────────────────────────────────────
	s.mux.Handle("GET /api/rule-providers", protected(s.ruleProviderHandler.List))
	s.mux.Handle("POST /api/rule-providers", protected(s.ruleProviderHandler.Create))
	s.mux.Handle("GET /api/rule-providers/{id}", protected(s.ruleProviderHandler.Get))
	s.mux.Handle("PUT /api/rule-providers/{id}", protected(s.ruleProviderHandler.Update))
	s.mux.Handle("DELETE /api/rule-providers/{id}", protected(s.ruleProviderHandler.Delete))
	s.mux.Handle("POST /api/rule-providers/{id}/refresh", protected(s.ruleProviderHandler.Refresh))

	// ── System Config ─────────────────────────────────────────────────────────
	s.mux.Handle("GET /api/system-config", protected(s.systemConfigHandler.Get))
	s.mux.Handle("PUT /api/system-config", protected(s.systemConfigHandler.Update))

	// ── Publish ───────────────────────────────────────────────────────────────
	s.mux.Handle("GET /api/publish/preview", protected(s.publishHandler.Preview))
	s.mux.Handle("POST /api/publish/validate", protected(s.publishHandler.Validate))
	s.mux.Handle("POST /api/publish", protected(s.publishHandler.Publish))
	s.mux.Handle("POST /api/publish/rollback", protected(s.publishHandler.Rollback))
	s.mux.Handle("GET /api/publish/history", protected(s.publishHandler.History))
	s.mux.Handle("GET /api/publish/history/{id}", protected(s.publishHandler.HistoryDetail))
	s.mux.Handle("GET /api/publish/status", protected(s.publishHandler.Status))

	// ── Runtime (proxy to mihomo) ─────────────────────────────────────────────
	s.mux.Handle("GET /api/runtime/connections", protected(s.runtimeHandler.Connections))
	s.mux.Handle("DELETE /api/runtime/connections/{id}", protected(s.runtimeHandler.CloseConnection))
	s.mux.Handle("GET /api/runtime/proxies", protected(s.runtimeHandler.Proxies))
	s.mux.Handle("GET /api/runtime/proxies/{name}/delay", protected(s.runtimeHandler.ProxyDelay))
	s.mux.Handle("PUT /api/runtime/proxies/{group}/selected", protected(s.runtimeHandler.SwitchProxy))
	s.mux.Handle("GET /api/runtime/rules", protected(s.runtimeHandler.Rules))
	s.mux.Handle("GET /api/runtime/providers", protected(s.runtimeHandler.Providers))
	s.mux.Handle("PUT /api/runtime/providers/rules/{name}", protected(s.runtimeHandler.RefreshProvider))
	s.mux.Handle("GET /api/runtime/version", protected(s.runtimeHandler.Version))

	// ── Import ────────────────────────────────────────────────────────────────
	s.mux.Handle("POST /api/import/config", protected(s.importHandler.Import))

	// ── Settings ──────────────────────────────────────────────────────────────
	s.mux.Handle("GET /api/settings", protected(s.settingsHandler.GetSettings))
	s.mux.Handle("PUT /api/settings", protected(s.settingsHandler.UpdateSettings))

	// User management — admin only
	s.mux.Handle("GET /api/settings/users", adminOnly(s.settingsHandler.ListUsers))
	s.mux.Handle("POST /api/settings/users", adminOnly(s.settingsHandler.CreateUser))
	s.mux.Handle("PUT /api/settings/users/{id}", adminOnly(s.settingsHandler.UpdateUser))
	s.mux.Handle("DELETE /api/settings/users/{id}", adminOnly(s.settingsHandler.DeleteUser))
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
