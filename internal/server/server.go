package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/gejiliang/mihomo-cp/internal/handler"
	"github.com/gejiliang/mihomo-cp/internal/middleware"
	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
	webFS "github.com/gejiliang/mihomo-cp/web"
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
	precheckHandler     *handler.NativePrecheckHandler
	importHandler       *handler.ImportHandler
	settingsHandler     *handler.SettingsHandler
	trafficHandler      *handler.TrafficHandler
	subscriptionHandler *handler.SubscriptionHandler
	scoresHandler       *handler.ScoresHandler
	trafficSvc          *service.TrafficService
	scoringSvc          *service.ScoringService
	subscriptionSvc     *service.SubscriptionService
	trafficCancel       context.CancelFunc
	scoringCancel       context.CancelFunc
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
			mihomoBaseURL = st.ExtController
		}
		mihomoSecret = st.ExtSecret
	}
	mihomoClient := service.NewMihomoClient(mihomoBaseURL, mihomoSecret)

	publishSvc := service.NewPublishService(publishStore, configSvc, validator, mihomoClient)

	trafficStore := store.NewTrafficStore(db)
	notifySvc := service.NewNotifyService()
	trafficSvc := service.NewTrafficService(trafficStore, mihomoClient, notifySvc)

	// Subscriptions & node exit-IP scoring (wired before handlers: the proxy
	// handler enqueues scoring work on proxy create/update)
	subscriptionStore := store.NewSubscriptionStore(db)
	nodeScoreStore := store.NewNodeScoreStore(db)
	scoringSvc := service.NewScoringService(nodeScoreStore, proxyStore, proxyGroupStore, subscriptionStore, configStore, settingsStore, nil)
	subscriptionSvc := service.NewSubscriptionService(subscriptionStore, configStore, trafficStore, nodeScoreStore, proxyGroupStore, proxyStore, mihomoClient, settingsStore, scoringSvc)

	// Handlers
	authHandler := handler.NewAuthHandler(userStore, authSvc)
	proxyHandler := handler.NewProxyHandler(proxyStore, proxyGroupStore, scoringSvc, settingsStore)
	proxyGroupHandler := handler.NewProxyGroupHandler(proxyGroupStore, ruleStore)
	ruleHandler := handler.NewRuleHandler(ruleStore)
	ruleProviderHandler := handler.NewRuleProviderHandler(ruleProviderStore, settingsStore, mihomoClient)
	systemConfigHandler := handler.NewSystemConfigHandler(configStore)
	publishHandler := handler.NewPublishHandler(
		proxyStore, proxyGroupStore, ruleStore, ruleProviderStore,
		configStore, publishStore, settingsStore,
		configSvc, validator, publishSvc, importSvc,
	)
	runtimeHandler := handler.NewRuntimeHandler(mihomoClient)
	precheckHandler := handler.NewNativePrecheckHandler(service.NewNativePrecheckService(mihomoClient))
	trafficHandler := handler.NewTrafficHandler(trafficSvc, trafficStore, notifySvc)
	subscriptionHandler := handler.NewSubscriptionHandler(subscriptionSvc)
	scoresHandler := handler.NewScoresHandler(nodeScoreStore, scoringSvc)
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
		precheckHandler:     precheckHandler,
		importHandler:       importHandler,
		settingsHandler:     settingsHandler,
		trafficHandler:      trafficHandler,
		subscriptionHandler: subscriptionHandler,
		scoresHandler:       scoresHandler,
		trafficSvc:          trafficSvc,
		scoringSvc:          scoringSvc,
		subscriptionSvc:     subscriptionSvc,
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
	s.mux.Handle("PUT /api/auth/change-password", protected(s.authHandler.ChangePassword))

	// ── Proxies ───────────────────────────────────────────────────────────────
	s.mux.Handle("GET /api/proxies", protected(s.proxyHandler.List))
	s.mux.Handle("POST /api/proxies", adminOnly(s.proxyHandler.Create))
	s.mux.Handle("POST /api/proxies/reorder", adminOnly(s.proxyHandler.Reorder))
	s.mux.Handle("GET /api/proxies/{id}", protected(s.proxyHandler.Get))
	s.mux.Handle("PUT /api/proxies/{id}", adminOnly(s.proxyHandler.Update))
	s.mux.Handle("DELETE /api/proxies/{id}", adminOnly(s.proxyHandler.Delete))
	s.mux.Handle("POST /api/proxies/{id}/copy", adminOnly(s.proxyHandler.Copy))
	s.mux.Handle("GET /api/proxies/{id}/refs", protected(s.proxyHandler.Refs))

	// ── Proxy Groups ──────────────────────────────────────────────────────────
	s.mux.Handle("GET /api/proxy-groups", protected(s.proxyGroupHandler.List))
	s.mux.Handle("POST /api/proxy-groups", adminOnly(s.proxyGroupHandler.Create))
	s.mux.Handle("POST /api/proxy-groups/reorder", adminOnly(s.proxyGroupHandler.Reorder))
	s.mux.Handle("GET /api/proxy-groups/{id}", protected(s.proxyGroupHandler.Get))
	s.mux.Handle("PUT /api/proxy-groups/{id}", adminOnly(s.proxyGroupHandler.Update))
	s.mux.Handle("DELETE /api/proxy-groups/{id}", adminOnly(s.proxyGroupHandler.Delete))
	s.mux.Handle("POST /api/proxy-groups/{id}/copy", adminOnly(s.proxyGroupHandler.Copy))
	s.mux.Handle("GET /api/proxy-groups/{id}/refs", protected(s.proxyGroupHandler.Refs))

	// ── Rules ─────────────────────────────────────────────────────────────────
	s.mux.Handle("GET /api/rules", protected(s.ruleHandler.List))
	s.mux.Handle("POST /api/rules", adminOnly(s.ruleHandler.Create))
	s.mux.Handle("POST /api/rules/reorder", adminOnly(s.ruleHandler.Reorder))
	s.mux.Handle("GET /api/rules/{id}", protected(s.ruleHandler.Get))
	s.mux.Handle("PUT /api/rules/{id}", adminOnly(s.ruleHandler.Update))
	s.mux.Handle("DELETE /api/rules/{id}", adminOnly(s.ruleHandler.Delete))

	// ── Rule Providers ────────────────────────────────────────────────────────
	s.mux.Handle("GET /api/rule-providers", protected(s.ruleProviderHandler.List))
	s.mux.Handle("POST /api/rule-providers", adminOnly(s.ruleProviderHandler.Create))
	s.mux.Handle("GET /api/rule-providers/{id}", protected(s.ruleProviderHandler.Get))
	s.mux.Handle("PUT /api/rule-providers/{id}", adminOnly(s.ruleProviderHandler.Update))
	s.mux.Handle("DELETE /api/rule-providers/{id}", adminOnly(s.ruleProviderHandler.Delete))
	s.mux.Handle("POST /api/rule-providers/{id}/refresh", adminOnly(s.ruleProviderHandler.Refresh))
	s.mux.Handle("GET /api/rule-providers/{id}/content", protected(s.ruleProviderHandler.GetContent))
	s.mux.Handle("PUT /api/rule-providers/{id}/content", adminOnly(s.ruleProviderHandler.UpdateContent))

	// ── System Config ─────────────────────────────────────────────────────────
	// Raw system config JSON contains subscription URLs and secrets — admin
	// only for reads and writes.
	s.mux.Handle("GET /api/system-config", adminOnly(s.systemConfigHandler.Get))
	s.mux.Handle("PUT /api/system-config", adminOnly(s.systemConfigHandler.Update))

	// ── Publish ───────────────────────────────────────────────────────────────
	// Preview and history render the full config (provider URLs, secrets) —
	// admin only. Status and validation stay readable.
	s.mux.Handle("GET /api/publish/preview", adminOnly(s.publishHandler.Preview))
	s.mux.Handle("POST /api/publish/validate", adminOnly(s.publishHandler.Validate))
	s.mux.Handle("POST /api/publish", adminOnly(s.publishHandler.Publish))
	s.mux.Handle("POST /api/publish/discard", adminOnly(s.publishHandler.Discard))
	s.mux.Handle("POST /api/publish/rollback", adminOnly(s.publishHandler.Rollback))
	s.mux.Handle("GET /api/publish/history", adminOnly(s.publishHandler.History))
	s.mux.Handle("GET /api/publish/history/{id}", adminOnly(s.publishHandler.HistoryDetail))
	s.mux.Handle("GET /api/publish/status", protected(s.publishHandler.Status))

	// ── Runtime (proxy to mihomo) ─────────────────────────────────────────────
	s.mux.Handle("POST /api/runtime/native-precheck", protected(s.precheckHandler.Check))
	s.mux.Handle("GET /api/runtime/connections", protected(s.runtimeHandler.Connections))
	s.mux.Handle("DELETE /api/runtime/connections/{id}", adminOnly(s.runtimeHandler.CloseConnection))
	s.mux.Handle("GET /api/runtime/proxies", protected(s.runtimeHandler.Proxies))
	s.mux.Handle("GET /api/runtime/proxies/{name}/delay", protected(s.runtimeHandler.ProxyDelay))
	s.mux.Handle("PUT /api/runtime/proxies/{group}/selected", adminOnly(s.runtimeHandler.SwitchProxy))
	s.mux.Handle("GET /api/runtime/rules", protected(s.runtimeHandler.Rules))
	s.mux.Handle("GET /api/runtime/providers", protected(s.runtimeHandler.Providers))
	s.mux.Handle("GET /api/runtime/proxy-providers", protected(s.runtimeHandler.ProxyProviders))
	s.mux.Handle("GET /api/runtime/proxy-providers/{provider}/{node}/healthcheck", protected(s.runtimeHandler.ProxyProviderNodeDelay))
	s.mux.Handle("PUT /api/runtime/providers/rules/{name}", adminOnly(s.runtimeHandler.RefreshProvider))
	s.mux.Handle("GET /api/runtime/version", protected(s.runtimeHandler.Version))

	// ── Traffic accounting & alerting ─────────────────────────────────────────
	s.mux.Handle("GET /api/traffic/summary", protected(s.trafficHandler.Summary))
	s.mux.Handle("GET /api/traffic/history", protected(s.trafficHandler.History))
	s.mux.Handle("GET /api/traffic/providers", protected(s.trafficHandler.ListProviders))
	s.mux.Handle("POST /api/traffic/providers", adminOnly(s.trafficHandler.CreateProvider))
	s.mux.Handle("PUT /api/traffic/providers/{id}", adminOnly(s.trafficHandler.UpdateProvider))
	s.mux.Handle("DELETE /api/traffic/providers/{id}", adminOnly(s.trafficHandler.DeleteProvider))
	s.mux.Handle("GET /api/traffic/alerts", protected(s.trafficHandler.Alerts))
	s.mux.Handle("GET /api/traffic/settings", adminOnly(s.trafficHandler.GetSettings))
	s.mux.Handle("PUT /api/traffic/settings", adminOnly(s.trafficHandler.UpdateSettings))
	s.mux.Handle("POST /api/traffic/test-notify", adminOnly(s.trafficHandler.TestNotify))

	// ── Subscriptions & node scores ───────────────────────────────────────────
	s.mux.Handle("GET /api/subscriptions", protected(s.subscriptionHandler.List))
	s.mux.Handle("POST /api/subscriptions", adminOnly(s.subscriptionHandler.Create))
	s.mux.Handle("GET /api/subscriptions/providers", protected(s.subscriptionHandler.Discover))
	s.mux.Handle("GET /api/subscriptions/{id}", protected(s.subscriptionHandler.Get))
	s.mux.Handle("PUT /api/subscriptions/{id}", adminOnly(s.subscriptionHandler.Update))
	s.mux.Handle("DELETE /api/subscriptions/{id}", adminOnly(s.subscriptionHandler.Delete))
	s.mux.Handle("POST /api/subscriptions/{id}/refresh", adminOnly(s.subscriptionHandler.Refresh))

	s.mux.Handle("GET /api/node-scores", protected(s.scoresHandler.List))
	s.mux.Handle("POST /api/node-scores/refresh", adminOnly(s.scoresHandler.RefreshBatch))
	s.mux.Handle("GET /api/node-scores/{id}", protected(s.scoresHandler.Get))
	s.mux.Handle("POST /api/node-scores/{id}/refresh", adminOnly(s.scoresHandler.RefreshOne))

	// ── Import ────────────────────────────────────────────────────────────────
	s.mux.Handle("POST /api/import/config", adminOnly(s.importHandler.Import))

	// ── Settings ──────────────────────────────────────────────────────────────
	// App settings carry the controller secret and isolation underlay —
	// admin only. Raw config YAML editor likewise.
	s.mux.Handle("GET /api/settings", adminOnly(s.settingsHandler.GetSettings))
	s.mux.Handle("PUT /api/settings", adminOnly(s.settingsHandler.UpdateSettings))

	// Config YAML editor
	s.mux.Handle("GET /api/settings/config-yaml", adminOnly(s.settingsHandler.GetConfigYAML))
	s.mux.Handle("PUT /api/settings/config-yaml", adminOnly(s.settingsHandler.UpdateConfigYAML))
	s.mux.Handle("DELETE /api/settings/config-yaml", adminOnly(s.settingsHandler.DeleteConfigYAML))

	// User management — admin only
	s.mux.Handle("GET /api/settings/users", adminOnly(s.settingsHandler.ListUsers))
	s.mux.Handle("POST /api/settings/users", adminOnly(s.settingsHandler.CreateUser))
	s.mux.Handle("PUT /api/settings/users/{id}", adminOnly(s.settingsHandler.UpdateUser))
	s.mux.Handle("DELETE /api/settings/users/{id}", adminOnly(s.settingsHandler.DeleteUser))

	// ── Embedded frontend (SPA) ────────────────────────────────────────────────
	distFS, err := fs.Sub(webFS.DistFS, "dist")
	if err != nil {
		panic("failed to create sub filesystem: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(distFS))

	// SPA fallback: serve index.html for any path not matched by API or static files.
	s.mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}
		_, statErr := fs.Stat(distFS, path[1:]) // remove leading /
		isHTML := path == "/index.html" || statErr != nil
		if isHTML {
			// Prevent browser from caching index.html so new deploys take effect immediately.
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		if statErr != nil {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	}))
}

// Start begins listening and serving HTTP requests. It blocks until the server
// is closed or encounters an error other than http.ErrServerClosed.
func (s *Server) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	s.trafficCancel = cancel
	go s.trafficSvc.Run(ctx)

	scoringCtx, scoringCancel := context.WithCancel(context.Background())
	s.scoringCancel = scoringCancel
	go s.scoringSvc.Run(scoringCtx)
	go s.subscriptionSvc.Run(scoringCtx)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully stops the server using the provided context.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.trafficCancel != nil {
		s.trafficCancel()
	}
	if s.scoringCancel != nil {
		s.scoringCancel()
	}
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
