package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// TrafficStore manages traffic providers, hourly usage buckets, vendor
// userinfo snapshots, alerts and alert settings.
type TrafficStore struct {
	db *DB
}

// NewTrafficStore creates a new TrafficStore.
func NewTrafficStore(db *DB) *TrafficStore {
	return &TrafficStore{db: db}
}

// ── Providers ────────────────────────────────────────────────────────────────

func decodePatterns(p *model.TrafficProvider) {
	p.NodePatternList = []string{}
	_ = json.Unmarshal([]byte(p.NodePatterns), &p.NodePatternList)
}

func encodePatterns(p *model.TrafficProvider) error {
	if p.NodePatternList == nil {
		p.NodePatternList = []string{}
	}
	data, err := json.Marshal(p.NodePatternList)
	if err != nil {
		return fmt.Errorf("marshal node patterns: %w", err)
	}
	p.NodePatterns = string(data)
	return nil
}

// ListProviders returns all traffic providers.
func (s *TrafficStore) ListProviders() ([]model.TrafficProvider, error) {
	var providers []model.TrafficProvider
	if err := s.db.Select(&providers, `SELECT * FROM traffic_providers ORDER BY name`); err != nil {
		return nil, fmt.Errorf("list traffic providers: %w", err)
	}
	for i := range providers {
		decodePatterns(&providers[i])
	}
	return providers, nil
}

// GetProvider returns one provider by id.
func (s *TrafficStore) GetProvider(id string) (*model.TrafficProvider, error) {
	var p model.TrafficProvider
	err := s.db.Get(&p, `SELECT * FROM traffic_providers WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get traffic provider: %w", err)
	}
	decodePatterns(&p)
	return &p, nil
}

// CreateProvider inserts a new provider.
func (s *TrafficStore) CreateProvider(p *model.TrafficProvider) error {
	if err := encodePatterns(p); err != nil {
		return err
	}
	now := time.Now().UTC()
	p.CreatedAt, p.UpdatedAt = now, now
	_, err := s.db.Exec(
		`INSERT INTO traffic_providers (id, name, quota_bytes, reset_day, node_patterns, subscription_url, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.QuotaBytes, p.ResetDay, p.NodePatterns, p.SubscriptionURL, p.Enabled, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create traffic provider: %w", err)
	}
	return nil
}

// UpdateProvider updates an existing provider.
func (s *TrafficStore) UpdateProvider(p *model.TrafficProvider) error {
	if err := encodePatterns(p); err != nil {
		return err
	}
	p.UpdatedAt = time.Now().UTC()
	res, err := s.db.Exec(
		`UPDATE traffic_providers SET name = ?, quota_bytes = ?, reset_day = ?, node_patterns = ?, subscription_url = ?, enabled = ?, updated_at = ?
		 WHERE id = ?`,
		p.Name, p.QuotaBytes, p.ResetDay, p.NodePatterns, p.SubscriptionURL, p.Enabled, p.UpdatedAt, p.ID,
	)
	if err != nil {
		return fmt.Errorf("update traffic provider: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteProvider removes a provider and its userinfo/alerts.
func (s *TrafficStore) DeleteProvider(id string) error {
	if _, err := s.db.Exec(`DELETE FROM traffic_userinfo WHERE provider_id = ?`, id); err != nil {
		return fmt.Errorf("delete traffic userinfo: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM traffic_alerts WHERE provider_id = ?`, id); err != nil {
		return fmt.Errorf("delete traffic alerts: %w", err)
	}
	res, err := s.db.Exec(`DELETE FROM traffic_providers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete traffic provider: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ── Hourly buckets ───────────────────────────────────────────────────────────

// AddHourly accumulates deltas into hourly buckets (upsert-add).
func (s *TrafficStore) AddHourly(rows []model.TrafficHourly) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	for _, r := range rows {
		if _, err := tx.Exec(
			`INSERT INTO traffic_hourly (bucket, node, upload, download) VALUES (?, ?, ?, ?)
			 ON CONFLICT(bucket, node) DO UPDATE SET upload = upload + excluded.upload, download = download + excluded.download`,
			r.Bucket, r.Node, r.Upload, r.Download,
		); err != nil {
			return fmt.Errorf("upsert traffic bucket: %w", err)
		}
	}
	return tx.Commit()
}

// HourlySince returns all buckets with bucket >= since (unix seconds).
func (s *TrafficStore) HourlySince(since int64) ([]model.TrafficHourly, error) {
	var rows []model.TrafficHourly
	if err := s.db.Select(&rows, `SELECT * FROM traffic_hourly WHERE bucket >= ? ORDER BY bucket`, since); err != nil {
		return nil, fmt.Errorf("query traffic hourly: %w", err)
	}
	return rows, nil
}

// PruneHourlyBefore deletes buckets older than the given unix timestamp.
func (s *TrafficStore) PruneHourlyBefore(before int64) error {
	if _, err := s.db.Exec(`DELETE FROM traffic_hourly WHERE bucket < ?`, before); err != nil {
		return fmt.Errorf("prune traffic hourly: %w", err)
	}
	return nil
}

// ── Vendor userinfo ──────────────────────────────────────────────────────────

// UpsertUserinfo stores the latest vendor-reported usage for a provider.
func (s *TrafficStore) UpsertUserinfo(u *model.TrafficUserinfo) error {
	u.FetchedAt = time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO traffic_userinfo (provider_id, upload, download, total, expire, fetched_at) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(provider_id) DO UPDATE SET upload = excluded.upload, download = excluded.download, total = excluded.total, expire = excluded.expire, fetched_at = excluded.fetched_at`,
		u.ProviderID, u.Upload, u.Download, u.Total, u.Expire, u.FetchedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert traffic userinfo: %w", err)
	}
	return nil
}

// GetUserinfo returns the latest vendor-reported usage, or ErrNotFound.
func (s *TrafficStore) GetUserinfo(providerID string) (*model.TrafficUserinfo, error) {
	var u model.TrafficUserinfo
	err := s.db.Get(&u, `SELECT * FROM traffic_userinfo WHERE provider_id = ?`, providerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get traffic userinfo: %w", err)
	}
	return &u, nil
}

// ── Alerts ───────────────────────────────────────────────────────────────────

// InsertAlert inserts an alert; returns false if this (provider, cycle,
// threshold) alert already fired.
func (s *TrafficStore) InsertAlert(a *model.TrafficAlert) (bool, error) {
	a.CreatedAt = time.Now().UTC()
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO traffic_alerts (id, provider_id, cycle_start, threshold, usage_bytes, quota_bytes, message, notified, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.ProviderID, a.CycleStart, a.Threshold, a.UsageBytes, a.QuotaBytes, a.Message, a.Notified, a.CreatedAt,
	)
	if err != nil {
		return false, fmt.Errorf("insert traffic alert: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// MarkAlertNotified marks an alert as delivered.
func (s *TrafficStore) MarkAlertNotified(id string) error {
	if _, err := s.db.Exec(`UPDATE traffic_alerts SET notified = 1 WHERE id = ?`, id); err != nil {
		return fmt.Errorf("mark alert notified: %w", err)
	}
	return nil
}

// ListAlerts returns the most recent alerts.
func (s *TrafficStore) ListAlerts(limit int) ([]model.TrafficAlert, error) {
	if limit <= 0 {
		limit = 50
	}
	var alerts []model.TrafficAlert
	if err := s.db.Select(&alerts, `SELECT * FROM traffic_alerts ORDER BY created_at DESC LIMIT ?`, limit); err != nil {
		return nil, fmt.Errorf("list traffic alerts: %w", err)
	}
	return alerts, nil
}

// ── Settings ─────────────────────────────────────────────────────────────────

// GetSettings returns the alert settings row.
func (s *TrafficStore) GetSettings() (*model.TrafficSettings, error) {
	var st model.TrafficSettings
	if err := s.db.Get(&st, `SELECT * FROM traffic_settings WHERE id = 1`); err != nil {
		return nil, fmt.Errorf("get traffic settings: %w", err)
	}
	return &st, nil
}

// UpdateSettings updates the alert settings row.
func (s *TrafficStore) UpdateSettings(st *model.TrafficSettings) error {
	st.UpdatedAt = time.Now().UTC()
	if _, err := s.db.Exec(
		`UPDATE traffic_settings SET alert_channel = ?, alert_target = ?, alert_thresholds = ?, updated_at = ? WHERE id = 1`,
		st.AlertChannel, st.AlertTarget, st.AlertThresholds, st.UpdatedAt,
	); err != nil {
		return fmt.Errorf("update traffic settings: %w", err)
	}
	return nil
}
