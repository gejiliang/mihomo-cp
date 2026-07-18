package model

import "time"

// TrafficProvider is an upstream proxy vendor with a monthly traffic quota.
// Nodes are attributed to a provider by matching the node name against
// NodePatterns (path.Match globs, e.g. "PS-*").
type TrafficProvider struct {
	ID              string    `json:"id" db:"id"`
	Name            string    `json:"name" db:"name"`
	QuotaBytes      int64     `json:"quota_bytes" db:"quota_bytes"`
	ResetDay        int       `json:"reset_day" db:"reset_day"`
	NodePatterns    string    `json:"-" db:"node_patterns"` // JSON array, exposed via NodePatternList
	SubscriptionURL string    `json:"subscription_url" db:"subscription_url"`
	Enabled         bool      `json:"enabled" db:"enabled"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`

	NodePatternList []string `json:"node_patterns" db:"-"`
}

// TrafficUserinfo is the vendor-reported usage parsed from the
// subscription-userinfo response header of the provider's subscription URL.
type TrafficUserinfo struct {
	ProviderID string    `json:"provider_id" db:"provider_id"`
	Upload     int64     `json:"upload" db:"upload"`
	Download   int64     `json:"download" db:"download"`
	Total      int64     `json:"total" db:"total"`
	Expire     int64     `json:"expire" db:"expire"`
	FetchedAt  time.Time `json:"fetched_at" db:"fetched_at"`
}

// TrafficAlert is a fired quota alert. (provider_id, cycle_start, threshold)
// is unique so each threshold fires at most once per billing cycle.
type TrafficAlert struct {
	ID         string    `json:"id" db:"id"`
	ProviderID string    `json:"provider_id" db:"provider_id"`
	CycleStart string    `json:"cycle_start" db:"cycle_start"`
	Threshold  int       `json:"threshold" db:"threshold"`
	UsageBytes int64     `json:"usage_bytes" db:"usage_bytes"`
	QuotaBytes int64     `json:"quota_bytes" db:"quota_bytes"`
	Message    string    `json:"message" db:"message"`
	Notified   bool      `json:"notified" db:"notified"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// TrafficSettings holds the alert notification configuration (single row).
// Channel: "" (disabled), "bark", "ntfy" or "webhook".
type TrafficSettings struct {
	ID              int       `json:"id" db:"id"`
	AlertChannel    string    `json:"alert_channel" db:"alert_channel"`
	AlertTarget     string    `json:"alert_target" db:"alert_target"`
	AlertThresholds string    `json:"alert_thresholds" db:"alert_thresholds"` // JSON array of percents
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

// TrafficHourly is one hour of accumulated traffic for one node.
// Bucket is the unix timestamp of the hour start (UTC-truncated).
type TrafficHourly struct {
	Bucket   int64  `json:"bucket" db:"bucket"`
	Node     string `json:"node" db:"node"`
	Upload   int64  `json:"upload" db:"upload"`
	Download int64  `json:"download" db:"download"`
}
