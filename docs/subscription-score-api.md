# Subscription & Node Score API contract (backend implementation)

Status: **implemented on this branch** (backend: `internal/handler/subscription.go`,
`internal/handler/scores.go`, `internal/service/subscription_service.go`,
`internal/service/scoring_service.go`). This document is the contract for the
GUI. All endpoints are under `/api` and require JWT auth unless noted.

Conventions carried over from the existing API: responses use the standard
envelope `{"data": ...}` / `{"error": {"code", "message"}}`.

## Permissions

- Reads (`GET`): any authenticated user (same as existing list endpoints).
- Mutations (`POST`/`PUT`/`DELETE`): **admin only**, enforced server-side with
  the same `middleware.RequireAdmin` used by user management. Readonly users
  receive `403 {"error":{"code":"forbidden"}}`.

## Secrets & sanitization

- The subscription URL is **never returned** by any endpoint, in any field,
  in any error. Reads expose `url_masked` (`https://host/…`) only.
- `PUT` accepts an absent/empty `url` and keeps the stored secret; `POST`
  (create) requires `url` unless `provider_name` links an existing provider.
- Error strings (`last_error`, `error`) are sanitized: no URLs, no tokens,
  no node passwords.

## Subscriptions

A subscription is normalized metadata linked to a **native mihomo
`proxy-providers` entry** in the system-config draft (the config source of
truth). Creating/modifying a subscription edits the draft only — nothing is
published automatically; the normal Publish Center preview → validate →
publish flow applies. Vendor rules/DNS/global settings are never imported.

| Method | Path | Description |
|---|---|---|
| GET    | `/api/subscriptions` | List subscriptions with status |
| POST   | `/api/subscriptions` | Create (draft change) — admin |
| GET    | `/api/subscriptions/providers` | Native providers available for linking |
| GET    | `/api/subscriptions/{id}` | One subscription with status |
| PUT    | `/api/subscriptions/{id}` | Update metadata (draft change) — admin |
| DELETE | `/api/subscriptions/{id}` | Delete metadata row — admin |
| POST   | `/api/subscriptions/{id}/refresh` | Manual refresh — admin |

> `GET /api/subscriptions/providers` is a literal path and takes precedence
> over `GET /api/subscriptions/{id}`.

### Subscription object (list/detail)

```json
{
  "id": "b7e6…",
  "name": "示例机场",
  "enabled": true,
  "status": "ok",                          // ok | stale | error | disabled
  "last_error": "",                        // sanitized; empty when healthy
  "provider_name": "示例机场",             // proxy-providers key in system config
  "provider_source": "created",            // created | linked
  "provider_exists": true,                 // present in draft proxy-providers now
  "url_masked": "https://sub.example.com/…",
  "node_count": 42,                        // null = unknown (provider cache not readable)
  "quota_bytes": 214748364800,             // null = unknown (never 0)
  "used_bytes": 18253611008,               // null = unknown
  "remaining_bytes": 196494753792,         // null = unknown; clamped to >= 0 server-side
  "usage_source": "vendor",                // vendor | local | manual | none
  "local_used_bytes": 3210054656,          // null when no linked traffic provider / no data
  "expires_at": "2027-01-09T12:00:00+08:00", // RFC3339 or null
  "expires_source": "vendor",              // vendor | manual | unknown
  "reset_day": 10,
  "reset_hour": 12,
  "reset_minute": 0,
  "timezone": "Asia/Shanghai",             // IANA name or null (= server local)
  "cycle_start": "2026-10-10T12:00:00+08:00",
  "cycle_end": "2026-11-10T12:00:00+08:00",
  "next_reset": "2026-11-10T12:00:00+08:00", // null when reset_day unset
  "userinfo_fetched_at": "2026-10-21T03:00:00Z",  // null = never fetched
  "last_refresh_at": "2026-10-21T03:00:00Z",      // last attempt (any outcome)
  "last_success_at": "2026-10-21T03:00:00Z",
  "stale": false,                          // true when last vendor fetch succeeded > 24h ago or never
  "userinfo": {                            // raw vendor snapshot; null when never fetched
    "upload": 123456789, "download": 18000000000, "total": 214748364800,
    "expire": 1799457600, "fetched_at": "2026-10-21T03:00:00Z"
  },
  "traffic_provider_id": "c1d2…",          // optional link for local usage, "" when none
  "created_at": "…", "updated_at": "…"
}
```

Field semantics:

- `status`: `disabled` when `enabled=false`; `error` when the latest refresh
  attempt failed (message in `last_error`); `stale` when the vendor snapshot
  is missing or older than 24h; otherwise `ok`.
- Quota/usage: vendor numbers come from the `subscription-userinfo` header of
  the subscription URL (fetched by this service; snapshots survive failed
  refreshes). Presence is tracked independently from values: a header without
  `upload`/`download` leaves `used_bytes` null (it must not become a silent
  `0`), while an explicit `upload=0;download=0` is a genuine zero.
  `quota_bytes` falls back to the manual supplement when the vendor does not
  report `total`. `remaining_bytes = max(0, quota - used)` and is `null` when
  either side is unknown. `local_used_bytes` is the locally sampled usage of
  the linked traffic provider for the current cycle — always reported
  separately from the vendor's own accounting.
- Expiry and billing reset are independent fields from independent sources;
  one is never derived from the other.

### Create / update

`POST /api/subscriptions` body:

```json
{
  "name": "示例机场",
  "url": "https://sub.example.com/token/…",   // required unless provider_name links an existing provider
  "provider_name": "示例机场",                 // proxy-providers key; default = name
  "quota_bytes": 214748364800,                 // manual supplement, optional
  "manual_expire": "2027-01-09T12:00:00+08:00",// RFC3339 or omitted
  "reset_day": 10, "reset_hour": 12, "reset_minute": 0,
  "timezone": "Asia/Shanghai",                 // IANA; omitted/"" = server local
  "traffic_provider_id": "",                   // optional link for local usage
  "enabled": true
}
```

- If `provider_name` already exists in the draft, the subscription links to it
  (`provider_source: "linked"`, no config change). Otherwise a native entry is
  **created in the draft** with `{type: http, url, path: "providers/proxies/<name>.yaml",
  interval: 86400}` (`provider_source: "created"`).
- Same-name duplicates are rejected (`409 {"error":{"code":"conflict"}}`) —
  both subscription names and provider keys must be unique.
- `PUT /api/subscriptions/{id}` accepts the same body; an absent/empty `url`
  keeps the stored provider URL untouched. `timezone` accepts any IANA name
  and is validated (`400 {"error":{"code":"bad_request"}}` otherwise).
- Response: the subscription object above, plus `"draft_changed": true` when
  the draft config was modified (GUI points users at Publish Center).

### Delete

`DELETE /api/subscriptions/{id}` **deletes the subscription**: when nothing
references it, the native provider entry is removed from the draft
`proxy-providers` (effective via the normal Publish Center flow) and the
metadata row plus its node score rows are deleted. A deletion that only
removed metadata while the provider kept downloading/being used would be
misleading, so that is not the behavior.

- If any proxy-group's `use` references the provider, the backend refuses with
  `409 {"error":{"code":"referenced","message":"…","references":["AI 组","手动选择"]}}`
  — resolve the group references first; the backend never rewrites business
  groups by itself.
- Linked traffic-provider records are shared accounting data and are **not**
  touched by a subscription deletion.
- There is no "unlink-only" mode under the delete semantics; leaving the
  provider in the config while dropping its management is not offered.

Success response:

```json
{
  "status": "deleted",
  "provider_name": "示例机场",
  "provider_removed_from_draft": true,
  "draft_changed": true
}
```

### Refresh

`POST /api/subscriptions/{id}/refresh` — re-fetches `subscription-userinfo`,
asks the running mihomo to refresh the provider (`PUT /providers/proxies/{name}`,
best effort), and re-enqueues newly discovered nodes for scoring. Never
publishes and never touches mode/selector state.

Response: `{ "subscription": {...}, "error": null }` or
`{ "subscription": {...}, "error": {"code": "rate_limited|userinfo_failed|provider_refresh_failed|not_found", "message": "<sanitized>", "retry_after_seconds": 3600 } }`.
Rate limits are surfaced, never retried silently.

### The `enabled` switch

`enabled=false` means **automatic metadata refresh and automatic node scoring
are paused** for this subscription. The native provider entry stays in the
managed config, business groups are never detached, and the provider can
still carry traffic — the switch must not be read (or labeled) as "stopping
quota usage". Manual refresh keeps working.

### Inventory reconciliation

The scheduler re-reconciles inventory every 10 minutes (and on every
subscription refresh / proxy change): new static proxies and newly discovered
provider-cache nodes enqueue immediately; a changed node configuration
(fingerprint) re-queues promptly while preserving the row's success history;
nodes removed from the provider cache, deleted static proxies, and providers
gone from the draft are pruned from the queue.

### Native providers (discover / link)

`GET /api/subscriptions/providers` →

```json
[
  {
    "name": "示例机场",
    "type": "http",
    "node_count": 42,                     // null = unknown (cache unreadable)
    "linked_subscription_id": "b7e6…",    // null when unlinked
    "linked": true
  }
]
```

`node_count` is read from the provider cache file (the `path` in the provider
entry, resolved relative to the configured mihomo working directory) with
`override.additional-prefix` applied to map cache names to runtime names. The
cache is read once per request and shared; the subscription URL is **not**
re-downloaded for counting.

Creating a subscription with `provider_name` set to one of the unlinked
entries adopts it (no config duplication).

## Node scores

| Method | Path | Description |
|---|---|---|
| GET  | `/api/node-scores` | List all nodes (static + provider) with scores |
| GET  | `/api/node-scores/{id}` | One row (id = node key) |
| POST | `/api/node-scores/{id}/refresh` | Queue one node — admin |
| POST | `/api/node-scores/refresh` | Queue batch — admin |

### Row object

```json
{
  "id": "provider/示例机场/香港-01",
  "node_name": "香港-01",
  "node_kind": "provider",                  // static | provider
  "provider_name": "示例机场",              // null for static nodes
  "status": "ok",                           // pending | checking | ok | error | skipped
  "error": "",                              // sanitized
  "error_code": "",                         // rate_limited | timeout | no_egress | test_failed | config_missing
  "exit_ip": "203.0.113.7",
  "exit_ip_cidr": "203.0.113.0/24",         // null when unknown
  "exit_ip_source": "claude-trace",         // claude-trace | cloudflare-trace
  "trust_score": 87,                        // null = unknown; 0..100, higher = more trusted
  "score_source": "netcoffee-claude",       // independent source; never merged with ip source
  "score_url": "https://ip.net.coffee/claude/",
  "detail_url": "https://ip.net.coffee/ip/203.0.113.7",
  "queried_ip": "203.0.113.7",              // IP we asked the vendor about
  "matched_ip": "203.0.113.7",              // IP the vendor says it scored (claude source)
  "ip_mismatch": false,                     // claude source returned an unrelated IP
  "range_only": false,                      // claude source returned a same-/24 (or /64) neighbor
  "country": "Japan", "country_code": "JP",
  "asn": 15169, "asn_org": "Google LLC", "company_type": "hosting",
  "is_residential": false, "is_datacenter": true,
  "is_vpn": false, "is_proxy": false, "is_tor": false,   // nullable booleans
  "abuser_score": "0.0039 (Low)",
  "ai_verdict": {"label": "…", "confidence": 92, "reasoning": "…"},  // trimmed public field, optional
  "sources": ["netcoffee-claude", "netcoffee-ip"],   // feeds that answered for this row
  "checked_at": "…", "last_success_at": "…", "next_due": "…", "updated_at": "…"
}
```

Semantics:

- `trust_score` is the public Net.Coffee **Claude trust score** (0–100,
  higher = better). It is a third-party public score, **not** an Anthropic
  official risk score — neither fields nor errors may claim otherwise. Unknown
  stays `null`, never `0`.
- The Claude source (`/api/iprisk/{ip}`) and the comprehensive source
  (`/api/ip/lookup/{ip}`) are validated **independently**: `ip_mismatch` /
  `range_only` describe the Claude source; `ip_info.lookup.ip_match` carries
  the comprehensive source's own validation. Scores from the two sources are
  never merged into one number.
- If the vendor answered for a same-/24 neighbor instead of the exact IP,
  `range_only=true` and the score is CIDR-level data, displayed as such —
  never presented as a precise single-IP score.
- The egress IP is observed through the real complete node exit (isolated
  mihomo instance, loopback-only, no TUN/DNS hijack, production
  mode/selectors untouched). `exit_ip_source` says which trace produced it
  (`claude-trace` = claude.ai `/cdn-cgi/trace`; `cloudflare-trace` fallback).
- Rows include queued (`pending`) and failed (`error`) nodes — the list is the
  queue view, not just successful results.

### Refresh (manual)

Both refresh endpoints return `{ "queued": n, "running": n, "skipped": n }`
(the GUI polls the list afterwards; `skipped` counts unknown ids and rows
currently being tested):

```json
{ "queued": 1, "running": 0, "skipped": 0 }
```

> Node score ids contain `/` (`provider/<provider>/<node>`). Pass them
> percent-encoded (`%2F`) in the path — Go's ServeMux matches the escaped
> segment and `{id}` decodes back to the full key.

`POST /api/node-scores/refresh` body: `{"ids": ["provider/示例机场/香港-01", "static/US-01"]}`
or `{"all": true}` (re-queues everything, including already-scored nodes).
Response:

```json
{ "queued": 3, "running": 0, "skipped": 1 }
```

`skipped` counts ids that do not exist or are currently being tested. The GUI
polls the list; the scheduler also auto-refreshes every node every **7 days**
(persisted `next_due`; survives restarts; single failures back off without
blocking the queue; 429 responses respect `Retry-After`).

## Operator settings: egress isolation underlay

The throwaway scoring instance needs to escape a TUN/fake-ip production
gateway. Two settings fields (admin API `GET`/`PUT /api/settings`, merge
semantics: empty value keeps the stored setting):

| Field | Meaning |
|---|---|
| `isolation_interface` | Physical underlay interface the isolated instance binds its sockets to (mihomo `interface-name`). Required when the managed config enables TUN or fake-ip DNS — the scheduler **refuses** to probe unbound (`config_missing`) instead of silently running through the production tunnel. |
| `isolation_dns` | Comma-separated explicit DNS upstreams (mihomo nameserver syntax) for the isolated instance. Empty = public DoH-by-IP defaults; the host system resolver (possibly fake-ip hijacked) is never relied upon. |

When `isolation_interface` is empty but every explicitly configured
nameserver in the managed config carries the same physical-interface suffix
(`…#<iface>`), that interface is derived and used after verifying it exists.
Deployment-specific values are set by the operator via the API; none are
hardcoded in code, tests, or docs.

## Examples (fixtures-safe)

All examples above use RFC 5737 documentation addresses and fictional vendor
names. Real subscription URLs, tokens, node credentials and internal hostnames
never appear in code, fixtures, docs, or logs.
