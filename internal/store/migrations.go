package store

// migrations is a list of SQL migrations in order.
// Each entry corresponds to a sequential version number starting at 1.
var migrations = []string{
	// 001: users
	`CREATE TABLE IF NOT EXISTS users (
		id         TEXT PRIMARY KEY,
		username   TEXT UNIQUE NOT NULL,
		password   TEXT NOT NULL,
		role       TEXT NOT NULL DEFAULT 'admin',
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`,

	// 002: proxies
	`CREATE TABLE IF NOT EXISTS proxies (
		id         TEXT PRIMARY KEY,
		name       TEXT UNIQUE NOT NULL,
		type       TEXT NOT NULL,
		config     TEXT NOT NULL DEFAULT '{}',
		sort_order INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`,

	// 003: proxy_groups
	`CREATE TABLE IF NOT EXISTS proxy_groups (
		id         TEXT PRIMARY KEY,
		name       TEXT UNIQUE NOT NULL,
		type       TEXT NOT NULL,
		config     TEXT NOT NULL DEFAULT '{}',
		members    TEXT NOT NULL DEFAULT '[]',
		sort_order INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`,

	// 004: rules
	`CREATE TABLE IF NOT EXISTS rules (
		id         TEXT PRIMARY KEY,
		type       TEXT NOT NULL,
		payload    TEXT NOT NULL,
		target     TEXT NOT NULL,
		params     TEXT NOT NULL DEFAULT '{}',
		sort_order INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`,

	// 005: rule_providers
	`CREATE TABLE IF NOT EXISTS rule_providers (
		id         TEXT PRIMARY KEY,
		name       TEXT UNIQUE NOT NULL,
		type       TEXT NOT NULL,
		behavior   TEXT NOT NULL,
		config     TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`,

	// 006: system_config
	`CREATE TABLE IF NOT EXISTS system_config (
		id         INTEGER PRIMARY KEY CHECK(id = 1),
		config     TEXT NOT NULL DEFAULT '{}',
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	INSERT OR IGNORE INTO system_config (id) VALUES (1)`,

	// 007: publish_history
	`CREATE TABLE IF NOT EXISTS publish_history (
		id          TEXT PRIMARY KEY,
		version     INTEGER NOT NULL,
		config_yaml TEXT NOT NULL,
		diff_text   TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL,
		error_msg   TEXT NOT NULL DEFAULT '',
		operator    TEXT NOT NULL DEFAULT '',
		note        TEXT NOT NULL DEFAULT '',
		created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
	)`,

	// 008: app_settings
	`CREATE TABLE IF NOT EXISTS app_settings (
		id              INTEGER PRIMARY KEY CHECK(id = 1),
		mihomo_config   TEXT NOT NULL DEFAULT '/etc/mihomo/config.yaml',
		mihomo_dir      TEXT NOT NULL DEFAULT '/etc/mihomo',
		mihomo_binary   TEXT NOT NULL DEFAULT 'mihomo',
		ext_controller  TEXT NOT NULL DEFAULT 'http://127.0.0.1:9090',
		ext_secret      TEXT NOT NULL DEFAULT '',
		updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	INSERT OR IGNORE INTO app_settings (id) VALUES (1)`,

	// 009: raw config draft column
	`ALTER TABLE app_settings ADD COLUMN raw_config_draft TEXT NOT NULL DEFAULT ''`,

	// 010: convert publish_history.version from INTEGER to TEXT (date-prefixed)
	// SQLite allows storing text in INTEGER columns, but we recreate for clarity.
	`CREATE TABLE publish_history_new (
		id          TEXT PRIMARY KEY,
		version     TEXT NOT NULL,
		config_yaml TEXT NOT NULL,
		diff_text   TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL,
		error_msg   TEXT NOT NULL DEFAULT '',
		operator    TEXT NOT NULL DEFAULT '',
		note        TEXT NOT NULL DEFAULT '',
		created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	INSERT INTO publish_history_new SELECT id, CAST(version AS TEXT), config_yaml, diff_text, status, error_msg, operator, note, created_at FROM publish_history;
	DROP TABLE publish_history;
	ALTER TABLE publish_history_new RENAME TO publish_history`,

	// 011: add country column to proxies
	`ALTER TABLE proxies ADD COLUMN country TEXT NOT NULL DEFAULT ''`,

	// 012: traffic accounting & alerting
	`CREATE TABLE IF NOT EXISTS traffic_providers (
		id               TEXT PRIMARY KEY,
		name             TEXT UNIQUE NOT NULL,
		quota_bytes      INTEGER NOT NULL DEFAULT 0,
		reset_day        INTEGER NOT NULL DEFAULT 1,
		node_patterns    TEXT NOT NULL DEFAULT '[]',
		subscription_url TEXT NOT NULL DEFAULT '',
		enabled          INTEGER NOT NULL DEFAULT 1,
		created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at       DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE IF NOT EXISTS traffic_hourly (
		bucket   INTEGER NOT NULL,
		node     TEXT NOT NULL,
		upload   INTEGER NOT NULL DEFAULT 0,
		download INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (bucket, node)
	);
	CREATE TABLE IF NOT EXISTS traffic_userinfo (
		provider_id TEXT PRIMARY KEY,
		upload      INTEGER NOT NULL DEFAULT 0,
		download    INTEGER NOT NULL DEFAULT 0,
		total       INTEGER NOT NULL DEFAULT 0,
		expire      INTEGER NOT NULL DEFAULT 0,
		fetched_at  DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS traffic_alerts (
		id          TEXT PRIMARY KEY,
		provider_id TEXT NOT NULL,
		cycle_start TEXT NOT NULL,
		threshold   INTEGER NOT NULL,
		usage_bytes INTEGER NOT NULL DEFAULT 0,
		quota_bytes INTEGER NOT NULL DEFAULT 0,
		message     TEXT NOT NULL DEFAULT '',
		notified    INTEGER NOT NULL DEFAULT 0,
		created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
		UNIQUE (provider_id, cycle_start, threshold)
	);
	CREATE TABLE IF NOT EXISTS traffic_settings (
		id               INTEGER PRIMARY KEY CHECK(id = 1),
		alert_channel    TEXT NOT NULL DEFAULT '',
		alert_target     TEXT NOT NULL DEFAULT '',
		alert_thresholds TEXT NOT NULL DEFAULT '[80,95]',
		updated_at       DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	INSERT OR IGNORE INTO traffic_settings (id) VALUES (1)`,

	// 013: subscriptions — normalized metadata linked to native proxy-providers
	`CREATE TABLE IF NOT EXISTS subscriptions (
		id                    TEXT PRIMARY KEY,
		name                  TEXT UNIQUE NOT NULL,
		enabled               INTEGER NOT NULL DEFAULT 1,
		provider_name         TEXT NOT NULL DEFAULT '',
		quota_bytes           INTEGER NOT NULL DEFAULT 0,
		manual_expire         INTEGER NOT NULL DEFAULT 0,
		reset_day             INTEGER NOT NULL DEFAULT 0,
		reset_hour            INTEGER NOT NULL DEFAULT 0,
		reset_minute          INTEGER NOT NULL DEFAULT 0,
		reset_timezone        TEXT NOT NULL DEFAULT '',
		traffic_provider_id   TEXT NOT NULL DEFAULT '',
		provider_source       TEXT NOT NULL DEFAULT 'created',
		userinfo_upload       INTEGER NOT NULL DEFAULT 0,
		userinfo_download     INTEGER NOT NULL DEFAULT 0,
		userinfo_total        INTEGER NOT NULL DEFAULT 0,
		userinfo_expire       INTEGER NOT NULL DEFAULT 0,
		userinfo_counters     INTEGER NOT NULL DEFAULT 0,
		userinfo_fetched_at   DATETIME,
		last_refresh_at       DATETIME,
		last_success_at       DATETIME,
		last_refresh_error    TEXT NOT NULL DEFAULT '',
		last_provider_update  DATETIME,
		created_at            DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at            DATETIME NOT NULL DEFAULT (datetime('now'))
	)`,

	// 014: node exit-IP scoring state + per-egress-IP score cache
	`CREATE TABLE IF NOT EXISTS node_scores (
		node_key            TEXT PRIMARY KEY,
		source              TEXT NOT NULL DEFAULT '',
		node_name           TEXT NOT NULL DEFAULT '',
		provider_name       TEXT NOT NULL DEFAULT '',
		status              TEXT NOT NULL DEFAULT 'pending',
		fingerprint         TEXT NOT NULL DEFAULT '',
		claude_score        INTEGER,
		claude_ip_match     TEXT NOT NULL DEFAULT 'unknown',
		claude_response_ip  TEXT NOT NULL DEFAULT '',
		lookup_match        TEXT NOT NULL DEFAULT 'unknown',
		egress_ip           TEXT NOT NULL DEFAULT '',
		exit_ip_source      TEXT NOT NULL DEFAULT '',
		ip_info             TEXT,
		error_code          TEXT NOT NULL DEFAULT '',
		checked_at          DATETIME,
		last_success_at     DATETIME,
		next_due            DATETIME,
		attempts            INTEGER NOT NULL DEFAULT 0,
		last_error          TEXT NOT NULL DEFAULT '',
		created_at          DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at          DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	CREATE INDEX IF NOT EXISTS idx_node_scores_next_due ON node_scores (next_due);
	CREATE TABLE IF NOT EXISTS egress_ip_scores (
		egress_ip     TEXT PRIMARY KEY,
		claude_score  INTEGER,
		ip_info       TEXT,
		fetched_at    DATETIME NOT NULL
	)`,

	// 015: extend monthly reset with hour/minute/IANA timezone (empty tz = server local, 0:00 = legacy behavior)
	`ALTER TABLE traffic_providers ADD COLUMN reset_hour   INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE traffic_providers ADD COLUMN reset_minute INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE traffic_providers ADD COLUMN reset_timezone TEXT NOT NULL DEFAULT ''`,

	// 017: persisted manual-refresh request flag for node scores — survives
	// restarts and lets a paused subscription still serve explicit manual
	// requests without overloading status/error semantics.
	`ALTER TABLE node_scores ADD COLUMN manual_request INTEGER NOT NULL DEFAULT 0`,

	// 016: generic egress-isolation underlay settings for the throwaway
	// scoring instance. Empty values keep safe defaults; deployment-specific
	// interface/DNS are set via the settings API by the operator.
	`ALTER TABLE app_settings ADD COLUMN isolation_interface TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE app_settings ADD COLUMN isolation_dns TEXT NOT NULL DEFAULT ''`,
}
