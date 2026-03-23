package store

import (
	"fmt"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// SettingsStore manages the single-row app_settings table.
type SettingsStore struct {
	db *DB
}

// NewSettingsStore creates a new SettingsStore.
func NewSettingsStore(db *DB) *SettingsStore {
	return &SettingsStore{db: db}
}

// Get returns the current app settings.
func (s *SettingsStore) Get() (*model.AppSettings, error) {
	var settings model.AppSettings
	err := s.db.Get(&settings, `SELECT id, mihomo_config, mihomo_dir, mihomo_binary, ext_controller, ext_secret, updated_at FROM app_settings WHERE id = 1`)
	if err != nil {
		return nil, fmt.Errorf("get app settings: %w", err)
	}
	return &settings, nil
}

// Update updates the single app_settings row.
func (s *SettingsStore) Update(st *model.AppSettings) error {
	st.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(
		`UPDATE app_settings SET mihomo_config = ?, mihomo_dir = ?, mihomo_binary = ?, ext_controller = ?, ext_secret = ?, updated_at = ? WHERE id = 1`,
		st.MihomoConfig, st.MihomoDir, st.MihomoBinary, st.ExtController, st.ExtSecret, st.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update app settings: %w", err)
	}
	return nil
}
