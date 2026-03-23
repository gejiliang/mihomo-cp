package store

import (
	"fmt"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// RuleStore provides CRUD operations for rules.
type RuleStore struct {
	db *DB
}

// NewRuleStore creates a new RuleStore.
func NewRuleStore(db *DB) *RuleStore {
	return &RuleStore{db: db}
}

// List returns rules with optional search and type filters, ordered by sort_order.
func (s *RuleStore) List(search, ruleType string) ([]*model.Rule, error) {
	query := `SELECT id, type, payload, target, params, sort_order, created_at, updated_at FROM rules WHERE 1=1`
	args := []any{}

	if search != "" {
		query += ` AND (payload LIKE ? OR target LIKE ?)`
		args = append(args, "%"+search+"%", "%"+search+"%")
	}
	if ruleType != "" {
		query += ` AND type = ?`
		args = append(args, ruleType)
	}
	query += ` ORDER BY sort_order`

	var rules []*model.Rule
	err := s.db.Select(&rules, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	return rules, nil
}

// GetByID retrieves a rule by ID.
func (s *RuleStore) GetByID(id string) (*model.Rule, error) {
	var r model.Rule
	err := s.db.Get(&r, `SELECT id, type, payload, target, params, sort_order, created_at, updated_at FROM rules WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("get rule by id: %w", err)
	}
	return &r, nil
}

// Create inserts a new rule.
func (s *RuleStore) Create(r *model.Rule) error {
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now
	_, err := s.db.Exec(
		`INSERT INTO rules (id, type, payload, target, params, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Type, r.Payload, r.Target, r.Params, r.SortOrder, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create rule: %w", err)
	}
	return nil
}

// Update updates an existing rule.
func (s *RuleStore) Update(r *model.Rule) error {
	r.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(
		`UPDATE rules SET type = ?, payload = ?, target = ?, params = ?, sort_order = ?, updated_at = ? WHERE id = ?`,
		r.Type, r.Payload, r.Target, r.Params, r.SortOrder, r.UpdatedAt, r.ID,
	)
	if err != nil {
		return fmt.Errorf("update rule: %w", err)
	}
	return nil
}

// Delete removes a rule by ID.
func (s *RuleStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	return nil
}

// Reorder updates the sort_order of rules based on the provided ordered IDs.
func (s *RuleStore) Reorder(ids []string) error {
	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin reorder transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for i, id := range ids {
		_, err := tx.Exec(`UPDATE rules SET sort_order = ? WHERE id = ?`, i, id)
		if err != nil {
			return fmt.Errorf("reorder rule %s: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reorder: %w", err)
	}
	return nil
}

// Count returns the number of rules.
func (s *RuleStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM rules`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count rules: %w", err)
	}
	return count, nil
}
