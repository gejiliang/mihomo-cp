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
}
