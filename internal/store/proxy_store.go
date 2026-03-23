package store

import (
	"fmt"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// ProxyStore provides CRUD operations for proxies.
type ProxyStore struct {
	db *DB
}

// NewProxyStore creates a new ProxyStore.
func NewProxyStore(db *DB) *ProxyStore {
	return &ProxyStore{db: db}
}

// List returns proxies with optional search and type filters, ordered by sort_order.
func (s *ProxyStore) List(search, proxyType string) ([]*model.Proxy, error) {
	query := `SELECT id, name, type, config, sort_order, created_at, updated_at FROM proxies WHERE 1=1`
	args := []any{}

	if search != "" {
		query += ` AND name LIKE ?`
		args = append(args, "%"+search+"%")
	}
	if proxyType != "" {
		query += ` AND type = ?`
		args = append(args, proxyType)
	}
	query += ` ORDER BY sort_order`

	var proxies []*model.Proxy
	err := s.db.Select(&proxies, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list proxies: %w", err)
	}
	return proxies, nil
}

// GetByID retrieves a proxy by ID.
func (s *ProxyStore) GetByID(id string) (*model.Proxy, error) {
	var p model.Proxy
	err := s.db.Get(&p, `SELECT id, name, type, config, sort_order, created_at, updated_at FROM proxies WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("get proxy by id: %w", err)
	}
	return &p, nil
}

// GetByName retrieves a proxy by name.
func (s *ProxyStore) GetByName(name string) (*model.Proxy, error) {
	var p model.Proxy
	err := s.db.Get(&p, `SELECT id, name, type, config, sort_order, created_at, updated_at FROM proxies WHERE name = ?`, name)
	if err != nil {
		return nil, fmt.Errorf("get proxy by name: %w", err)
	}
	return &p, nil
}

// Create inserts a new proxy.
func (s *ProxyStore) Create(p *model.Proxy) error {
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	_, err := s.db.Exec(
		`INSERT INTO proxies (id, name, type, config, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Type, p.Config, p.SortOrder, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create proxy: %w", err)
	}
	return nil
}

// Update updates an existing proxy.
func (s *ProxyStore) Update(p *model.Proxy) error {
	p.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(
		`UPDATE proxies SET name = ?, type = ?, config = ?, sort_order = ?, updated_at = ? WHERE id = ?`,
		p.Name, p.Type, p.Config, p.SortOrder, p.UpdatedAt, p.ID,
	)
	if err != nil {
		return fmt.Errorf("update proxy: %w", err)
	}
	return nil
}

// Delete removes a proxy by ID.
func (s *ProxyStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM proxies WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete proxy: %w", err)
	}
	return nil
}

// Reorder updates the sort_order of proxies based on the provided ordered IDs.
func (s *ProxyStore) Reorder(ids []string) error {
	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin reorder transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for i, id := range ids {
		_, err := tx.Exec(`UPDATE proxies SET sort_order = ? WHERE id = ?`, i, id)
		if err != nil {
			return fmt.Errorf("reorder proxy %s: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reorder: %w", err)
	}
	return nil
}

// Count returns the number of proxies.
func (s *ProxyStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM proxies`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count proxies: %w", err)
	}
	return count, nil
}

