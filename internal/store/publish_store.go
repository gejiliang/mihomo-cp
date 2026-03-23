package store

import (
	"fmt"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// PublishStore provides operations for publish history records.
type PublishStore struct {
	db *DB
}

// NewPublishStore creates a new PublishStore.
func NewPublishStore(db *DB) *PublishStore {
	return &PublishStore{db: db}
}

// Create inserts a new publish record.
func (s *PublishStore) Create(r *model.PublishRecord) error {
	r.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO publish_history (id, version, config_yaml, diff_text, status, error_msg, operator, note, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Version, r.ConfigYAML, r.DiffText, r.Status, r.ErrorMsg, r.Operator, r.Note, r.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create publish record: %w", err)
	}
	return nil
}

// List returns the most recent publish records up to the given limit, ordered by created_at DESC.
func (s *PublishStore) List(limit int) ([]*model.PublishRecord, error) {
	var records []*model.PublishRecord
	err := s.db.Select(&records,
		`SELECT id, version, config_yaml, diff_text, status, error_msg, operator, note, created_at FROM publish_history ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list publish records: %w", err)
	}
	return records, nil
}

// GetByID retrieves a publish record by ID.
func (s *PublishStore) GetByID(id string) (*model.PublishRecord, error) {
	var r model.PublishRecord
	err := s.db.Get(&r, `SELECT id, version, config_yaml, diff_text, status, error_msg, operator, note, created_at FROM publish_history WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("get publish record by id: %w", err)
	}
	return &r, nil
}

// NextVersion returns MAX(version)+1, starting at 1 if no records exist.
func (s *PublishStore) NextVersion() (int, error) {
	var version int
	err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) + 1 FROM publish_history`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("next publish version: %w", err)
	}
	return version, nil
}

// GetLastSuccess returns the most recent publish record with status="success".
func (s *PublishStore) GetLastSuccess() (*model.PublishRecord, error) {
	var r model.PublishRecord
	err := s.db.Get(&r,
		`SELECT id, version, config_yaml, diff_text, status, error_msg, operator, note, created_at FROM publish_history WHERE status = 'success' ORDER BY created_at DESC LIMIT 1`,
	)
	if err != nil {
		return nil, fmt.Errorf("get last success publish: %w", err)
	}
	return &r, nil
}
