package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// SubscriptionStore manages normalized subscription metadata and status
// snapshots. The config source of truth (proxy-provider URL/settings) lives
// in the system config draft; this store only carries metadata and status.
type SubscriptionStore struct {
	db *DB
}

// NewSubscriptionStore creates a new SubscriptionStore.
func NewSubscriptionStore(db *DB) *SubscriptionStore {
	return &SubscriptionStore{db: db}
}

// List returns all subscriptions ordered by name.
func (s *SubscriptionStore) List() ([]model.Subscription, error) {
	var subs []model.Subscription
	if err := s.db.Select(&subs, `SELECT * FROM subscriptions ORDER BY name`); err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	return subs, nil
}

// Get returns one subscription by id, or ErrNotFound.
func (s *SubscriptionStore) Get(id string) (*model.Subscription, error) {
	var sub model.Subscription
	err := s.db.Get(&sub, `SELECT * FROM subscriptions WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	return &sub, nil
}

// GetByName returns one subscription by display name, or ErrNotFound.
func (s *SubscriptionStore) GetByName(name string) (*model.Subscription, error) {
	var sub model.Subscription
	err := s.db.Get(&sub, `SELECT * FROM subscriptions WHERE name = ?`, name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get subscription by name: %w", err)
	}
	return &sub, nil
}

// GetByProvider returns the subscription linked to a native provider name, or ErrNotFound.
func (s *SubscriptionStore) GetByProvider(providerName string) (*model.Subscription, error) {
	var sub model.Subscription
	err := s.db.Get(&sub, `SELECT * FROM subscriptions WHERE provider_name = ?`, providerName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get subscription by provider: %w", err)
	}
	return &sub, nil
}

// GetByTrafficProvider returns the traffic-SOURCE subscription managing a
// traffic provider, or ErrNotFound. Native subscriptions may reference the
// same traffic provider for local accounting — they are not managers and do
// not conflict.
func (s *SubscriptionStore) GetByTrafficProvider(trafficProviderID string) (*model.Subscription, error) {
	var sub model.Subscription
	err := s.db.Get(&sub,
		`SELECT * FROM subscriptions WHERE traffic_provider_id = ? AND source_type = 'traffic'`,
		trafficProviderID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get subscription by traffic provider: %w", err)
	}
	return &sub, nil
}

// Create inserts a new subscription row.
func (s *SubscriptionStore) Create(sub *model.Subscription) error {
	now := time.Now().UTC()
	sub.CreatedAt, sub.UpdatedAt = now, now
	_, err := s.db.Exec(
		`INSERT INTO subscriptions (
			id, name, enabled, provider_name, quota_bytes, manual_expire,
			reset_day, reset_hour, reset_minute, reset_timezone, traffic_provider_id, provider_source, source_type,
			userinfo_upload, userinfo_download, userinfo_total, userinfo_expire, userinfo_counters, userinfo_fetched_at,
			userinfo_is_aggregate, userinfo_aggregate_used, vendor_reset_day,
			last_refresh_at, last_success_at, last_refresh_error, last_provider_update,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sub.ID, sub.Name, sub.Enabled, sub.ProviderName, sub.QuotaBytes, sub.ManualExpire,
		sub.ResetDay, sub.ResetHour, sub.ResetMinute, sub.ResetTimezone, sub.TrafficProviderID, sub.ProviderSource, sub.SourceType,
		sub.UserinfoUpload, sub.UserinfoDownload, sub.UserinfoTotal, sub.UserinfoExpire, sub.UserinfoCountersSeen, sub.UserinfoFetchedAt,
		sub.UserinfoIsAggregate, sub.UserinfoAggregateUsed, sub.VendorResetDay,
		sub.LastRefreshAt, sub.LastSuccessAt, sub.LastRefreshError, sub.LastProviderUpdate,
		sub.CreatedAt, sub.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create subscription: %w", err)
	}
	return nil
}

// UpdateMetadata persists ONLY metadata fields (name, enabled, provider
// binding, reset policy, manual supplements, traffic link). It never writes
// snapshot/status columns, so a concurrent refresh's fresh vendor snapshot
// cannot be clobbered by a metadata save.
func (s *SubscriptionStore) UpdateMetadata(sub *model.Subscription) error {
	sub.UpdatedAt = time.Now().UTC()
	res, err := s.db.Exec(
		`UPDATE subscriptions SET
			name = ?, enabled = ?, provider_name = ?, quota_bytes = ?, manual_expire = ?,
			reset_day = ?, reset_hour = ?, reset_minute = ?, reset_timezone = ?, traffic_provider_id = ?, provider_source = ?, source_type = ?, updated_at = ?
		WHERE id = ?`,
		sub.Name, sub.Enabled, sub.ProviderName, sub.QuotaBytes, sub.ManualExpire,
		sub.ResetDay, sub.ResetHour, sub.ResetMinute, sub.ResetTimezone, sub.TrafficProviderID, sub.ProviderSource, sub.SourceType,
		sub.UpdatedAt, sub.ID,
	)
	if err != nil {
		return fmt.Errorf("update subscription metadata: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateMetadataRebind persists metadata AND clears every vendor
// snapshot/status column in ONE atomic statement. It is used when a
// traffic-source subscription is rebound to a different traffic provider:
// vendor data fetched from the old source (directional or aggregate values
// and flags, totals, expiry, observed reset day, fetch/success/refresh/
// error/provider-update timestamps) must never be attributed to the new
// source. Manual supplements (quota_bytes, manual_expire) and the reset
// schedule travel with the metadata columns and are preserved unless the
// request explicitly edited them.
func (s *SubscriptionStore) UpdateMetadataRebind(sub *model.Subscription) error {
	sub.UpdatedAt = time.Now().UTC()
	res, err := s.db.Exec(
		`UPDATE subscriptions SET
			name = ?, enabled = ?, provider_name = ?, quota_bytes = ?, manual_expire = ?,
			reset_day = ?, reset_hour = ?, reset_minute = ?, reset_timezone = ?, traffic_provider_id = ?, provider_source = ?, source_type = ?,
			userinfo_upload = 0, userinfo_download = 0, userinfo_total = 0, userinfo_expire = 0, userinfo_counters = 0,
			userinfo_is_aggregate = 0, userinfo_aggregate_used = 0, vendor_reset_day = 0,
			userinfo_fetched_at = NULL, last_refresh_at = NULL, last_success_at = NULL,
			last_refresh_error = '', last_provider_update = NULL, updated_at = ?
		WHERE id = ?`,
		sub.Name, sub.Enabled, sub.ProviderName, sub.QuotaBytes, sub.ManualExpire,
		sub.ResetDay, sub.ResetHour, sub.ResetMinute, sub.ResetTimezone, sub.TrafficProviderID, sub.ProviderSource, sub.SourceType,
		sub.UpdatedAt, sub.ID,
	)
	if err != nil {
		return fmt.Errorf("update subscription metadata with snapshot reset: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateSnapshot persists ONLY the vendor snapshot and refresh-status columns.
// Metadata columns are untouched, so a snapshot write can never overwrite
// concurrent name/enabled/reset edits. The row's current metadata (including
// provider binding) is read separately for source validation.
func (s *SubscriptionStore) UpdateSnapshot(sub *model.Subscription) error {
	res, err := s.db.Exec(
		`UPDATE subscriptions SET
			userinfo_upload = ?, userinfo_download = ?, userinfo_total = ?, userinfo_expire = ?, userinfo_counters = ?, userinfo_fetched_at = ?,
			userinfo_is_aggregate = ?, userinfo_aggregate_used = ?, vendor_reset_day = ?,
			last_refresh_at = ?, last_success_at = ?, last_refresh_error = ?, last_provider_update = ?, updated_at = ?
		WHERE id = ?`,
		sub.UserinfoUpload, sub.UserinfoDownload, sub.UserinfoTotal, sub.UserinfoExpire, sub.UserinfoCountersSeen, sub.UserinfoFetchedAt,
		sub.UserinfoIsAggregate, sub.UserinfoAggregateUsed, sub.VendorResetDay,
		sub.LastRefreshAt, sub.LastSuccessAt, sub.LastRefreshError, sub.LastProviderUpdate,
		time.Now().UTC(), sub.ID,
	)
	if err != nil {
		return fmt.Errorf("update subscription snapshot: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a subscription row.
func (s *SubscriptionStore) Delete(id string) error {
	res, err := s.db.Exec(`DELETE FROM subscriptions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
