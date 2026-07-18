# Changelog

## 2026-07-18

### Added
- **Traffic accounting** — Background collector polls mihomo `/connections` every 2s, attributes per-connection traffic deltas to the actual outbound node (`chains[0]`), and persists hourly buckets in SQLite (90-day retention)
- **Traffic providers** — Vendors with monthly quota, custom billing-cycle reset day (1-31), and glob node patterns (e.g. `PS-*`) for attribution; full CRUD in the new Traffic page
- **Vendor-reported usage** — Optional subscription URL per provider; the `subscription-userinfo` response header is fetched every 30 minutes (with `clash.meta` UA) and used as the authoritative usage figure when larger than the local count
- **Quota alerts** — Configurable thresholds (default 80%, 95%) checked every 5 minutes; each threshold fires at most once per billing cycle; delivery via Bark, ntfy, or generic JSON webhook, plus an in-app alert log and test-notification button
- **Traffic page** — Per-provider usage meters with cycle info, burn-rate exhaustion estimate, top nodes, 30-day stacked daily usage chart, unassigned-node visibility, and alert settings

## 2026-03-25

### Added
- **Auto country detection** — Proxy exit country is automatically detected via mihomo when creating or updating a proxy node (async, no user action needed)
- **GeoIP service** — New `GeoIPService` that detects proxy exit countries by switching mihomo's GLOBAL selector and querying ip-api.com through the SOCKS proxy
- **Latency testing** — Per-proxy delay test button in proxy list and overview page, color-coded results (green <300ms, yellow <600ms, red ≥600ms)
- **Batch latency testing** — "Test All" button on proxies page and overview page, runs all tests concurrently via `Promise.all`
- **Discard changes** — "Discard Changes" button in publish center that re-imports running config and resets draft state
- **Clickable list rows** — Click any row in proxies, proxy groups, rules, or rule providers to open the edit dialog
- **Add rule from connection** — "+" button on each connection row in overview page to quickly create a rule for that host (matches runtime page behavior)
- **Overview redesign** — Replaced stats cards with active connections table (auto-refreshes every 5s, with close/add-rule actions) and proxy list with latency testing
- **Country filter** — Filter proxies by detected country in the proxies page

### Changed
- Country field removed from proxy edit form (now auto-detected, not user-editable)
- "Detect Countries" button removed from proxies page header (replaced by automatic detection)
- Overview page no longer shows publish history or simple stat counters

### Fixed
- Publish discard now properly resets `has_changes` by re-rendering YAML from DB back to config file (byte-identical comparison)
- Frontend detect-countries API timeout increased to 120s to handle slow proxy detection

## 2026-03-24

### Added
- i18n support (Chinese and English) with header dropdown switcher
- Config editor for direct YAML editing integrated with publish workflow
- Runtime connections search, close, and "add rule from host" feature
- Cascade delete: removing a proxy auto-removes it from all referencing groups

### Fixed
- Header dropdown crash when GroupLabel used outside Group context
- Import service null params handling
- Mihomo client double `http://` prefix

## 2026-03-23

### Added
- Initial release of mihomo-cp
- Proxy management (ss, vmess, vless, trojan, hysteria2, tuic, http, socks5)
- Proxy groups with drag-sortable members
- Rules and rule providers CRUD
- System config UI (general, TUN, DNS, external controller)
- Publish center with diff preview, validation, publish, rollback, and version history
- Runtime monitoring (connections, proxy groups, rules, providers)
- JWT authentication with admin/readonly roles
- Single binary deployment with embedded frontend
