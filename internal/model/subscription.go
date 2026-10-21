package model

import "time"

// Subscription is normalized subscription metadata linked to a mihomo native
// proxy-provider entry in the system config draft (the config source of
// truth). The subscription row itself never duplicates the provider config;
// it only carries display metadata, reset/expiry policy supplements, and
// status snapshots maintained by SubscriptionService.
type Subscription struct {
	ID      string `json:"id" db:"id"`
	Name    string `json:"name" db:"name"`
	Enabled bool   `json:"enabled" db:"enabled"`

	// ProviderName is the key of the linked native proxy-provider in
	// system_config["proxy-providers"]. Empty for subscriptions that only
	// track metadata (e.g. before linking).
	ProviderName string `json:"provider_name" db:"provider_name"`
	// ProviderSource records how the association came to be:
	// "created" (subscription created the provider entry in the draft)
	// or "linked" (an existing native provider was adopted).
	ProviderSource string `json:"provider_source" db:"provider_source"`

	// Manual supplements for vendor data missing from subscription-userinfo.
	// Zero values mean "unknown", never displayed as 0 quota.
	QuotaBytes    int64  `json:"quota_bytes" db:"quota_bytes"`       // 0 = unknown
	ManualExpire  int64  `json:"manual_expire" db:"manual_expire"`   // unix seconds, 0 = unknown
	ResetDay      int    `json:"reset_day" db:"reset_day"`           // 1-31, 0 = unset
	ResetHour     int    `json:"reset_hour" db:"reset_hour"`         // 0-23
	ResetMinute   int    `json:"reset_minute" db:"reset_minute"`     // 0-59
	ResetTimezone string `json:"reset_timezone" db:"reset_timezone"` // IANA name, "" = server local

	// Optional link to a traffic provider row for locally-sampled usage.
	TrafficProviderID string `json:"traffic_provider_id" db:"traffic_provider_id"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`

	// ── Status snapshots (maintained by SubscriptionService, read-only via API) ──

	// Vendor-reported usage from subscription-userinfo. Kept on last success;
	// a NULL userinfo_fetched_at means "never fetched". Errors do not
	// overwrite previous successful values.
	UserinfoUpload   int64 `json:"userinfo_upload" db:"userinfo_upload"`
	UserinfoDownload int64 `json:"userinfo_download" db:"userinfo_download"`
	UserinfoTotal    int64 `json:"userinfo_total" db:"userinfo_total"`
	UserinfoExpire   int64 `json:"userinfo_expire" db:"userinfo_expire"` // unix seconds, 0 = unknown
	// UserinfoCountersSeen records that the last header carried BOTH upload
	// and download; only then is vendor usage "known".
	UserinfoCountersSeen bool     `json:"-" db:"userinfo_counters"`
	UserinfoFetchedAt    NullTime `json:"-" db:"userinfo_fetched_at"`
	LastRefreshAt        NullTime `json:"-" db:"last_refresh_at"`                     // last refresh attempt
	LastSuccessAt        NullTime `json:"-" db:"last_success_at"`                     // last fully successful refresh
	LastRefreshError     string   `json:"last_refresh_error" db:"last_refresh_error"` // sanitized, no URL/credentials
	LastProviderUpdate   NullTime `json:"-" db:"last_provider_update"`                // last provider refresh via runtime API
}

// DiscoveredProvider is a native proxy-provider found in the system config
// draft that is not yet linked to a subscription row.
type DiscoveredProvider struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	URL             string `json:"url"` // masked when sensitive; discovery masks by default
	Path            string `json:"path"`
	Interval        int    `json:"interval"`
	HasSubscription bool   `json:"has_subscription"` // true if linked already
}

// NodeScore is the persisted scoring state for one proxy node (static or
// provider-dynamic). NodeKey uniquely identifies the node and doubles as the
// public API id: "static/<proxy-name>" or "provider/<provider-name>/<runtime-name>".
type NodeScore struct {
	NodeKey      string `json:"id" db:"node_key"`
	Source       string `json:"node_kind" db:"source"`            // "static" | "provider"
	NodeName     string `json:"node_name" db:"node_name"`         // runtime/display name
	ProviderName string `json:"provider_name" db:"provider_name"` // for provider nodes, "" for static

	Status string `json:"status" db:"status"` // "pending" | "testing" | "ok" | "failed"
	// Fingerprint of the node configuration that produced the current row;
	// a changed fingerprint re-queues the node promptly (keeping history).
	Fingerprint string `json:"-" db:"fingerprint"`
	// Claude-source score (from ip.net.coffee /api/iprisk/{ip}). Pointer
	// distinguishes unknown from a genuine 0.
	ClaudeScore      *int     `json:"trust_score" db:"claude_score"`
	ClaudeIPMatch    string   `json:"claude_ip_match" db:"claude_ip_match"`       // exact|cidr|mismatch|unknown
	ClaudeResponseIP string   `json:"claude_response_ip" db:"claude_response_ip"` // IP the vendor says it scored
	LookupMatch      string   `json:"lookup_ip_match" db:"lookup_match"`          // comprehensive-source validation
	EgressIP         string   `json:"exit_ip" db:"egress_ip"`
	ExitIPSource     string   `json:"exit_ip_source" db:"exit_ip_source"` // claude-trace | cloudflare-trace
	IPInfo           NullJSON `json:"-" db:"ip_info"`                     // trimmed public payload subset
	ErrorCode        string   `json:"error_code" db:"error_code"`

	CheckedAt     NullTime `json:"checked_at" db:"checked_at"`           // last attempt (any outcome)
	LastSuccessAt NullTime `json:"last_success_at" db:"last_success_at"` // last successful scoring
	NextDue       NullTime `json:"next_due" db:"next_due"`
	Attempts      int      `json:"attempts" db:"attempts"`
	LastError     string   `json:"error" db:"last_error"` // sanitized, no URLs/credentials

	CreatedAt NullTime `json:"created_at" db:"created_at"`
	UpdatedAt NullTime `json:"updated_at" db:"updated_at"`
}

// Score status values.
const (
	ScoreStatusPending = "pending"
	ScoreStatusTesting = "testing"
	ScoreStatusOK      = "ok"
	ScoreStatusFailed  = "failed"
)

// IPMatch values for Net.Coffee response-IP validation.
const (
	IPMatchExact    = "exact"    // response IP == Claude-visible egress IP
	IPMatchCIDR     = "cidr"     // response IP is a neighbor in the same /24 (or /64) — CIDR-level data, not a precise single-IP score
	IPMatchMismatch = "mismatch" // response IP unrelated to the egress IP
	IPMatchUnknown  = "unknown"  // no IP to compare against
)
