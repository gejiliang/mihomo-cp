# Mihomo Control Plane Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Status:** MVP implemented (2026-03-25). All tasks complete.

**Goal:** Build a single-binary web management console for locally deployed mihomo, providing config editing, publish/rollback, and runtime observability.

**Architecture:** Go backend with embedded React SPA frontend. SQLite for persistence. Backend reads/writes mihomo config files directly and proxies runtime API calls to mihomo's external-controller. Draft-first editing model — all changes stay in draft until validated and published.

**Tech Stack:** Go 1.25, `net/http`, SQLite (`modernc.org/sqlite`), `sqlx`, `yaml.v3` | React 19, TypeScript, Vite, Tailwind CSS 4, shadcn/ui, Zustand, CodeMirror 6, `@dnd-kit/core`

**Spec:** `docs/superpowers/specs/2026-03-23-mihomo-control-plane-design.md`

---

## File Structure

```
cmd/mihomo-cp/main.go                    # Entry point, flag parsing, startup
internal/
  server/server.go                       # HTTP server, middleware chain, route registration
  middleware/auth.go                     # JWT verification middleware
  middleware/logging.go                  # Request logging middleware
  handler/auth.go                        # Login, refresh, me
  handler/proxy.go                       # Proxy CRUD + reorder + refs
  handler/proxy_group.go                 # Proxy group CRUD + reorder + refs
  handler/rule.go                        # Rule CRUD + reorder
  handler/rule_provider.go               # Rule provider CRUD + refresh
  handler/system_config.go               # System config get/put
  handler/publish.go                     # Preview, validate, publish, rollback, history
  handler/runtime.go                     # Proxy to mihomo external-controller
  handler/import.go                      # Import existing config
  handler/settings.go                    # App settings + user management
  handler/helpers.go                     # Shared handler utilities (JSON response, error handling)
  model/proxy.go                         # Proxy node model
  model/proxy_group.go                   # Proxy group model
  model/rule.go                          # Rule model
  model/rule_provider.go                 # Rule provider model
  model/config.go                        # Full mihomo config assembly model
  model/publish.go                       # Publish record model
  model/user.go                          # User model
  model/settings.go                      # App settings model
  store/db.go                            # SQLite connection, migration runner
  store/migrations.go                    # SQL migration definitions
  store/proxy_store.go                   # Proxy persistence
  store/proxy_group_store.go             # Proxy group persistence
  store/rule_store.go                    # Rule persistence
  store/rule_provider_store.go           # Rule provider persistence
  store/config_store.go                  # System config persistence
  store/publish_store.go                 # Publish history persistence
  store/user_store.go                    # User persistence
  service/config_service.go              # Draft → YAML assembly
  service/config_service_test.go         # Config assembly tests
  service/validator.go                   # Reference checks, structural validation
  service/validator_test.go              # Validator tests
  service/publish_service.go             # Publish workflow (validate → backup → write → reload)
  service/publish_service_test.go        # Publish tests
  service/mihomo_client.go               # HTTP client for mihomo external-controller
  service/import_service.go              # YAML → structured data import
  service/import_service_test.go         # Import tests
  service/auth_service.go                # JWT token generation/validation
  service/auth_service_test.go           # Auth tests
  service/geoip_service.go              # GeoIP detection via mihomo SOCKS proxy
web/                                     # React frontend (Vite project)
  index.html
  vite.config.ts
  tsconfig.json
  tailwind.config.ts
  postcss.config.js
  package.json
  components.json                        # shadcn/ui config
  src/
    main.tsx
    App.tsx
    routes.tsx
    api/client.ts                        # ky instance with auth interceptor
    api/proxies.ts
    api/proxy-groups.ts
    api/rules.ts
    api/rule-providers.ts
    api/system-config.ts
    api/publish.ts
    api/runtime.ts
    api/auth.ts
    api/settings.ts
    stores/auth.ts                       # Zustand auth store
    stores/draft.ts                      # Draft dirty state tracking
    pages/login.tsx
    pages/overview.tsx
    pages/proxies.tsx
    pages/proxy-groups.tsx
    pages/rules.tsx
    pages/rule-providers.tsx
    pages/system-config.tsx
    pages/publish.tsx
    pages/runtime.tsx
    pages/settings.tsx
    components/layout/sidebar.tsx
    components/layout/header.tsx
    components/layout/app-layout.tsx
    components/shared/data-table.tsx
    components/shared/confirm-dialog.tsx
    components/shared/yaml-editor.tsx
    components/shared/diff-viewer.tsx
    components/proxies/proxy-form.tsx
    components/proxies/proxy-list.tsx
    components/proxy-groups/group-form.tsx
    components/proxy-groups/group-member-list.tsx
    components/rules/rule-form.tsx
    components/rules/rule-list.tsx
    components/runtime/connection-table.tsx
    components/runtime/log-viewer.tsx
    components/publish/publish-preview.tsx
    components/publish/history-list.tsx
    lib/utils.ts                         # cn() helper etc
```

---

## Task 1: Project Scaffolding — Go Backend

**Files:**
- Create: `cmd/mihomo-cp/main.go`
- Create: `go.mod`
- Create: `internal/server/server.go`
- Create: `internal/handler/helpers.go`
- Create: `Makefile`
- Create: `.gitignore`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/gejiliang/Projects/homelab
go mod init github.com/gejiliang/mihomo-cp
```

- [ ] **Step 2: Create .gitignore**

```gitignore
# Go
/mihomo-cp
*.exe
*.test
*.out

# Frontend
web/node_modules/
web/dist/

# Data
*.db
*.db-journal
data/

# IDE
.idea/
.vscode/
*.swp

# OS
.DS_Store
```

- [ ] **Step 3: Create handler helpers — shared JSON response utilities**

```go
// internal/handler/helpers.go
package handler

import (
	"encoding/json"
	"net/http"
)

type APIResponse struct {
	Data  any        `json:"data,omitempty"`
	Error *APIError  `json:"error,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{Data: data})
}

func Error(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{Error: &APIError{Code: code, Message: message}})
}

func DecodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
```

- [ ] **Step 4: Create server skeleton**

```go
// internal/server/server.go
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type Server struct {
	httpServer *http.Server
	mux        *http.ServeMux
}

type Config struct {
	Host string
	Port int
}

func New(cfg Config) *Server {
	mux := http.NewServeMux()
	s := &Server{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		mux: mux,
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
}

func (s *Server) Start() error {
	slog.Info("server starting", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
```

- [ ] **Step 5: Create main.go entry point**

```go
// cmd/mihomo-cp/main.go
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gejiliang/mihomo-cp/internal/server"
)

func main() {
	host := flag.String("host", "0.0.0.0", "server host")
	port := flag.Int("port", 8080, "server port")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	srv := server.New(server.Config{
		Host: *host,
		Port: *port,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.Start(); err != nil {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	srv.Shutdown(context.Background())
}
```

- [ ] **Step 6: Create Makefile**

```makefile
.PHONY: build run dev test clean

build-backend:
	go build -o mihomo-cp ./cmd/mihomo-cp

run: build-backend
	./mihomo-cp

test:
	go test ./... -v

clean:
	rm -f mihomo-cp
```

- [ ] **Step 7: Download dependencies and verify build**

Run: `go mod tidy && go build ./cmd/mihomo-cp`
Expected: Builds successfully, produces `mihomo-cp` binary

- [ ] **Step 8: Verify health endpoint**

Run: `./mihomo-cp &; sleep 1; curl -s http://localhost:8080/api/health; kill %1`
Expected: `{"status":"ok"}`

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat: project scaffolding — Go backend with health endpoint"
```

---

## Task 2: SQLite Database + Migrations

**Files:**
- Create: `internal/store/db.go`
- Create: `internal/store/migrations.go`
- Modify: `internal/server/server.go`
- Modify: `cmd/mihomo-cp/main.go`

- [ ] **Step 1: Add SQLite dependency**

Run: `go get modernc.org/sqlite && go get github.com/jmoiron/sqlx`

- [ ] **Step 2: Create migration definitions**

```go
// internal/store/migrations.go
package store

var migrations = []string{
	// 001: users table
	`CREATE TABLE IF NOT EXISTS users (
		id         TEXT PRIMARY KEY,
		username   TEXT NOT NULL UNIQUE,
		password   TEXT NOT NULL,
		role       TEXT NOT NULL DEFAULT 'admin',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,

	// 002: proxies table
	`CREATE TABLE IF NOT EXISTS proxies (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL UNIQUE,
		type       TEXT NOT NULL,
		config     TEXT NOT NULL DEFAULT '{}',
		sort_order INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,

	// 003: proxy_groups table
	`CREATE TABLE IF NOT EXISTS proxy_groups (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL UNIQUE,
		type       TEXT NOT NULL,
		config     TEXT NOT NULL DEFAULT '{}',
		members    TEXT NOT NULL DEFAULT '[]',
		sort_order INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,

	// 004: rules table
	`CREATE TABLE IF NOT EXISTS rules (
		id         TEXT PRIMARY KEY,
		type       TEXT NOT NULL,
		payload    TEXT NOT NULL,
		target     TEXT NOT NULL,
		params     TEXT DEFAULT '{}',
		sort_order INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,

	// 005: rule_providers table
	`CREATE TABLE IF NOT EXISTS rule_providers (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL UNIQUE,
		type       TEXT NOT NULL,
		behavior   TEXT NOT NULL,
		config     TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,

	// 006: system_config table (single row)
	`CREATE TABLE IF NOT EXISTS system_config (
		id         INTEGER PRIMARY KEY CHECK (id = 1),
		config     TEXT NOT NULL DEFAULT '{}',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	INSERT OR IGNORE INTO system_config (id, config) VALUES (1, '{}');`,

	// 007: publish_history table
	`CREATE TABLE IF NOT EXISTS publish_history (
		id          TEXT PRIMARY KEY,
		version     INTEGER NOT NULL,
		config_yaml TEXT NOT NULL,
		diff_text   TEXT,
		status      TEXT NOT NULL,
		error_msg   TEXT,
		operator    TEXT NOT NULL,
		note        TEXT,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,

	// 008: app_settings table (single row)
	`CREATE TABLE IF NOT EXISTS app_settings (
		id              INTEGER PRIMARY KEY CHECK (id = 1),
		mihomo_config   TEXT NOT NULL DEFAULT '/etc/mihomo/config.yaml',
		mihomo_dir      TEXT NOT NULL DEFAULT '/etc/mihomo',
		mihomo_binary   TEXT NOT NULL DEFAULT 'mihomo',
		ext_controller  TEXT NOT NULL DEFAULT 'http://127.0.0.1:9090',
		ext_secret      TEXT NOT NULL DEFAULT '',
		updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	INSERT OR IGNORE INTO app_settings (id) VALUES (1);`,
}
```

- [ ] **Step 3: Create database connection and migration runner**

```go
// internal/store/db.go
package store

import (
	"fmt"
	"log/slog"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type DB struct {
	*sqlx.DB
}

func Open(path string) (*DB, error) {
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON", path)
	db, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite single-writer
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &DB{db}, nil
}

func (db *DB) Migrate() error {
	// Create migration tracking table
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY
	)`)
	if err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	var current int
	db.Get(&current, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations")

	for i := current; i < len(migrations); i++ {
		slog.Info("running migration", "version", i+1)
		if _, err := db.Exec(migrations[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", i+1); err != nil {
			return fmt.Errorf("record migration %d: %w", i+1, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Wire database into server and main**

Update `server.go` to accept `*store.DB` and update `main.go` to open DB, run migrations, and pass to server.

- [ ] **Step 5: Verify migrations run**

Run: `go build ./cmd/mihomo-cp && ./mihomo-cp --db ./test.db &; sleep 1; kill %1; sqlite3 test.db ".tables"; rm test.db`
Expected: Tables listed including `users`, `proxies`, `proxy_groups`, `rules`, etc.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: SQLite database with migration system"
```

---

## Task 3: Auth System — Backend

**Files:**
- Create: `internal/model/user.go`
- Create: `internal/store/user_store.go`
- Create: `internal/service/auth_service.go`
- Create: `internal/service/auth_service_test.go`
- Create: `internal/middleware/auth.go`
- Create: `internal/middleware/logging.go`
- Create: `internal/handler/auth.go`
- Modify: `internal/server/server.go`

- [ ] **Step 1: Create user model**

```go
// internal/model/user.go
package model

import "time"

type User struct {
	ID        string    `json:"id" db:"id"`
	Username  string    `json:"username" db:"username"`
	Password  string    `json:"-" db:"password"` // never serialize
	Role      string    `json:"role" db:"role"`   // admin, readonly
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}
```

- [ ] **Step 2: Create user store**

```go
// internal/store/user_store.go
package store

import (
	"github.com/gejiliang/mihomo-cp/internal/model"
)

type UserStore struct{ db *DB }

func NewUserStore(db *DB) *UserStore { return &UserStore{db: db} }

func (s *UserStore) GetByUsername(username string) (*model.User, error) {
	var u model.User
	err := s.db.Get(&u, "SELECT * FROM users WHERE username = ?", username)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *UserStore) GetByID(id string) (*model.User, error) {
	var u model.User
	err := s.db.Get(&u, "SELECT * FROM users WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *UserStore) List() ([]model.User, error) {
	var users []model.User
	return users, s.db.Select(&users, "SELECT * FROM users ORDER BY created_at")
}

func (s *UserStore) Create(u *model.User) error {
	_, err := s.db.NamedExec(`INSERT INTO users (id, username, password, role, created_at, updated_at)
		VALUES (:id, :username, :password, :role, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, u)
	return err
}

func (s *UserStore) Update(u *model.User) error {
	_, err := s.db.NamedExec(`UPDATE users SET username=:username, password=:password, role=:role, updated_at=CURRENT_TIMESTAMP WHERE id=:id`, u)
	return err
}

func (s *UserStore) Delete(id string) error {
	_, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func (s *UserStore) Count() (int, error) {
	var count int
	return count, s.db.Get(&count, "SELECT COUNT(*) FROM users")
}
```

- [ ] **Step 3: Write auth service test**

```go
// internal/service/auth_service_test.go
package service

import (
	"testing"
)

func TestGenerateAndValidateToken(t *testing.T) {
	auth := NewAuthService("test-secret-key", 24, 168)

	token, err := auth.GenerateAccessToken("user-123", "admin")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	claims, err := auth.ValidateToken(token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}

	if claims.UserID != "user-123" {
		t.Errorf("got user_id=%s, want user-123", claims.UserID)
	}
	if claims.Role != "admin" {
		t.Errorf("got role=%s, want admin", claims.Role)
	}
}

func TestHashAndVerifyPassword(t *testing.T) {
	auth := NewAuthService("secret", 24, 168)

	hash, err := auth.HashPassword("mypassword")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if !auth.VerifyPassword(hash, "mypassword") {
		t.Error("password should match")
	}
	if auth.VerifyPassword(hash, "wrongpassword") {
		t.Error("wrong password should not match")
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/service/ -v -run TestGenerate`
Expected: FAIL — `NewAuthService` not defined

- [ ] **Step 5: Implement auth service**

```go
// internal/service/auth_service.go
package service

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	secret          string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	Type   string `json:"type"` // "access" or "refresh"
	jwt.RegisteredClaims
}

func NewAuthService(secret string, accessHours, refreshHours int) *AuthService {
	return &AuthService{
		secret:          secret,
		accessTokenTTL:  time.Duration(accessHours) * time.Hour,
		refreshTokenTTL: time.Duration(refreshHours) * time.Hour,
	}
}

func (s *AuthService) GenerateAccessToken(userID, role string) (string, error) {
	return s.generateToken(userID, role, "access", s.accessTokenTTL)
}

func (s *AuthService) GenerateRefreshToken(userID, role string) (string, error) {
	return s.generateToken(userID, role, "refresh", s.refreshTokenTTL)
}

func (s *AuthService) generateToken(userID, role, tokenType string, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID: userID,
		Role:   role,
		Type:   tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.secret))
}

func (s *AuthService) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}
	return claims, nil
}

func (s *AuthService) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(hash), err
}

func (s *AuthService) VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
```

- [ ] **Step 6: Run tests**

Run: `go get github.com/golang-jwt/jwt/v5 golang.org/x/crypto && go test ./internal/service/ -v`
Expected: PASS

- [ ] **Step 7: Create auth middleware**

```go
// internal/middleware/auth.go
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gejiliang/mihomo-cp/internal/handler"
	"github.com/gejiliang/mihomo-cp/internal/service"
)

type contextKey string

const (
	UserIDKey contextKey = "user_id"
	RoleKey   contextKey = "role"
)

func Auth(authSvc *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				handler.Error(w, http.StatusUnauthorized, "unauthorized", "missing authorization header")
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := authSvc.ValidateToken(token)
			if err != nil {
				handler.Error(w, http.StatusUnauthorized, "unauthorized", "invalid token")
				return
			}
			if claims.Type != "access" {
				handler.Error(w, http.StatusUnauthorized, "unauthorized", "invalid token type")
				return
			}
			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, RoleKey, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, _ := r.Context().Value(RoleKey).(string)
		if role != "admin" {
			handler.Error(w, http.StatusForbidden, "forbidden", "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func GetUserID(ctx context.Context) string {
	id, _ := ctx.Value(UserIDKey).(string)
	return id
}

func GetRole(ctx context.Context) string {
	role, _ := ctx.Value(RoleKey).(string)
	return role
}
```

- [ ] **Step 8: Create logging middleware**

```go
// internal/middleware/logging.go
package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration", time.Since(start),
		)
	})
}
```

- [ ] **Step 9: Create auth handler**

Auth handler handles login, refresh, me, and auto-creates default admin on first request if no users exist.

```go
// internal/handler/auth.go
package handler

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/gejiliang/mihomo-cp/internal/middleware"
	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
)

type AuthHandler struct {
	users   *store.UserStore
	authSvc *service.AuthService
}

func NewAuthHandler(users *store.UserStore, authSvc *service.AuthService) *AuthHandler {
	return &AuthHandler{users: users, authSvc: authSvc}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req model.LoginRequest
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	// Auto-create default admin if no users exist
	count, _ := h.users.Count()
	if count == 0 {
		hash, _ := h.authSvc.HashPassword("admin")
		h.users.Create(&model.User{
			ID:       uuid.New().String(),
			Username: "admin",
			Password: hash,
			Role:     "admin",
		})
	}

	user, err := h.users.GetByUsername(req.Username)
	if err != nil {
		Error(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}

	if !h.authSvc.VerifyPassword(user.Password, req.Password) {
		Error(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}

	access, _ := h.authSvc.GenerateAccessToken(user.ID, user.Role)
	refresh, _ := h.authSvc.GenerateRefreshToken(user.ID, user.Role)

	JSON(w, http.StatusOK, model.TokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
	})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req model.RefreshRequest
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	claims, err := h.authSvc.ValidateToken(req.RefreshToken)
	if err != nil || claims.Type != "refresh" {
		Error(w, http.StatusUnauthorized, "unauthorized", "invalid refresh token")
		return
	}

	user, err := h.users.GetByID(claims.UserID)
	if err != nil {
		Error(w, http.StatusUnauthorized, "unauthorized", "user not found")
		return
	}

	access, _ := h.authSvc.GenerateAccessToken(user.ID, user.Role)
	JSON(w, http.StatusOK, map[string]string{"access_token": access})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	user, err := h.users.GetByID(userID)
	if err != nil {
		Error(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	JSON(w, http.StatusOK, user)
}
```

- [ ] **Step 10: Wire auth routes into server**

Update `server.go` to register auth routes: `POST /api/auth/login`, `POST /api/auth/refresh`, `GET /api/auth/me` (protected).

- [ ] **Step 11: Test login flow manually**

Run: `go get github.com/google/uuid && go build ./cmd/mihomo-cp && ./mihomo-cp --db ./test.db &; sleep 1; curl -s -X POST http://localhost:8080/api/auth/login -d '{"username":"admin","password":"admin"}'; kill %1; rm test.db`
Expected: JSON with access_token and refresh_token

- [ ] **Step 12: Commit**

```bash
git add -A
git commit -m "feat: auth system — JWT, login, middleware, auto-create admin"
```

---

## Task 4: Core Models

**Files:**
- Create: `internal/model/proxy.go`
- Create: `internal/model/proxy_group.go`
- Create: `internal/model/rule.go`
- Create: `internal/model/rule_provider.go`
- Create: `internal/model/config.go`
- Create: `internal/model/publish.go`
- Create: `internal/model/settings.go`

- [ ] **Step 1: Create all model files**

Each model maps to the SQLite schema. JSON config fields use `json.RawMessage` for protocol-specific flexibility.

```go
// internal/model/proxy.go
package model

import (
	"encoding/json"
	"time"
)

type Proxy struct {
	ID        string          `json:"id" db:"id"`
	Name      string          `json:"name" db:"name"`
	Type      string          `json:"type" db:"type"`
	Config    json.RawMessage `json:"config" db:"config"`
	SortOrder int             `json:"sort_order" db:"sort_order"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}

// ValidProxyTypes lists all mihomo-supported proxy types
var ValidProxyTypes = []string{
	"ss", "trojan", "vmess", "vless", "http", "socks5", "hysteria2", "tuic",
}
```

```go
// internal/model/proxy_group.go
package model

import (
	"encoding/json"
	"time"
)

type ProxyGroup struct {
	ID        string          `json:"id" db:"id"`
	Name      string          `json:"name" db:"name"`
	Type      string          `json:"type" db:"type"`
	Config    json.RawMessage `json:"config" db:"config"`
	Members   json.RawMessage `json:"members" db:"members"` // JSON array of names
	SortOrder int             `json:"sort_order" db:"sort_order"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}

var ValidGroupTypes = []string{
	"select", "fallback", "url-test", "load-balance", "relay",
}
```

```go
// internal/model/rule.go
package model

import (
	"encoding/json"
	"time"
)

type Rule struct {
	ID        string          `json:"id" db:"id"`
	Type      string          `json:"type" db:"type"`
	Payload   string          `json:"payload" db:"payload"`
	Target    string          `json:"target" db:"target"`
	Params    json.RawMessage `json:"params" db:"params"`
	SortOrder int             `json:"sort_order" db:"sort_order"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}

var ValidRuleTypes = []string{
	"DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD",
	"IP-CIDR", "IP-CIDR6", "SRC-IP-CIDR",
	"DST-PORT", "PROCESS-NAME", "GEOIP",
	"RULE-SET", "MATCH",
}
```

```go
// internal/model/rule_provider.go
package model

import (
	"encoding/json"
	"time"
)

type RuleProvider struct {
	ID        string          `json:"id" db:"id"`
	Name      string          `json:"name" db:"name"`
	Type      string          `json:"type" db:"type"`     // file, http
	Behavior  string          `json:"behavior" db:"behavior"` // domain, ipcidr, classical
	Config    json.RawMessage `json:"config" db:"config"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}
```

```go
// internal/model/config.go
package model

// MihomoConfig represents the full mihomo configuration for YAML rendering
type MihomoConfig struct {
	// System-level fields rendered from system_config
	Port               int                    `yaml:"port,omitempty"`
	SocksPort          int                    `yaml:"socks-port,omitempty"`
	MixedPort          int                    `yaml:"mixed-port,omitempty"`
	AllowLan           bool                   `yaml:"allow-lan,omitempty"`
	BindAddress        string                 `yaml:"bind-address,omitempty"`
	Mode               string                 `yaml:"mode,omitempty"`
	LogLevel           string                 `yaml:"log-level,omitempty"`
	IPv6               bool                   `yaml:"ipv6,omitempty"`
	ExternalController string                 `yaml:"external-controller,omitempty"`
	Secret             string                 `yaml:"secret,omitempty"`
	DNS                map[string]any         `yaml:"dns,omitempty"`
	Tun                map[string]any         `yaml:"tun,omitempty"`
	Listeners          []map[string]any       `yaml:"listeners,omitempty"`
	Extra              map[string]any         `yaml:",inline"` // passthrough unknown fields

	Proxies       []map[string]any          `yaml:"proxies,omitempty"`
	ProxyGroups   []map[string]any          `yaml:"proxy-groups,omitempty"`
	Rules         []string                   `yaml:"rules,omitempty"`
	RuleProviders map[string]map[string]any `yaml:"rule-providers,omitempty"`
}
```

```go
// internal/model/publish.go
package model

import "time"

type PublishRecord struct {
	ID         string    `json:"id" db:"id"`
	Version    int       `json:"version" db:"version"`
	ConfigYAML string   `json:"config_yaml" db:"config_yaml"`
	DiffText   string    `json:"diff_text" db:"diff_text"`
	Status     string    `json:"status" db:"status"` // success, failed, rolled_back
	ErrorMsg   string    `json:"error_msg" db:"error_msg"`
	Operator   string    `json:"operator" db:"operator"`
	Note       string    `json:"note" db:"note"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}
```

```go
// internal/model/settings.go
package model

import "time"

type AppSettings struct {
	ID             int       `json:"id" db:"id"`
	MihomoConfig   string    `json:"mihomo_config" db:"mihomo_config"`
	MihomoDir      string    `json:"mihomo_dir" db:"mihomo_dir"`
	MihomoBinary   string    `json:"mihomo_binary" db:"mihomo_binary"`
	ExtController  string    `json:"ext_controller" db:"ext_controller"`
	ExtSecret      string    `json:"ext_secret" db:"ext_secret"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./...`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat: core data models — proxy, group, rule, provider, config, publish"
```

---

## Task 5: Store Layer — All CRUD Operations

**Files:**
- Create: `internal/store/proxy_store.go`
- Create: `internal/store/proxy_group_store.go`
- Create: `internal/store/rule_store.go`
- Create: `internal/store/rule_provider_store.go`
- Create: `internal/store/config_store.go`
- Create: `internal/store/publish_store.go`

- [ ] **Step 1: Create proxy store**

Standard CRUD + reorder + reference lookup. All stores follow the same pattern.

```go
// internal/store/proxy_store.go
package store

import "github.com/gejiliang/mihomo-cp/internal/model"

type ProxyStore struct{ db *DB }

func NewProxyStore(db *DB) *ProxyStore { return &ProxyStore{db: db} }

func (s *ProxyStore) List(search, proxyType string) ([]model.Proxy, error) {
	var proxies []model.Proxy
	query := "SELECT * FROM proxies WHERE 1=1"
	args := []any{}
	if search != "" {
		query += " AND name LIKE ?"
		args = append(args, "%"+search+"%")
	}
	if proxyType != "" {
		query += " AND type = ?"
		args = append(args, proxyType)
	}
	query += " ORDER BY sort_order, created_at"
	return proxies, s.db.Select(&proxies, query, args...)
}

func (s *ProxyStore) GetByID(id string) (*model.Proxy, error) {
	var p model.Proxy
	return &p, s.db.Get(&p, "SELECT * FROM proxies WHERE id = ?", id)
}

func (s *ProxyStore) GetByName(name string) (*model.Proxy, error) {
	var p model.Proxy
	return &p, s.db.Get(&p, "SELECT * FROM proxies WHERE name = ?", name)
}

func (s *ProxyStore) Create(p *model.Proxy) error {
	_, err := s.db.NamedExec(`INSERT INTO proxies (id, name, type, config, sort_order, created_at, updated_at)
		VALUES (:id, :name, :type, :config, :sort_order, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, p)
	return err
}

func (s *ProxyStore) Update(p *model.Proxy) error {
	_, err := s.db.NamedExec(`UPDATE proxies SET name=:name, type=:type, config=:config, sort_order=:sort_order, updated_at=CURRENT_TIMESTAMP WHERE id=:id`, p)
	return err
}

func (s *ProxyStore) Delete(id string) error {
	_, err := s.db.Exec("DELETE FROM proxies WHERE id = ?", id)
	return err
}

func (s *ProxyStore) Reorder(ids []string) error {
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.Exec("UPDATE proxies SET sort_order = ? WHERE id = ?", i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *ProxyStore) Count() (int, error) {
	var count int
	return count, s.db.Get(&count, "SELECT COUNT(*) FROM proxies")
}
```

- [ ] **Step 2: Create proxy_group_store, rule_store, rule_provider_store, config_store, publish_store**

Follow the same CRUD pattern as proxy_store. Each store is a thin data access layer. `config_store` and `publish_store` have single-row/append-only semantics respectively.

- [ ] **Step 3: Verify compilation**

Run: `go build ./...`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat: store layer — CRUD for all domain entities"
```

---

## Task 6: Config Assembly Service

**Files:**
- Create: `internal/service/config_service.go`
- Create: `internal/service/config_service_test.go`

- [ ] **Step 1: Write config assembly test**

```go
// internal/service/config_service_test.go
package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

func TestRenderProxies(t *testing.T) {
	proxies := []model.Proxy{
		{Name: "jp-01", Type: "ss", Config: json.RawMessage(`{"server":"1.2.3.4","port":443,"cipher":"aes-256-gcm","password":"secret"}`)},
	}

	cs := &ConfigService{}
	result := cs.renderProxies(proxies)

	if len(result) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result))
	}
	if result[0]["name"] != "jp-01" {
		t.Errorf("expected name=jp-01, got %v", result[0]["name"])
	}
	if result[0]["type"] != "ss" {
		t.Errorf("expected type=ss, got %v", result[0]["type"])
	}
	if result[0]["server"] != "1.2.3.4" {
		t.Errorf("expected server=1.2.3.4, got %v", result[0]["server"])
	}
}

func TestRenderRules(t *testing.T) {
	rules := []model.Rule{
		{Type: "DOMAIN-SUFFIX", Payload: "google.com", Target: "Proxy"},
		{Type: "MATCH", Payload: "", Target: "DIRECT"},
	}

	cs := &ConfigService{}
	result := cs.renderRules(rules)

	if result[0] != "DOMAIN-SUFFIX,google.com,Proxy" {
		t.Errorf("unexpected rule: %s", result[0])
	}
	if result[1] != "MATCH,DIRECT" {
		t.Errorf("unexpected MATCH rule: %s", result[1])
	}
}

func TestRenderFullConfig(t *testing.T) {
	cs := &ConfigService{}
	yaml, err := cs.RenderYAML(
		json.RawMessage(`{"port":7890,"mixed-port":7891,"allow-lan":true,"mode":"rule","log-level":"info","external-controller":"0.0.0.0:9090"}`),
		[]model.Proxy{{Name: "test", Type: "ss", Config: json.RawMessage(`{"server":"1.2.3.4","port":443,"cipher":"aes-256-gcm","password":"pw"}`)}},
		[]model.ProxyGroup{{Name: "Auto", Type: "url-test", Members: json.RawMessage(`["test"]`), Config: json.RawMessage(`{"url":"http://www.gstatic.com/generate_204","interval":300}`)}},
		[]model.Rule{{Type: "MATCH", Payload: "", Target: "DIRECT"}},
		map[string]model.RuleProvider{},
	)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(string(yaml), "port: 7890") {
		t.Errorf("missing port in output:\n%s", yaml)
	}
	if !strings.Contains(string(yaml), "name: test") {
		t.Errorf("missing proxy in output:\n%s", yaml)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/service/ -v -run TestRender`
Expected: FAIL

- [ ] **Step 3: Implement config service**

`ConfigService` takes all draft entities and renders a valid mihomo YAML. Key logic: merge proxy `config` JSON fields into a flat map with `name` and `type` at top level.

```go
// internal/service/config_service.go
package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"gopkg.in/yaml.v3"
)

type ConfigService struct{}

func NewConfigService() *ConfigService {
	return &ConfigService{}
}

func (s *ConfigService) RenderYAML(
	sysConfig json.RawMessage,
	proxies []model.Proxy,
	groups []model.ProxyGroup,
	rules []model.Rule,
	providers map[string]model.RuleProvider,
) ([]byte, error) {
	// Start with system config as base
	cfg := make(map[string]any)
	if len(sysConfig) > 0 && string(sysConfig) != "{}" {
		if err := json.Unmarshal(sysConfig, &cfg); err != nil {
			return nil, fmt.Errorf("parse system config: %w", err)
		}
	}

	cfg["proxies"] = s.renderProxies(proxies)
	cfg["proxy-groups"] = s.renderGroups(groups)
	cfg["rules"] = s.renderRules(rules)

	if len(providers) > 0 {
		cfg["rule-providers"] = s.renderProviders(providers)
	}

	return yaml.Marshal(cfg)
}

func (s *ConfigService) renderProxies(proxies []model.Proxy) []map[string]any {
	result := make([]map[string]any, 0, len(proxies))
	for _, p := range proxies {
		m := make(map[string]any)
		json.Unmarshal(p.Config, &m)
		m["name"] = p.Name
		m["type"] = p.Type
		result = append(result, m)
	}
	return result
}

func (s *ConfigService) renderGroups(groups []model.ProxyGroup) []map[string]any {
	result := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		m := make(map[string]any)
		json.Unmarshal(g.Config, &m)
		m["name"] = g.Name
		m["type"] = g.Type
		var members []string
		json.Unmarshal(g.Members, &members)
		m["proxies"] = members
		result = append(result, m)
	}
	return result
}

func (s *ConfigService) renderRules(rules []model.Rule) []string {
	result := make([]string, 0, len(rules))
	for _, r := range rules {
		if r.Type == "MATCH" {
			result = append(result, fmt.Sprintf("MATCH,%s", r.Target))
		} else {
			parts := []string{r.Type, r.Payload, r.Target}
			// Append optional params
			if len(r.Params) > 0 && string(r.Params) != "{}" && string(r.Params) != "null" {
				var params map[string]any
				if json.Unmarshal(r.Params, &params) == nil {
					if noResolve, ok := params["no-resolve"].(bool); ok && noResolve {
						parts = append(parts, "no-resolve")
					}
				}
			}
			result = append(result, strings.Join(parts, ","))
		}
	}
	return result
}

func (s *ConfigService) renderProviders(providers map[string]model.RuleProvider) map[string]map[string]any {
	result := make(map[string]map[string]any)
	for name, p := range providers {
		m := make(map[string]any)
		json.Unmarshal(p.Config, &m)
		m["type"] = p.Type
		m["behavior"] = p.Behavior
		result[name] = m
	}
	return result
}
```

- [ ] **Step 4: Run tests**

Run: `go get gopkg.in/yaml.v3 && go test ./internal/service/ -v -run TestRender`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: config assembly service — draft entities to mihomo YAML"
```

---

## Task 7: Validation Service

**Files:**
- Create: `internal/service/validator.go`
- Create: `internal/service/validator_test.go`

- [ ] **Step 1: Write validator tests**

Test reference integrity, empty groups, circular refs, MATCH rule presence.

```go
// internal/service/validator_test.go
package service

import (
	"encoding/json"
	"testing"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

func TestValidateDanglingReference(t *testing.T) {
	v := NewValidator()
	errs := v.Validate(
		[]model.Proxy{{Name: "jp-01"}},
		[]model.ProxyGroup{{Name: "JP", Members: json.RawMessage(`["jp-01","jp-02"]`)}},
		[]model.Rule{{Type: "MATCH", Target: "JP"}},
		map[string]model.RuleProvider{},
	)
	found := false
	for _, e := range errs {
		if e.Code == "dangling_reference" {
			found = true
		}
	}
	if !found {
		t.Error("expected dangling_reference error for jp-02")
	}
}

func TestValidateEmptyGroup(t *testing.T) {
	v := NewValidator()
	errs := v.Validate(
		[]model.Proxy{},
		[]model.ProxyGroup{{Name: "Empty", Members: json.RawMessage(`[]`)}},
		[]model.Rule{{Type: "MATCH", Target: "DIRECT"}},
		map[string]model.RuleProvider{},
	)
	found := false
	for _, e := range errs {
		if e.Code == "empty_group" {
			found = true
		}
	}
	if !found {
		t.Error("expected empty_group warning")
	}
}

func TestValidateMissingMatch(t *testing.T) {
	v := NewValidator()
	errs := v.Validate(
		[]model.Proxy{{Name: "jp-01"}},
		[]model.ProxyGroup{},
		[]model.Rule{{Type: "DOMAIN", Payload: "google.com", Target: "DIRECT"}},
		map[string]model.RuleProvider{},
	)
	found := false
	for _, e := range errs {
		if e.Code == "no_match_rule" {
			found = true
		}
	}
	if !found {
		t.Error("expected no_match_rule warning")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/service/ -v -run TestValidate`
Expected: FAIL

- [ ] **Step 3: Implement validator**

```go
// internal/service/validator.go
package service

import (
	"encoding/json"
	"fmt"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

type ValidationError struct {
	Code    string `json:"code"`
	Level   string `json:"level"` // error, warning
	Message string `json:"message"`
}

type Validator struct{}

func NewValidator() *Validator { return &Validator{} }

func (v *Validator) Validate(
	proxies []model.Proxy,
	groups []model.ProxyGroup,
	rules []model.Rule,
	providers map[string]model.RuleProvider,
) []ValidationError {
	var errs []ValidationError

	// Build name sets
	proxyNames := make(map[string]bool)
	for _, p := range proxies {
		proxyNames[p.Name] = true
	}
	groupNames := make(map[string]bool)
	for _, g := range groups {
		groupNames[g.Name] = true
	}
	providerNames := make(map[string]bool)
	for name := range providers {
		providerNames[name] = true
	}
	allNames := make(map[string]bool)
	for k := range proxyNames { allNames[k] = true }
	for k := range groupNames { allNames[k] = true }
	// Built-in targets
	allNames["DIRECT"] = true
	allNames["REJECT"] = true
	allNames["REJECT-DROP"] = true
	allNames["PASS"] = true
	allNames["COMPATIBLE"] = true

	// Check group members reference valid proxies/groups
	for _, g := range groups {
		var members []string
		json.Unmarshal(g.Members, &members)
		if len(members) == 0 {
			errs = append(errs, ValidationError{
				Code:    "empty_group",
				Level:   "warning",
				Message: fmt.Sprintf("proxy group '%s' has no members", g.Name),
			})
		}
		for _, m := range members {
			if !allNames[m] {
				errs = append(errs, ValidationError{
					Code:    "dangling_reference",
					Level:   "error",
					Message: fmt.Sprintf("proxy group '%s' references unknown member '%s'", g.Name, m),
				})
			}
		}
	}

	// Check rule targets
	for _, r := range rules {
		if r.Type == "RULE-SET" {
			if !providerNames[r.Payload] {
				errs = append(errs, ValidationError{
					Code:    "dangling_reference",
					Level:   "error",
					Message: fmt.Sprintf("rule references unknown rule-provider '%s'", r.Payload),
				})
			}
		}
		if !allNames[r.Target] {
			errs = append(errs, ValidationError{
				Code:    "dangling_reference",
				Level:   "error",
				Message: fmt.Sprintf("rule target '%s' is not a known proxy or group", r.Target),
			})
		}
	}

	// Check MATCH rule exists and is last
	hasMatch := false
	for i, r := range rules {
		if r.Type == "MATCH" {
			hasMatch = true
			if i != len(rules)-1 {
				errs = append(errs, ValidationError{
					Code:    "match_not_last",
					Level:   "warning",
					Message: "MATCH rule should be the last rule",
				})
			}
		}
	}
	if !hasMatch && len(rules) > 0 {
		errs = append(errs, ValidationError{
			Code:    "no_match_rule",
			Level:   "warning",
			Message: "no MATCH rule found — unmatched traffic will have no fallback",
		})
	}

	// Check circular group references
	errs = append(errs, v.checkCircularRefs(groups)...)

	return errs
}

func (v *Validator) checkCircularRefs(groups []model.ProxyGroup) []ValidationError {
	var errs []ValidationError
	groupMembers := make(map[string][]string)
	for _, g := range groups {
		var members []string
		json.Unmarshal(g.Members, &members)
		groupMembers[g.Name] = members
	}

	for name := range groupMembers {
		visited := map[string]bool{}
		if v.hasCycle(name, groupMembers, visited) {
			errs = append(errs, ValidationError{
				Code:    "circular_reference",
				Level:   "error",
				Message: fmt.Sprintf("proxy group '%s' has circular reference", name),
			})
		}
	}
	return errs
}

func (v *Validator) hasCycle(name string, graph map[string][]string, visited map[string]bool) bool {
	if visited[name] {
		return true
	}
	visited[name] = true
	for _, member := range graph[name] {
		if _, isGroup := graph[member]; isGroup {
			if v.hasCycle(member, graph, visited) {
				return true
			}
		}
	}
	delete(visited, name)
	return false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/ -v -run TestValidate`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: validation service — reference integrity, empty groups, circular refs"
```

---

## Task 8: Import Service

**Files:**
- Create: `internal/service/import_service.go`
- Create: `internal/service/import_service_test.go`

- [ ] **Step 1: Write import test**

```go
// internal/service/import_service_test.go
package service

import (
	"testing"
)

func TestImportConfig(t *testing.T) {
	yamlContent := `
port: 7890
mixed-port: 7891
allow-lan: true
mode: rule
log-level: info
external-controller: 0.0.0.0:9090
proxies:
  - name: jp-01
    type: ss
    server: 1.2.3.4
    port: 443
    cipher: aes-256-gcm
    password: secret
proxy-groups:
  - name: Auto
    type: url-test
    proxies:
      - jp-01
    url: http://www.gstatic.com/generate_204
    interval: 300
rules:
  - DOMAIN-SUFFIX,google.com,Auto
  - MATCH,DIRECT
`
	svc := NewImportService()
	result, err := svc.ParseConfig([]byte(yamlContent))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(result.Proxies) != 1 {
		t.Errorf("expected 1 proxy, got %d", len(result.Proxies))
	}
	if result.Proxies[0].Name != "jp-01" {
		t.Errorf("expected proxy name=jp-01, got %s", result.Proxies[0].Name)
	}
	if len(result.ProxyGroups) != 1 {
		t.Errorf("expected 1 group, got %d", len(result.ProxyGroups))
	}
	if len(result.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(result.Rules))
	}
	if result.Rules[0].Type != "DOMAIN-SUFFIX" {
		t.Errorf("expected first rule type=DOMAIN-SUFFIX, got %s", result.Rules[0].Type)
	}
	if result.Rules[1].Type != "MATCH" {
		t.Errorf("expected second rule type=MATCH, got %s", result.Rules[1].Type)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/service/ -v -run TestImport`
Expected: FAIL

- [ ] **Step 3: Implement import service**

Parse mihomo YAML → decompose into structured models (proxies, groups, rules, system config). Key challenge: rule string parsing (`TYPE,PAYLOAD,TARGET[,PARAMS]`).

```go
// internal/service/import_service.go
package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/gejiliang/mihomo-cp/internal/model"
	"gopkg.in/yaml.v3"
)

type ImportResult struct {
	Proxies       []model.Proxy
	ProxyGroups   []model.ProxyGroup
	Rules         []model.Rule
	RuleProviders map[string]model.RuleProvider
	SystemConfig  json.RawMessage
}

type ImportService struct{}

func NewImportService() *ImportService { return &ImportService{} }

func (s *ImportService) ParseConfig(data []byte) (*ImportResult, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	result := &ImportResult{
		RuleProviders: make(map[string]model.RuleProvider),
	}

	// Extract proxies
	if proxiesRaw, ok := raw["proxies"].([]any); ok {
		for i, p := range proxiesRaw {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			name, _ := pm["name"].(string)
			ptype, _ := pm["type"].(string)
			delete(pm, "name")
			delete(pm, "type")
			config, _ := json.Marshal(pm)
			result.Proxies = append(result.Proxies, model.Proxy{
				ID:        uuid.New().String(),
				Name:      name,
				Type:      ptype,
				Config:    config,
				SortOrder: i,
			})
		}
	}

	// Extract proxy groups
	if groupsRaw, ok := raw["proxy-groups"].([]any); ok {
		for i, g := range groupsRaw {
			gm, ok := g.(map[string]any)
			if !ok {
				continue
			}
			name, _ := gm["name"].(string)
			gtype, _ := gm["type"].(string)
			members, _ := gm["proxies"].([]any)
			memberStrs := make([]string, 0, len(members))
			for _, m := range members {
				if s, ok := m.(string); ok {
					memberStrs = append(memberStrs, s)
				}
			}
			delete(gm, "name")
			delete(gm, "type")
			delete(gm, "proxies")
			config, _ := json.Marshal(gm)
			membersJSON, _ := json.Marshal(memberStrs)
			result.ProxyGroups = append(result.ProxyGroups, model.ProxyGroup{
				ID:        uuid.New().String(),
				Name:      name,
				Type:      gtype,
				Config:    config,
				Members:   membersJSON,
				SortOrder: i,
			})
		}
	}

	// Extract rules
	if rulesRaw, ok := raw["rules"].([]any); ok {
		for i, r := range rulesRaw {
			rStr, ok := r.(string)
			if !ok {
				continue
			}
			rule := s.parseRule(rStr, i)
			result.Rules = append(result.Rules, rule)
		}
	}

	// Extract rule-providers
	if rpRaw, ok := raw["rule-providers"].(map[string]any); ok {
		for name, v := range rpRaw {
			vm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			rpType, _ := vm["type"].(string)
			behavior, _ := vm["behavior"].(string)
			delete(vm, "type")
			delete(vm, "behavior")
			config, _ := json.Marshal(vm)
			result.RuleProviders[name] = model.RuleProvider{
				ID:       uuid.New().String(),
				Name:     name,
				Type:     rpType,
				Behavior: behavior,
				Config:   config,
			}
		}
	}

	// System config = everything except proxies, proxy-groups, rules, rule-providers
	sysConfig := make(map[string]any)
	for k, v := range raw {
		switch k {
		case "proxies", "proxy-groups", "rules", "rule-providers":
			continue
		default:
			sysConfig[k] = v
		}
	}
	result.SystemConfig, _ = json.Marshal(sysConfig)

	return result, nil
}

func (s *ImportService) parseRule(ruleStr string, order int) model.Rule {
	parts := strings.SplitN(ruleStr, ",", 4)
	rule := model.Rule{
		ID:        uuid.New().String(),
		SortOrder: order,
		Params:    json.RawMessage("{}"),
	}

	if len(parts) >= 1 {
		rule.Type = parts[0]
	}
	if rule.Type == "MATCH" {
		if len(parts) >= 2 {
			rule.Target = parts[1]
		}
		return rule
	}
	if len(parts) >= 2 {
		rule.Payload = parts[1]
	}
	if len(parts) >= 3 {
		rule.Target = parts[2]
	}
	if len(parts) >= 4 {
		params := map[string]any{}
		if parts[3] == "no-resolve" {
			params["no-resolve"] = true
		}
		rule.Params, _ = json.Marshal(params)
	}
	return rule
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/ -v -run TestImport`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: import service — parse existing mihomo config into structured data"
```

---

## Task 9: Publish Service + Mihomo Client

**Files:**
- Create: `internal/service/publish_service.go`
- Create: `internal/service/mihomo_client.go`

- [ ] **Step 1: Create mihomo client**

HTTP client wrapper for mihomo's external-controller API. Used for reload, runtime queries, and proxy switching.

```go
// internal/service/mihomo_client.go
package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type MihomoClient struct {
	baseURL string
	secret  string
	client  *http.Client
}

func NewMihomoClient(baseURL, secret string) *MihomoClient {
	return &MihomoClient{
		baseURL: baseURL,
		secret:  secret,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *MihomoClient) do(method, path string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mihomo API request failed: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mihomo API error %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

func (c *MihomoClient) ReloadConfig(configDir string) error {
	_, err := c.do("PUT", "/configs", map[string]string{"path": configDir})
	return err
}

func (c *MihomoClient) GetConnections() (json.RawMessage, error) {
	data, err := c.do("GET", "/connections", nil)
	return data, err
}

func (c *MihomoClient) CloseConnection(id string) error {
	_, err := c.do("DELETE", "/connections/"+id, nil)
	return err
}

func (c *MihomoClient) GetProxies() (json.RawMessage, error) {
	data, err := c.do("GET", "/proxies", nil)
	return data, err
}

func (c *MihomoClient) GetProxyDelay(name string, url string, timeout int) (json.RawMessage, error) {
	data, err := c.do("GET", fmt.Sprintf("/proxies/%s/delay?url=%s&timeout=%d", name, url, timeout), nil)
	return data, err
}

func (c *MihomoClient) SwitchProxy(group, proxy string) error {
	_, err := c.do("PUT", "/proxies/"+group, map[string]string{"name": proxy})
	return err
}

func (c *MihomoClient) GetRules() (json.RawMessage, error) {
	data, err := c.do("GET", "/rules", nil)
	return data, err
}

func (c *MihomoClient) GetProviders() (json.RawMessage, error) {
	data, err := c.do("GET", "/providers/rules", nil)
	return data, err
}

func (c *MihomoClient) RefreshProvider(name string) error {
	_, err := c.do("PUT", "/providers/rules/"+name, nil)
	return err
}

func (c *MihomoClient) GetVersion() (json.RawMessage, error) {
	data, err := c.do("GET", "/version", nil)
	return data, err
}
```

- [ ] **Step 2: Create publish service**

Orchestrates the full publish workflow: assemble YAML → validate with `mihomo -t` → backup → write → reload → rollback on failure.

```go
// internal/service/publish_service.go
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/store"
)

type PublishService struct {
	publishStore *store.PublishStore
	configSvc    *ConfigService
	validator    *Validator
	mihomo       *MihomoClient
}

func NewPublishService(
	ps *store.PublishStore,
	cs *ConfigService,
	v *Validator,
	mc *MihomoClient,
) *PublishService {
	return &PublishService{
		publishStore: ps,
		configSvc:    cs,
		validator:    v,
		mihomo:       mc,
	}
}

type PublishRequest struct {
	ConfigYAML  []byte
	ConfigPath  string // path to write YAML
	ConfigDir   string // mihomo working directory
	MihomoBin   string // path to mihomo binary
	Operator    string
	Note        string
}

func (s *PublishService) ValidateWithMihomo(yamlContent []byte, mihomoDir, mihomoBin string) (string, error) {
	// Write to temp file
	tmpDir, err := os.MkdirTemp("", "mihomo-cp-validate-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(tmpFile, yamlContent, 0644); err != nil {
		return "", fmt.Errorf("write temp config: %w", err)
	}

	cmd := exec.Command(mihomoBin, "-t", "-d", tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("validation failed: %s", string(output))
	}
	return string(output), nil
}

func (s *PublishService) Publish(req PublishRequest) (*model.PublishRecord, error) {
	// Get next version number
	version, err := s.publishStore.NextVersion()
	if err != nil {
		return nil, fmt.Errorf("get next version: %w", err)
	}

	record := &model.PublishRecord{
		ID:         uuid.New().String(),
		Version:    version,
		ConfigYAML: string(req.ConfigYAML),
		Operator:   req.Operator,
		Note:       req.Note,
		CreatedAt:  time.Now(),
	}

	// Generate diff
	currentYAML, _ := os.ReadFile(req.ConfigPath)
	record.DiffText = generateDiff(string(currentYAML), string(req.ConfigYAML))

	// Backup current config
	backup := make([]byte, len(currentYAML))
	copy(backup, currentYAML)

	// Write new config
	if err := os.WriteFile(req.ConfigPath, req.ConfigYAML, 0644); err != nil {
		record.Status = "failed"
		record.ErrorMsg = fmt.Sprintf("write config: %v", err)
		s.publishStore.Create(record)
		return record, fmt.Errorf("write config: %w", err)
	}

	// Reload mihomo
	if err := s.mihomo.ReloadConfig(req.ConfigDir); err != nil {
		// Rollback
		os.WriteFile(req.ConfigPath, backup, 0644)
		s.mihomo.ReloadConfig(req.ConfigDir) // best effort reload with old config

		record.Status = "failed"
		record.ErrorMsg = fmt.Sprintf("reload failed, rolled back: %v", err)
		s.publishStore.Create(record)
		return record, fmt.Errorf("reload failed: %w", err)
	}

	record.Status = "success"
	s.publishStore.Create(record)
	return record, nil
}

func (s *PublishService) Rollback(configPath, configDir string, operator string) (*model.PublishRecord, error) {
	prev, err := s.publishStore.GetLastSuccess()
	if err != nil {
		return nil, fmt.Errorf("no previous successful version to rollback to")
	}

	return s.Publish(PublishRequest{
		ConfigYAML: []byte(prev.ConfigYAML),
		ConfigPath: configPath,
		ConfigDir:  configDir,
		Operator:   operator,
		Note:       fmt.Sprintf("rollback to version %d", prev.Version),
	})
}

func generateDiff(old, new string) string {
	// Simple line-by-line diff indication. A full unified diff library
	// can be added later; for MVP this shows the two configs.
	if old == "" {
		return "(initial publish)"
	}
	if old == new {
		return "(no changes)"
	}
	return fmt.Sprintf("--- old config\n+++ new config\n(diff available in UI)")
}
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./...`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat: publish service + mihomo API client"
```

---

## Task 10: All Handlers — CRUD + Publish + Runtime + Import

**Files:**
- Create: `internal/handler/proxy.go`
- Create: `internal/handler/proxy_group.go`
- Create: `internal/handler/rule.go`
- Create: `internal/handler/rule_provider.go`
- Create: `internal/handler/system_config.go`
- Create: `internal/handler/publish.go`
- Create: `internal/handler/runtime.go`
- Create: `internal/handler/import.go`
- Create: `internal/handler/settings.go`
- Modify: `internal/server/server.go` (register all routes)

- [ ] **Step 1: Create proxy handler**

Each handler follows the pattern: parse request → call store → return JSON. Includes reference checking on delete.

```go
// internal/handler/proxy.go — standard CRUD pattern
// ProxyHandler with List, Get, Create, Update, Delete, Copy, Reorder, Refs methods
```

- [ ] **Step 2: Create remaining handlers**

`proxy_group.go`, `rule.go`, `rule_provider.go`, `system_config.go` all follow the same CRUD pattern.

- [ ] **Step 3: Create publish handler**

Preview (render YAML + diff), Validate (run mihomo -t), Publish, Rollback, History endpoints. Wires together config_service, validator, and publish_service.

- [ ] **Step 4: Create runtime handler**

Thin proxy layer: each endpoint calls `MihomoClient` and forwards the response. WebSocket proxy for logs/traffic/memory streams.

- [ ] **Step 5: Create import handler**

`POST /api/import/config` reads the mihomo config file, calls ImportService, inserts entities into stores.

- [ ] **Step 6: Create settings handler**

CRUD for app settings and user management.

- [ ] **Step 7: Wire all routes in server.go**

Register all route patterns with appropriate middleware (auth, admin-only, etc.).

```go
func (s *Server) registerRoutes() {
    // Public
    s.mux.HandleFunc("POST /api/auth/login", s.authHandler.Login)
    s.mux.HandleFunc("POST /api/auth/refresh", s.authHandler.Refresh)

    // Protected (wrap with auth middleware)
    protected := s.authMiddleware(http.NewServeMux())
    // ... all CRUD routes
    // Admin-only routes wrap with RequireAdmin

    // Static files (embedded frontend)
    s.mux.Handle("/", http.FileServer(http.FS(webFS)))
}
```

- [ ] **Step 8: Verify full compilation**

Run: `go build ./cmd/mihomo-cp`
Expected: Builds successfully

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat: all API handlers — CRUD, publish, runtime, import, settings"
```

---

## Task 11: Frontend Scaffolding

**Files:**
- Create: `web/` (entire Vite + React project)

- [ ] **Step 1: Scaffold Vite React TypeScript project**

```bash
cd /Users/gejiliang/Projects/homelab
pnpm create vite web --template react-ts
cd web
pnpm install
```

- [ ] **Step 2: Install dependencies**

```bash
cd /Users/gejiliang/Projects/homelab/web
pnpm add react-router-dom zustand ky @dnd-kit/core @dnd-kit/sortable @dnd-kit/utilities
pnpm add -D tailwindcss @tailwindcss/vite
```

- [ ] **Step 3: Configure Tailwind**

Update `vite.config.ts` to add Tailwind plugin and API proxy. Update `src/index.css` with `@import "tailwindcss"`.

- [ ] **Step 4: Initialize shadcn/ui**

```bash
cd /Users/gejiliang/Projects/homelab/web
pnpm dlx shadcn@latest init -d
```

- [ ] **Step 5: Add core shadcn components**

```bash
cd /Users/gejiliang/Projects/homelab/web
pnpm dlx shadcn@latest add button input label card dialog table badge select textarea tabs toast dropdown-menu separator scroll-area sheet alert
```

- [ ] **Step 6: Create API client with auth interceptor**

```typescript
// web/src/api/client.ts
import ky from 'ky';
import { useAuthStore } from '../stores/auth';

export const api = ky.create({
  prefixUrl: '/api',
  hooks: {
    beforeRequest: [
      (request) => {
        const token = useAuthStore.getState().accessToken;
        if (token) {
          request.headers.set('Authorization', `Bearer ${token}`);
        }
      },
    ],
    afterResponse: [
      async (_request, _options, response) => {
        if (response.status === 401) {
          useAuthStore.getState().logout();
          window.location.href = '/login';
        }
      },
    ],
  },
});
```

- [ ] **Step 7: Create auth store**

```typescript
// web/src/stores/auth.ts
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface AuthState {
  accessToken: string | null;
  refreshToken: string | null;
  user: { id: string; username: string; role: string } | null;
  setTokens: (access: string, refresh: string) => void;
  setUser: (user: AuthState['user']) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      accessToken: null,
      refreshToken: null,
      user: null,
      setTokens: (access, refresh) => set({ accessToken: access, refreshToken: refresh }),
      setUser: (user) => set({ user }),
      logout: () => set({ accessToken: null, refreshToken: null, user: null }),
    }),
    { name: 'auth-storage' }
  )
);
```

- [ ] **Step 8: Verify dev server starts**

Run: `cd /Users/gejiliang/Projects/homelab/web && pnpm dev &; sleep 3; curl -s http://localhost:5173 | head -5; kill %1`
Expected: HTML response from Vite dev server

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat: frontend scaffolding — Vite, React, Tailwind, shadcn/ui, API client"
```

---

## Task 12: Frontend Layout + Routing + Login

**Files:**
- Create: `web/src/routes.tsx`
- Create: `web/src/pages/login.tsx`
- Create: `web/src/components/layout/sidebar.tsx`
- Create: `web/src/components/layout/header.tsx`
- Create: `web/src/components/layout/app-layout.tsx`
- Create: `web/src/api/auth.ts`
- Modify: `web/src/App.tsx`
- Create placeholder pages for all routes

- [ ] **Step 1: Create login page**

Full-screen centered login form with username/password. On submit, calls `POST /api/auth/login`, stores tokens, redirects to `/`.

- [ ] **Step 2: Create sidebar**

Navigation links matching the spec: Overview, Proxies, Proxy Groups, Rules, Rule Providers, System Config, Publish, Runtime, Settings. Active state based on current route. Collapsible on mobile.

- [ ] **Step 3: Create header**

Top bar showing: mihomo status indicator, draft changes badge, quick publish button, user dropdown (logout).

- [ ] **Step 4: Create app layout**

Combines sidebar + header + content area. Wraps all authenticated routes.

- [ ] **Step 5: Set up routing**

```typescript
// web/src/routes.tsx
const router = createBrowserRouter([
  { path: '/login', element: <LoginPage /> },
  {
    path: '/',
    element: <ProtectedRoute><AppLayout /></ProtectedRoute>,
    children: [
      { index: true, element: <OverviewPage /> },
      { path: 'proxies', element: <ProxiesPage /> },
      { path: 'proxy-groups', element: <ProxyGroupsPage /> },
      { path: 'rules', element: <RulesPage /> },
      { path: 'rule-providers', element: <RuleProvidersPage /> },
      { path: 'system-config', element: <SystemConfigPage /> },
      { path: 'publish', element: <PublishPage /> },
      { path: 'runtime', element: <RuntimePage /> },
      { path: 'settings', element: <SettingsPage /> },
    ],
  },
]);
```

- [ ] **Step 6: Create placeholder pages**

Each page is a simple component with the page title. Will be fleshed out in subsequent tasks.

- [ ] **Step 7: Verify routing works**

Run dev server, navigate to `/login`, verify login form renders. Log in with default admin/admin, verify redirect to overview with sidebar.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat: frontend layout — sidebar, header, routing, login page"
```

---

## Task 13: Frontend — Proxy Management Page

**Files:**
- Create: `web/src/api/proxies.ts`
- Create: `web/src/pages/proxies.tsx`
- Create: `web/src/components/proxies/proxy-form.tsx`
- Create: `web/src/components/proxies/proxy-list.tsx`
- Create: `web/src/components/shared/data-table.tsx`
- Create: `web/src/components/shared/confirm-dialog.tsx`

- [ ] **Step 1: Create proxies API module**

```typescript
// web/src/api/proxies.ts
import { api } from './client';

export interface Proxy {
  id: string;
  name: string;
  type: string;
  config: Record<string, any>;
  sort_order: number;
}

export const proxiesApi = {
  list: (params?: { search?: string; type?: string }) =>
    api.get('proxies', { searchParams: params }).json<{ data: Proxy[] }>(),
  get: (id: string) => api.get(`proxies/${id}`).json<{ data: Proxy }>(),
  create: (data: Partial<Proxy>) => api.post('proxies', { json: data }).json<{ data: Proxy }>(),
  update: (id: string, data: Partial<Proxy>) => api.put(`proxies/${id}`, { json: data }).json<{ data: Proxy }>(),
  delete: (id: string) => api.delete(`proxies/${id}`).json(),
  copy: (id: string) => api.post(`proxies/${id}/copy`).json<{ data: Proxy }>(),
  reorder: (ids: string[]) => api.post('proxies/reorder', { json: { ids } }).json(),
  refs: (id: string) => api.get(`proxies/${id}/refs`).json<{ data: string[] }>(),
};
```

- [ ] **Step 2: Create reusable data table component**

Sortable, filterable table with shadcn/ui Table. Supports search input, type filter dropdown, and action buttons per row.

- [ ] **Step 3: Create proxy form with protocol-dynamic fields**

Dialog form that shows/hides fields based on selected protocol type. Common fields (name, server, port) always shown. Protocol-specific fields rendered dynamically from a field definition map.

Protocol field definitions:
- `ss`: cipher, password, udp, plugin, plugin-opts
- `trojan`: password, sni, alpn, skip-cert-verify
- `vmess`: uuid, alterId, cipher, tls, servername, network, ws-opts
- `vless`: uuid, flow, tls, servername, network, reality-opts
- `hysteria2`: password, obfs, obfs-password, ports
- `tuic`: uuid, password, congestion-controller, alpn

- [ ] **Step 4: Create proxy list page**

Full page with: search bar, type filter, add button, data table showing all proxies. Each row has edit/copy/delete actions. Delete shows reference check before confirming.

- [ ] **Step 5: Wire page into routes**

Replace placeholder with real ProxiesPage component.

- [ ] **Step 6: Test with running backend**

Start both backend and frontend dev servers. Create, edit, delete a proxy through the UI.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: proxy management UI — list, create, edit, delete, copy"
```

---

## Task 14: Frontend — Proxy Groups Page

**Files:**
- Create: `web/src/api/proxy-groups.ts`
- Create: `web/src/pages/proxy-groups.tsx`
- Create: `web/src/components/proxy-groups/group-form.tsx`
- Create: `web/src/components/proxy-groups/group-member-list.tsx`

- [ ] **Step 1: Create proxy groups API module**

Same pattern as proxies API.

- [ ] **Step 2: Create drag-sortable member list component**

Uses `@dnd-kit/sortable` for reordering group members. Shows member name + type indicator. Supports adding members from a dropdown (lists all proxies + other groups).

- [ ] **Step 3: Create group form**

Dialog form with: name, type (select/fallback/url-test/load-balance/relay), type-specific config fields (url, interval for url-test/fallback), and embedded member list component.

- [ ] **Step 4: Create proxy groups page**

Table showing all groups with: name, type, member count, current selected proxy (from runtime). Edit/delete with reference checking.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: proxy group management UI — drag-sortable members, type-specific forms"
```

---

## Task 15: Frontend — Rules Page

**Files:**
- Create: `web/src/api/rules.ts`
- Create: `web/src/pages/rules.tsx`
- Create: `web/src/components/rules/rule-form.tsx`
- Create: `web/src/components/rules/rule-list.tsx`

- [ ] **Step 1: Create rules API module**

- [ ] **Step 2: Create rule form**

Dialog with: type dropdown (all valid rule types), payload input (dynamic label based on type), target dropdown (populated from proxy groups + DIRECT/REJECT), params checkboxes (no-resolve).

- [ ] **Step 3: Create drag-sortable rule list**

Full-page list with drag-sort capability using `@dnd-kit`. Each row shows: order number, type badge, payload, target, params. Search and filter by type.

- [ ] **Step 4: MATCH rule handling**

MATCH rule is always pinned to the bottom. Cannot be reordered. Only one MATCH rule allowed. If missing, show a warning banner suggesting to add one.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: rule management UI — drag-sortable list, type-aware forms"
```

---

## Task 16: Frontend — Rule Providers + System Config Pages

**Files:**
- Create: `web/src/api/rule-providers.ts`
- Create: `web/src/api/system-config.ts`
- Create: `web/src/pages/rule-providers.tsx`
- Create: `web/src/pages/system-config.tsx`

- [ ] **Step 1: Create rule providers page**

Table with: name, type (file/http), behavior, last update time. Create/edit dialog with type-specific fields. Refresh button for HTTP providers.

- [ ] **Step 2: Create system config page**

Form-based page organized in sections:
- General: port, socks-port, mixed-port, allow-lan, bind-address, mode, log-level, ipv6
- External Controller: external-controller, secret
- TUN: enable, stack, dns-hijack, auto-route
- DNS: enable, listen, enhanced-mode, nameserver, fallback
- Listeners: list editor

Each section is a collapsible card. Save button applies to draft.

- [ ] **Step 3: Add YAML text mode toggle**

Each config section supports switching between form mode and raw YAML mode. Uses CodeMirror 6 for YAML editing.

```bash
pnpm add @codemirror/lang-yaml @codemirror/view @codemirror/state codemirror
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat: rule providers + system config UI with dual-mode editing"
```

---

## Task 17: Frontend — Publish Center

**Files:**
- Create: `web/src/api/publish.ts`
- Create: `web/src/pages/publish.tsx`
- Create: `web/src/components/publish/publish-preview.tsx`
- Create: `web/src/components/publish/history-list.tsx`
- Create: `web/src/components/shared/diff-viewer.tsx`
- Create: `web/src/stores/draft.ts`

- [ ] **Step 1: Create draft status store**

Zustand store that tracks whether there are unpublished changes. Polls `/api/publish/status` periodically to check draft vs running config.

- [ ] **Step 2: Create diff viewer component**

Side-by-side diff display showing old (running) vs new (draft) config. Highlights added/removed/changed lines.

- [ ] **Step 3: Create publish preview component**

Shows: rendered YAML preview, diff view, validation status, optional publish note input. "Validate" button runs `mihomo -t` check. "Publish" button triggers publish.

- [ ] **Step 4: Create history list component**

Table showing all publish records: version, time, operator, status (success/failed/rolled_back), note. Click to expand and view full YAML. Rollback button on previous successful versions.

- [ ] **Step 5: Create publish page**

Combines preview + history. Top section: current draft status + publish controls. Bottom section: publish history timeline.

- [ ] **Step 6: Wire draft badge into header**

Header shows "N changes pending" badge that links to publish page.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: publish center UI — preview, diff, validate, publish, history, rollback"
```

---

## Task 18: Frontend — Runtime Observation

**Files:**
- Create: `web/src/api/runtime.ts`
- Create: `web/src/pages/runtime.tsx`
- Create: `web/src/components/runtime/connection-table.tsx`
- Create: `web/src/components/runtime/log-viewer.tsx`

- [ ] **Step 1: Create runtime API module**

Includes WebSocket connections for logs, traffic, and memory streams.

- [ ] **Step 2: Create connection table**

Real-time connection list from mihomo. Columns: host, network, type, chains (proxy chain), rule, download/upload speed, time. Search and filter. Close connection button.

- [ ] **Step 3: Create log viewer**

Streaming log display using WebSocket. Auto-scroll, level filter (debug/info/warning/error), search. Color-coded by level.

- [ ] **Step 4: Create runtime page**

Tabs layout:
- Connections: live connection table
- Logs: streaming log viewer
- Proxies: runtime proxy status with delay test buttons
- Rules: runtime rule list with match count
- Providers: rule provider status with refresh buttons

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: runtime observation UI — connections, logs, proxies, rules"
```

---

## Task 19: Frontend — Overview + Settings Pages

**Files:**
- Create: `web/src/pages/overview.tsx`
- Create: `web/src/pages/settings.tsx`
- Create: `web/src/api/settings.ts`

- [ ] **Step 1: Create overview dashboard**

Summary cards showing:
- mihomo status (version, uptime, mode)
- Total proxies / groups / rules count
- Active connections count
- Traffic stats (upload/download)
- Recent publish history (last 5)
- Validation warnings count

- [ ] **Step 2: Create settings page**

Two sections:
- App Settings: mihomo config path, binary path, external controller URL, secret
- User Management: list users, create/edit/delete (admin only)

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat: overview dashboard + settings page"
```

---

## Task 20: Go Embed Frontend + Docker + Final Integration

**Files:**
- Modify: `internal/server/server.go` (add `embed.FS` static file serving)
- Create: `web/embed.go`
- Create: `Dockerfile`
- Modify: `Makefile`

- [ ] **Step 1: Create frontend embed**

```go
//go:build !dev

package web

import "embed"

//go:embed dist/*
var DistFS embed.FS
```

And a dev fallback:
```go
//go:build dev

package web

import "embed"

var DistFS embed.FS
```

- [ ] **Step 2: Update server to serve embedded frontend**

Serve API routes under `/api/`, serve embedded frontend for all other paths. Handle SPA client-side routing (fallback to `index.html`).

- [ ] **Step 3: Build frontend**

```bash
cd /Users/gejiliang/Projects/homelab/web
pnpm build
```

- [ ] **Step 4: Build full binary**

```bash
cd /Users/gejiliang/Projects/homelab
go build -o mihomo-cp ./cmd/mihomo-cp
```

- [ ] **Step 5: Create Dockerfile**

Multi-stage build: Node → build frontend, Go → build backend with embedded frontend, Alpine → minimal runtime.

- [ ] **Step 6: Update Makefile**

```makefile
.PHONY: build dev test clean

build: build-frontend build-backend

build-frontend:
	cd web && pnpm install --frozen-lockfile && pnpm build

build-backend:
	go build -o mihomo-cp ./cmd/mihomo-cp

dev:
	cd web && pnpm dev &
	go run -tags dev ./cmd/mihomo-cp

test:
	go test ./... -v

clean:
	rm -f mihomo-cp
	rm -rf web/dist
```

- [ ] **Step 7: Smoke test full binary**

```bash
./mihomo-cp --port 8080 &
sleep 2
curl -s http://localhost:8080/ | head -3  # Should return HTML
curl -s http://localhost:8080/api/health   # Should return JSON
kill %1
```

- [ ] **Step 8: Final commit**

```bash
git add -A
git commit -m "feat: production build — embedded frontend, Dockerfile, Makefile"
```

---

## Summary

| Task | Description | Key Deliverable |
|------|-------------|----------------|
| 1 | Go backend scaffolding | Health endpoint, project structure |
| 2 | SQLite + migrations | Database layer with all tables |
| 3 | Auth system | JWT login, middleware, auto-admin |
| 4 | Core models | All domain model structs |
| 5 | Store layer | CRUD for all entities |
| 6 | Config assembly | Draft → mihomo YAML rendering |
| 7 | Validation service | Reference integrity, structural checks |
| 8 | Import service | Existing mihomo config → structured data |
| 9 | Publish + mihomo client | Publish workflow, runtime API client |
| 10 | All handlers | Complete REST API |
| 11 | Frontend scaffolding | Vite, React, Tailwind, shadcn/ui |
| 12 | Layout + routing + login | Sidebar, header, auth flow |
| 13 | Proxies page | Protocol-dynamic CRUD UI |
| 14 | Proxy groups page | Drag-sortable member management |
| 15 | Rules page | Drag-sortable rule list |
| 16 | Providers + system config | Dual-mode editing |
| 17 | Publish center | Preview, diff, validate, publish, rollback |
| 18 | Runtime observation | Connections, logs, proxies, rules |
| 19 | Overview + settings | Dashboard, user management |
| 20 | Production build | Embedded frontend, Docker, Makefile |
