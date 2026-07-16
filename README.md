# Mihomo Control Plane (mihomo-cp)

Web-based management panel for [mihomo](https://github.com/MetaCubeX/mihomo) proxy. Edit configuration through a structured UI or raw YAML editor, validate with the mihomo binary, and publish changes with one click.

[中文说明](./README.zh.md)

## Features

- **Proxy Management** — Create, edit, delete, copy, and reorder proxy nodes (ss, vmess, vless, trojan, hysteria2, tuic, etc.) with auto country detection and per-proxy latency testing
- **Proxy Groups** — Selector, URLTest, Fallback, LoadBalance, Relay groups with drag-sort members
- **Rules & Rule Providers** — Full rule CRUD with type/payload/target, rule-set provider management
- **System Config** — General, TUN, DNS, External Controller settings via structured UI
- **Config Editor** — Direct YAML editing for advanced users, integrated with the publish workflow
- **Publish Center** — Preview diff, validate (structural + mihomo binary check), publish, discard changes, rollback, version history with date-prefixed versions (e.g., `20260324-1`)
- **Overview Dashboard** — Active connections with close/add-rule actions (auto-refresh), proxy list with concurrent batch latency testing
- **Runtime Monitor** — Live connections with search/close, proxy groups with delay testing and switching, running rules/providers, quick "add rule" from connection host
- **i18n** — Chinese and English, switchable from header dropdown
- **Auth** — JWT-based login, admin/readonly roles, user management, password change
- **Single Binary** — Frontend embedded via `embed.FS`, no separate web server needed

## Tech Stack

| Layer    | Technology                                         |
|----------|----------------------------------------------------|
| Backend  | Go 1.22+, `net/http` stdlib router, SQLite (modernc.org/sqlite) |
| Frontend | React 19, TypeScript, Vite, Tailwind CSS, shadcn/ui |
| State    | Zustand (auth, drafts, i18n)                       |
| HTTP     | ky (frontend), stdlib (backend)                    |
| Auth     | JWT (access + refresh tokens), bcrypt passwords    |

## Quick Start

### Prerequisites

- Go 1.22+
- Node.js 20+ and npm (or pnpm)
- mihomo binary (for config validation)

### Build

```bash
# Build frontend + backend
make build

# Or separately:
cd web && npm install && npm run build
cd .. && go build -o mihomo-cp ./cmd/mihomo-cp
```

### Cross-compile for Linux

```bash
cd web && npm run build && cd ..
GOOS=linux GOARCH=amd64 go build -o mihomo-cp ./cmd/mihomo-cp
```

### Run

```bash
./mihomo-cp -host 0.0.0.0 -port 8080 -db ./data/mihomo-cp.db
```

On first launch, create an admin user via the UI (default signup creates admin role).

### Configuration

After login, go to **Settings** and configure:

| Setting              | Description                          | Example                    |
|----------------------|--------------------------------------|----------------------------|
| Config Path          | Path to mihomo's config.yaml         | `/etc/mihomo/config.yaml`  |
| Working Directory    | mihomo working directory             | `/etc/mihomo`              |
| Binary Path          | Path to mihomo executable            | `/usr/local/bin/mihomo`    |
| Controller URL       | mihomo external controller endpoint  | `http://127.0.0.1:9090`   |
| Controller Secret    | API secret for mihomo controller     | `your-secret`              |

### Docker

```bash
docker build -t mihomo-cp .
docker run -d -p 8080:8080 -v ./data:/app/data mihomo-cp
```

## Architecture

```
cmd/mihomo-cp/          # Entry point
internal/
  handler/              # HTTP handlers (auth, proxies, groups, rules, publish, runtime, settings)
  middleware/           # JWT auth, admin-only, request logging
  model/               # Data models (User, Proxy, ProxyGroup, Rule, RuleProvider, etc.)
  server/              # HTTP server setup and route registration
  service/             # Business logic (auth, config rendering, validation, publish, mihomo client, GeoIP detection)
  store/               # SQLite data access layer with auto-migrations
web/
  src/
    api/               # API client (ky-based, typed endpoints)
    components/        # React components (layout, proxies, rules, publish, runtime, shared)
    i18n/              # Internationalization (en.ts, zh.ts)
    pages/             # Route pages (overview, proxies, rules, settings, publish, runtime)
    stores/            # Zustand stores (auth, draft)
```

## Workflow

1. **Edit** — Modify proxies, groups, rules, system config, or raw YAML via the UI
2. **Preview** — Go to Publish Center to see the diff between draft and running config
3. **Validate** — Structural validation (dangling refs, circular deps) + mihomo binary test
4. **Publish** — Write config to disk and reload mihomo; auto-rollback on failure
5. **Monitor** — Check runtime connections, proxy delays, and active rules

## API

All endpoints are prefixed with `/api/`. Authentication required unless noted.

| Method | Path                              | Description                |
|--------|-----------------------------------|----------------------------|
| POST   | `/auth/login`                     | Login (public)             |
| POST   | `/auth/refresh`                   | Refresh token (public)     |
| GET    | `/auth/me`                        | Current user info          |
| PUT    | `/auth/change-password`           | Change password            |
| GET    | `/proxies`                        | List proxies               |
| POST   | `/proxies`                        | Create proxy               |
| GET    | `/proxy-groups`                   | List proxy groups          |
| POST   | `/proxy-groups`                   | Create proxy group         |
| GET    | `/rules`                          | List rules                 |
| POST   | `/rules`                          | Create rule                |
| GET    | `/rule-providers`                 | List rule providers        |
| GET    | `/system-config`                  | Get system config          |
| PUT    | `/system-config`                  | Update system config       |
| GET    | `/publish/status`                 | Draft change status        |
| GET    | `/publish/preview`                | Preview draft YAML + diff  |
| POST   | `/publish/validate`               | Validate draft             |
| POST   | `/publish`                        | Publish draft              |
| POST   | `/publish/discard`                | Discard draft changes      |
| POST   | `/publish/rollback`               | Rollback to last success   |
| GET    | `/publish/history`                | Publish history            |
| GET    | `/runtime/connections`            | Live connections           |
| GET    | `/runtime/proxies`                | Running proxy status       |
| GET    | `/runtime/rules`                  | Running rules              |
| GET    | `/runtime/providers`              | Running providers          |
| GET    | `/settings`                       | App settings               |
| PUT    | `/settings`                       | Update app settings        |
| GET    | `/settings/config-yaml`           | Get config YAML (draft or file) |
| PUT    | `/settings/config-yaml`           | Save config YAML draft     |
| DELETE | `/settings/config-yaml`           | Clear config YAML draft    |
| GET    | `/settings/users`                 | List users (admin only)    |
| POST   | `/settings/users`                 | Create user (admin only)   |

## License

MIT
