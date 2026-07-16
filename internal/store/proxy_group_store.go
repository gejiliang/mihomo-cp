package store

import (
	"fmt"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// ProxyGroupStore provides CRUD operations for proxy groups.
type ProxyGroupStore struct {
	db *DB
}

// NewProxyGroupStore creates a new ProxyGroupStore.
func NewProxyGroupStore(db *DB) *ProxyGroupStore {
	return &ProxyGroupStore{db: db}
}

// List returns all proxy groups ordered by sort_order.
func (s *ProxyGroupStore) List() ([]*model.ProxyGroup, error) {
	var groups []*model.ProxyGroup
	err := s.db.Select(&groups, `SELECT id, name, type, config, members, sort_order, created_at, updated_at FROM proxy_groups ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("list proxy groups: %w", err)
	}
	return groups, nil
}

// GetByID retrieves a proxy group by ID.
func (s *ProxyGroupStore) GetByID(id string) (*model.ProxyGroup, error) {
	var g model.ProxyGroup
	err := s.db.Get(&g, `SELECT id, name, type, config, members, sort_order, created_at, updated_at FROM proxy_groups WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("get proxy group by id: %w", err)
	}
	return &g, nil
}

// GetByName retrieves a proxy group by name.
func (s *ProxyGroupStore) GetByName(name string) (*model.ProxyGroup, error) {
	var g model.ProxyGroup
	err := s.db.Get(&g, `SELECT id, name, type, config, members, sort_order, created_at, updated_at FROM proxy_groups WHERE name = ?`, name)
	if err != nil {
		return nil, fmt.Errorf("get proxy group by name: %w", err)
	}
	return &g, nil
}

// Create inserts a new proxy group.
func (s *ProxyGroupStore) Create(g *model.ProxyGroup) error {
	now := time.Now().UTC()
	g.CreatedAt = now
	g.UpdatedAt = now
	_, err := s.db.Exec(
		`INSERT INTO proxy_groups (id, name, type, config, members, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		g.ID, g.Name, g.Type, g.Config, g.Members, g.SortOrder, g.CreatedAt, g.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create proxy group: %w", err)
	}
	return nil
}

// Update updates an existing proxy group.
func (s *ProxyGroupStore) Update(g *model.ProxyGroup) error {
	g.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(
		`UPDATE proxy_groups SET name = ?, type = ?, config = ?, members = ?, sort_order = ?, updated_at = ? WHERE id = ?`,
		g.Name, g.Type, g.Config, g.Members, g.SortOrder, g.UpdatedAt, g.ID,
	)
	if err != nil {
		return fmt.Errorf("update proxy group: %w", err)
	}
	return nil
}

// Delete removes a proxy group by ID.
func (s *ProxyGroupStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM proxy_groups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete proxy group: %w", err)
	}
	return nil
}

// Reorder updates the sort_order of proxy groups based on the provided ordered IDs.
func (s *ProxyGroupStore) Reorder(ids []string) error {
	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin reorder transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for i, id := range ids {
		_, err := tx.Exec(`UPDATE proxy_groups SET sort_order = ? WHERE id = ?`, i, id)
		if err != nil {
			return fmt.Errorf("reorder proxy group %s: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reorder: %w", err)
	}
	return nil
}

// Count returns the number of proxy groups.
func (s *ProxyGroupStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM proxy_groups`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count proxy groups: %w", err)
	}
	return count, nil
}
