# Mihomo Control Plane - Technical Design Spec

- Version: v1.1
- Date: 2026-03-25
- Status: Implemented
- Source PRD: `PRD-mihomo-control-plane.md`

## 1. System Overview

A single-binary web management console for locally deployed mihomo instances. Provides configuration editing, publishing with validation/rollback, and runtime observability — all without requiring SSH or manual YAML editing.

### 1.1 Deployment Model

```
┌─────────────────────────────────────────────┐
│                  Host Machine               │
│                                             │
│  ┌──────────────────┐  ┌─────────────────┐  │
│  │  Mihomo Control  │  │    mihomo core   │  │
│  │     Plane        │  │                  │  │
│  │                  │  │  :9090 ext-ctrl  │  │
│  │  :8080 Web UI    │──│  :7890 proxy     │  │
│  │                  │  │  :7891 socks     │  │
│  │  SQLite DB       │  │                  │  │
│  │  Config Files    │  │  config.yaml     │  │
│  └──────────────────┘  └─────────────────┘  │
└─────────────────────────────────────────────┘
```

- MVP: same-host deployment only
- Control Plane reads/writes mihomo config files directly
- Validates via `mihomo -t -d <config-dir>`
- Controls runtime via mihomo's external-controller RESTful API
- GeoIP detection via mihomo SOCKS proxy (switches GLOBAL selector, queries ip-api.com)
- Phase 3: extend to remote multi-instance management

### 1.2 Key Design Principles

1. **mihomo config is the source of truth** — Control Plane never introduces abstractions that diverge from native mihomo YAML structure
2. **Draft-first editing** — all changes go to draft; only validated drafts can be published
3. **Zero external dependencies** — single Go binary with embedded frontend and SQLite
4. **Non-destructive** — Control Plane failure must never affect a running mihomo instance

## 2. Tech Stack

| Layer | Technology | Rationale |
|-------|-----------|-----------|
| Backend | Go 1.25+ | Single binary, `embed.FS`, excellent YAML/JSON handling |
| HTTP Router | `net/http` (stdlib) | No framework dependency, Go 1.22+ routing is sufficient |
| Database | SQLite via `modernc.org/sqlite` | Pure Go, no CGO, embedded |
| ORM | `jmoiron/sqlx` | Lightweight, no magic |
| YAML | `gopkg.in/yaml.v3` | mihomo config parsing/rendering |
| Frontend | React 19 + TypeScript | Component model, ecosystem |
| Build | Vite | Fast dev server, optimized builds |
| UI | shadcn/ui + Tailwind CSS 4 | Modern, customizable, tree-shakeable |
| State | Zustand | Lightweight, no boilerplate |
| HTTP Client | `ky` | Lightweight fetch wrapper |
| Drag & Drop | `@dnd-kit/core` | Accessible, performant |
| Auth | JWT (access + refresh tokens) | Stateless, simple |

## 3. Architecture

### 3.1 Backend Architecture

```
cmd/
  mihomo-cp/
    main.go              # Entry point, flag parsing, startup

internal/
  server/
    server.go            # HTTP server setup, middleware chain
    routes.go            # Route registration

  handler/
    auth.go              # Login, token refresh
    proxy.go             # Proxy node CRUD
    proxy_group.go       # Proxy group CRUD
    rule.go              # Rule CRUD
    rule_provider.go     # Rule provider CRUD
    system_config.go     # System config (ports, DNS, tun, etc.)
    publish.go           # Draft → validate → publish → rollback
    runtime.go           # Runtime observation (connections, logs, delays)

  model/
    proxy.go             # Proxy node types
    proxy_group.go       # Proxy group types
    rule.go              # Rule types
    rule_provider.go     # Rule provider types
    config.go            # Full mihomo config model
    publish.go           # Publish record, diff
    user.go              # User model

  store/
    db.go                # SQLite connection, migrations
    proxy_store.go       # Proxy node persistence
    proxy_group_store.go # Proxy group persistence
    rule_store.go        # Rule persistence
    rule_provider_store.go
    config_store.go      # System config persistence
    publish_store.go     # Publish history
    user_store.go        # User persistence

  service/
    config_service.go    # Config assembly: struct → YAML rendering
    publish_service.go   # Publish workflow: validate → backup → write → reload
    mihomo_client.go     # HTTP client for mihomo external-controller API
    validator.go         # Config validation (reference checks, schema validation)
    import_service.go    # Import existing mihomo config YAML → structured data
    geoip_service.go     # GeoIP detection via mihomo SOCKS proxy (auto country detection)

  middleware/
    auth.go              # JWT verification
    logging.go           # Request logging
    cors.go              # CORS headers

web/
  dist/                  # Embedded frontend build output (go:embed)
```

### 3.2 Frontend Architecture

```
web/
  src/
    main.tsx
    App.tsx
    routes.tsx            # React Router config

    api/
      client.ts           # API client (ky instance with auth interceptor)
      proxies.ts           # Proxy node API
      proxy-groups.ts      # Proxy group API
      rules.ts             # Rule API
      rule-providers.ts    # Rule provider API
      system-config.ts     # System config API
      publish.ts           # Publish API
      runtime.ts           # Runtime API
      auth.ts              # Auth API

    stores/
      auth.ts              # Auth state (Zustand)
      draft.ts             # Draft dirty state tracking

    pages/
      login.tsx
      overview.tsx         # Dashboard overview
      proxies.tsx          # Proxy node management
      proxy-groups.tsx     # Proxy group management
      rules.tsx            # Rule management
      rule-providers.tsx   # Rule provider management
      system-config.tsx    # DNS, ports, tun, listeners
      publish.tsx          # Publish center
      runtime.tsx          # Runtime observation
      settings.tsx         # System settings (users, etc.)

    components/
      layout/
        sidebar.tsx        # Navigation sidebar
        header.tsx         # Top bar with status + publish button
      shared/
        data-table.tsx     # Reusable sortable/filterable table
        form-field.tsx     # Dynamic form field renderer
        yaml-editor.tsx    # Monaco-based YAML editor (text mode)
        diff-viewer.tsx    # Side-by-side diff display
        confirm-dialog.tsx
      proxies/
        proxy-form.tsx     # Dynamic form by protocol type
        proxy-list.tsx
      proxy-groups/
        group-form.tsx
        group-member-list.tsx  # Drag-sortable member list
      rules/
        rule-form.tsx
        rule-list.tsx          # Drag-sortable rule list
      runtime/
        connection-table.tsx
        log-viewer.tsx         # Streaming log display
        rule-match-viewer.tsx
```

### 3.3 Navigation Structure

```
Sidebar:
  ├── Overview          # Active connections + proxy latency dashboard
  ├── Proxies           # Node management (with country filter & batch delay test)
  ├── Proxy Groups      # Strategy group management
  ├── Rules             # Rule management
  ├── Rule Providers    # Rule provider management
  ├── System Config     # DNS, ports, tun, listeners
  ├── Publish           # Publish center (with badge for pending changes)
  ├── Runtime           # Runtime observation
  └── Settings          # Users, mihomo path config
```

## 4. Data Model

### 4.1 Database Schema

```sql
-- Proxy nodes (draft state)
CREATE TABLE proxies (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL,  -- ss, trojan, vmess, vless, http, socks5, hysteria2, tuic
    country     TEXT NOT NULL DEFAULT '',  -- ISO 3166-1 alpha-2 (auto-detected via GeoIP)
    config      TEXT NOT NULL,  -- JSON: protocol-specific fields
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Proxy groups (draft state)
CREATE TABLE proxy_groups (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL,  -- select, fallback, url-test, load-balance, relay
    config      TEXT NOT NULL,  -- JSON: type-specific fields (url, interval, etc.)
    members     TEXT NOT NULL,  -- JSON array: ordered list of proxy/group names
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Rules (draft state)
CREATE TABLE rules (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL,  -- DOMAIN, DOMAIN-SUFFIX, IP-CIDR, RULE-SET, MATCH, etc.
    payload     TEXT NOT NULL,  -- The match value
    target      TEXT NOT NULL,  -- Proxy group name or DIRECT/REJECT
    params      TEXT,           -- JSON: optional params (no-resolve, etc.)
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Rule providers (draft state)
CREATE TABLE rule_providers (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL,  -- file, http
    behavior    TEXT NOT NULL,  -- domain, ipcidr, classical
    config      TEXT NOT NULL,  -- JSON: url, path, interval, etc.
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- System config (draft state, single row)
CREATE TABLE system_config (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    config      TEXT NOT NULL,  -- JSON: all system-level config fields
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Publish history
CREATE TABLE publish_history (
    id          TEXT PRIMARY KEY,
    version     INTEGER NOT NULL,
    config_yaml TEXT NOT NULL,     -- The full rendered YAML that was published
    diff_text   TEXT,              -- Diff from previous version
    status      TEXT NOT NULL,     -- success, failed, rolled_back
    error_msg   TEXT,              -- Error message if failed
    operator    TEXT NOT NULL,     -- Username who published
    note        TEXT,              -- User-provided publish note
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Users
CREATE TABLE users (
    id          TEXT PRIMARY KEY,
    username    TEXT NOT NULL UNIQUE,
    password    TEXT NOT NULL,     -- bcrypt hash
    role        TEXT NOT NULL DEFAULT 'admin',  -- admin, readonly
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### 4.2 Config Assembly Pipeline

```
Draft State (SQLite)
    │
    ├── proxies table      → config.proxies[]
    ├── proxy_groups table → config.proxy-groups[]
    ├── rules table        → config.rules[]
    ├── rule_providers     → config.rule-providers{}
    └── system_config      → config.port, config.dns, config.tun, etc.
    │
    ▼
Config Assembly (config_service.go)
    │
    ▼
Full mihomo YAML (in memory)
    │
    ├──→ Validation: `mihomo -t` on temp file
    ├──→ Diff: compare with current running config
    │
    ▼
Publish (publish_service.go)
    │
    ├── 1. Backup current config → publish_history
    ├── 2. Write new YAML to mihomo config path
    ├── 3. Reload mihomo via external-controller PUT /configs
    ├── 4. If reload fails → auto-rollback (restore backup)
    └── 5. Record result in publish_history
```

### 4.3 Import Flow (First Run)

On first launch or user-triggered import:
1. Read existing mihomo `config.yaml`
2. Parse YAML into structured model
3. Decompose into proxies, proxy_groups, rules, rule_providers, system_config
4. Insert into SQLite tables
5. Mark as "imported — not yet published through Control Plane"

This ensures users don't start from scratch.

## 5. API Design

### 5.1 Authentication

```
POST   /api/auth/login          # { username, password } → { access_token, refresh_token }
POST   /api/auth/refresh         # { refresh_token } → { access_token }
GET    /api/auth/me              # Current user info
```

### 5.2 Proxy Nodes

```
GET    /api/proxies              # List all (with filters: ?type=ss&search=jp)
POST   /api/proxies              # Create
GET    /api/proxies/:id          # Get one
PUT    /api/proxies/:id          # Update
DELETE /api/proxies/:id          # Delete (with reference check)
POST   /api/proxies/:id/copy     # Duplicate
POST   /api/proxies/reorder      # Batch reorder: { ids: [...] }
GET    /api/proxies/:id/refs     # What groups reference this proxy
POST   /api/proxies/detect-countries  # Bulk country detection via mihomo SOCKS proxy
```

> **Note**: Country detection is also triggered automatically (async) when creating or updating a proxy.

### 5.3 Proxy Groups

```
GET    /api/proxy-groups         # List all
POST   /api/proxy-groups         # Create
GET    /api/proxy-groups/:id     # Get one
PUT    /api/proxy-groups/:id     # Update
DELETE /api/proxy-groups/:id     # Delete (with reference check)
POST   /api/proxy-groups/reorder # Batch reorder
GET    /api/proxy-groups/:id/refs # What rules/groups reference this group
```

### 5.4 Rules

```
GET    /api/rules                # List all (with filters: ?type=DOMAIN&search=openai)
POST   /api/rules                # Create
GET    /api/rules/:id            # Get one
PUT    /api/rules/:id            # Update
DELETE /api/rules/:id            # Delete
POST   /api/rules/reorder        # Batch reorder: { ids: [...] }
```

### 5.5 Rule Providers

```
GET    /api/rule-providers       # List all
POST   /api/rule-providers       # Create
GET    /api/rule-providers/:id   # Get one
PUT    /api/rule-providers/:id   # Update
DELETE /api/rule-providers/:id   # Delete
POST   /api/rule-providers/:id/refresh  # Refresh remote provider
```

### 5.6 System Config

```
GET    /api/system-config        # Get current draft config
PUT    /api/system-config        # Update draft config
```

### 5.7 Publish

```
GET    /api/publish/preview      # Render current draft → YAML + diff
POST   /api/publish/validate     # Run mihomo -t on rendered config
POST   /api/publish              # Publish: validate → backup → write → reload
POST   /api/publish/discard      # Discard draft: re-import running config, reset has_changes
POST   /api/publish/rollback     # Rollback to previous version
GET    /api/publish/history      # List publish history
GET    /api/publish/history/:id  # Get specific publish record with YAML
GET    /api/publish/status       # Current publish state (draft clean/dirty, running version)
```

### 5.8 Runtime (Proxy to mihomo external-controller)

```
GET    /api/runtime/connections      # → mihomo GET /connections
DELETE /api/runtime/connections/:id  # → mihomo DELETE /connections/:id
GET    /api/runtime/proxies          # → mihomo GET /proxies (with delays)
GET    /api/runtime/proxies/:name/delay  # → mihomo GET /proxies/:name/delay
PUT    /api/runtime/proxies/:group/selected  # → mihomo PUT /proxies/:name (switch selected)
GET    /api/runtime/rules            # → mihomo GET /rules
GET    /api/runtime/providers/rules  # → mihomo GET /providers/rules
PUT    /api/runtime/providers/rules/:name  # → mihomo PUT /providers/rules/:name (refresh)
GET    /api/runtime/logs             # WebSocket → mihomo WS /logs
GET    /api/runtime/traffic          # WebSocket → mihomo WS /traffic
GET    /api/runtime/memory           # WebSocket → mihomo WS /memory
```

### 5.9 Import

```
POST   /api/import/config        # Import from mihomo config file path
POST   /api/import/detect        # Auto-detect mihomo config location
```

### 5.10 Settings

```
GET    /api/settings             # Get app settings (mihomo path, etc.)
PUT    /api/settings             # Update app settings
GET    /api/users                # List users (admin only)
POST   /api/users                # Create user (admin only)
PUT    /api/users/:id            # Update user (admin only)
DELETE /api/users/:id            # Delete user (admin only)
```

## 6. Key Workflows

### 6.1 Publish Workflow

```
User edits config (draft)
         │
         ▼
    Click "Publish"
         │
         ▼
    Render YAML from draft
         │
         ▼
    Show diff (draft vs running)
         │
         ▼
    Run `mihomo -t -d <tmpdir>`  ──failed──→ Show errors, block publish
         │ passed
         ▼
    User confirms + optional note
         │
         ▼
    Backup current config to publish_history
         │
         ▼
    Write new config.yaml
         │
         ▼
    PUT /configs to mihomo external-controller
         │
         ├──success──→ Record success in history
         │
         └──failed───→ Restore backup config.yaml
                       Record failure in history
                       Reload mihomo with restored config
```

### 6.2 Reference Integrity

Before any delete operation:
1. Query all references to the target entity
2. If referenced: show impact list, block delete
3. If not referenced: proceed with confirmation

Reference graph:
- Proxy → referenced by ProxyGroup.members
- ProxyGroup → referenced by ProxyGroup.members (nesting), Rule.target
- RuleProvider → referenced by Rule (RULE-SET type)

### 6.3 Validation Checks

Pre-publish validation includes:
1. **Schema validation**: required fields, valid enum values
2. **Reference integrity**: no dangling references in groups/rules
3. **Empty group detection**: proxy groups with no members
4. **Recursive reference detection**: circular group references
5. **MATCH rule check**: exactly one MATCH rule, must be last
6. **Port conflict detection**: duplicate port bindings
7. **mihomo -t validation**: final structural validation by mihomo itself

## 7. Frontend Key Interactions

### 7.1 Dual-Mode Editing

Each config section supports:
- **Form mode** (default): structured forms with dynamic fields
- **Text mode**: Monaco editor with YAML syntax highlighting

Both modes share the same draft state. Switching modes re-renders from the same underlying data.

### 7.2 Draft Status Indicator

Global header shows:
- Draft status: "Clean" (matches running) or "N changes pending"
- Quick publish button when changes exist
- Running mihomo version and status

### 7.3 Protocol-Dynamic Forms

Proxy node forms dynamically show/hide fields based on selected protocol:
- Common fields: name, server, port
- Protocol-specific: cipher (ss), uuid (vmess/vless), password (trojan), etc.
- Transport options: ws, grpc, h2 (nested form section)

## 8. Configuration

Application config file (`config.yaml` for the Control Plane itself):

```yaml
# Mihomo Control Plane configuration
server:
  host: 0.0.0.0
  port: 8080

mihomo:
  config_path: /etc/mihomo/config.yaml    # Path to mihomo config file
  config_dir: /etc/mihomo                  # mihomo working directory
  binary_path: mihomo                      # Path to mihomo binary (for -t validation)
  external_controller: http://127.0.0.1:9090  # mihomo API endpoint
  secret: ""                                # mihomo API secret

database:
  path: ./data/mihomo-cp.db               # SQLite database path

auth:
  jwt_secret: ""                           # Auto-generated on first run if empty
  access_token_ttl: 24h
  refresh_token_ttl: 168h                  # 7 days
```

## 9. Error Handling

- All API responses use consistent JSON envelope: `{ "data": ..., "error": { "code": "...", "message": "..." } }`
- Publish failures include mihomo's stderr output for diagnostics
- Frontend shows toast notifications for transient errors, inline errors for form validation
- Network errors to mihomo external-controller are surfaced as "mihomo unreachable" status

## 10. Security

- Passwords stored as bcrypt hashes (cost 12)
- JWT tokens signed with HS256
- Sensitive fields (proxy passwords, UUID) masked in API responses by default, revealed on explicit request
- CORS restricted to same-origin in production
- All mihomo API calls are server-side only (secret never exposed to browser)

## 11. Testing Strategy

- **Backend unit tests**: handler, service, store layers with SQLite in-memory
- **Backend integration tests**: full HTTP request cycle
- **Frontend**: component tests with Vitest + Testing Library
- **E2E**: optional, Playwright for critical publish workflow

## 12. Deployment

### Single Binary

```bash
# Build frontend
cd web && pnpm build

# Build Go binary (embeds frontend)
go build -o mihomo-cp ./cmd/mihomo-cp

# Run
./mihomo-cp --config ./config.yaml
```

### Docker

```dockerfile
FROM node:25-alpine AS frontend
WORKDIR /app/web
COPY web/ .
RUN pnpm install --frozen-lockfile && pnpm build

FROM golang:1.25-alpine AS backend
WORKDIR /app
COPY --from=frontend /app/web/dist web/dist
COPY . .
RUN go build -o mihomo-cp ./cmd/mihomo-cp

FROM alpine:3.21
COPY --from=backend /app/mihomo-cp /usr/local/bin/
EXPOSE 8080
ENTRYPOINT ["mihomo-cp"]
```

### systemd

```ini
[Unit]
Description=Mihomo Control Plane
After=network.target

[Service]
ExecStart=/usr/local/bin/mihomo-cp --config /etc/mihomo-cp/config.yaml
Restart=on-failure

[Install]
WantedBy=multi-user.target
```
