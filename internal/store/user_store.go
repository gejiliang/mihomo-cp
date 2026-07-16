package store

import (
	"fmt"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// UserStore provides CRUD operations for users.
type UserStore struct {
	db *DB
}

// NewUserStore creates a new UserStore.
func NewUserStore(db *DB) *UserStore {
	return &UserStore{db: db}
}

// GetByUsername retrieves a user by username.
func (s *UserStore) GetByUsername(username string) (*model.User, error) {
	var u model.User
	err := s.db.Get(&u, `SELECT id, username, password, role, created_at, updated_at FROM users WHERE username = ?`, username)
	if err != nil {
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return &u, nil
}

// GetByID retrieves a user by ID.
func (s *UserStore) GetByID(id string) (*model.User, error) {
	var u model.User
	err := s.db.Get(&u, `SELECT id, username, password, role, created_at, updated_at FROM users WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

// List returns all users.
func (s *UserStore) List() ([]*model.User, error) {
	var users []*model.User
	err := s.db.Select(&users, `SELECT id, username, password, role, created_at, updated_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

// Create inserts a new user.
func (s *UserStore) Create(u *model.User) error {
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now
	_, err := s.db.Exec(
		`INSERT INTO users (id, username, password, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.Password, u.Role, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// Update updates an existing user.
func (s *UserStore) Update(u *model.User) error {
	u.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(
		`UPDATE users SET username = ?, password = ?, role = ?, updated_at = ? WHERE id = ?`,
		u.Username, u.Password, u.Role, u.UpdatedAt, u.ID,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

// Delete removes a user by ID.
func (s *UserStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

// Count returns the number of users.
func (s *UserStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}
