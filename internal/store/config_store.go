package store

import (
	"encoding/json"
	"fmt"
	"time"
)

// ConfigStore manages the single-row system_config table.
type ConfigStore struct {
	db *DB
}

// NewConfigStore creates a new ConfigStore.
func NewConfigStore(db *DB) *ConfigStore {
	return &ConfigStore{db: db}
}

// Get returns the raw JSON config from system_config.
func (s *ConfigStore) Get() (json.RawMessage, error) {
	var config string
	err := s.db.QueryRow(`SELECT config FROM system_config WHERE id = 1`).Scan(&config)
	if err != nil {
		return nil, fmt.Errorf("get system config: %w", err)
	}
	return json.RawMessage(config), nil
}

// Update updates the single system_config row.
func (s *ConfigStore) Update(config json.RawMessage) error {
	_, err := s.db.Exec(
		`UPDATE system_config SET config = ?, updated_at = ? WHERE id = 1`,
		string(config), time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("update system config: %w", err)
	}
	return nil
}
