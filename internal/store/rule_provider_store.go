package store

import (
	"fmt"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// RuleProviderStore provides CRUD operations for rule providers.
type RuleProviderStore struct {
	db *DB
}

// NewRuleProviderStore creates a new RuleProviderStore.
func NewRuleProviderStore(db *DB) *RuleProviderStore {
	return &RuleProviderStore{db: db}
}

// List returns all rule providers.
func (s *RuleProviderStore) List() ([]*model.RuleProvider, error) {
	var providers []*model.RuleProvider
	err := s.db.Select(&providers, `SELECT id, name, type, behavior, config, created_at, updated_at FROM rule_providers ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list rule providers: %w", err)
	}
	return providers, nil
}

// GetByID retrieves a rule provider by ID.
func (s *RuleProviderStore) GetByID(id string) (*model.RuleProvider, error) {
	var p model.RuleProvider
	err := s.db.Get(&p, `SELECT id, name, type, behavior, config, created_at, updated_at FROM rule_providers WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("get rule provider by id: %w", err)
	}
	return &p, nil
}

// GetByName retrieves a rule provider by name.
func (s *RuleProviderStore) GetByName(name string) (*model.RuleProvider, error) {
	var p model.RuleProvider
	err := s.db.Get(&p, `SELECT id, name, type, behavior, config, created_at, updated_at FROM rule_providers WHERE name = ?`, name)
	if err != nil {
		return nil, fmt.Errorf("get rule provider by name: %w", err)
	}
	return &p, nil
}

// Create inserts a new rule provider.
func (s *RuleProviderStore) Create(p *model.RuleProvider) error {
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	_, err := s.db.Exec(
		`INSERT INTO rule_providers (id, name, type, behavior, config, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Type, p.Behavior, p.Config, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create rule provider: %w", err)
	}
	return nil
}

// Update updates an existing rule provider.
func (s *RuleProviderStore) Update(p *model.RuleProvider) error {
	p.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(
		`UPDATE rule_providers SET name = ?, type = ?, behavior = ?, config = ?, updated_at = ? WHERE id = ?`,
		p.Name, p.Type, p.Behavior, p.Config, p.UpdatedAt, p.ID,
	)
	if err != nil {
		return fmt.Errorf("update rule provider: %w", err)
	}
	return nil
}

// Delete removes a rule provider by ID.
func (s *RuleProviderStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM rule_providers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete rule provider: %w", err)
	}
	return nil
}

// Count returns the number of rule providers.
func (s *RuleProviderStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM rule_providers`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count rule providers: %w", err)
	}
	return count, nil
}
