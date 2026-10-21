package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// NodeScoreStore persists node scoring state and the per-egress-IP score
// cache. Rows double as the durable scheduler queue (status + next_due).
type NodeScoreStore struct {
	db *DB
}

// NewNodeScoreStore creates a new NodeScoreStore.
func NewNodeScoreStore(db *DB) *NodeScoreStore {
	return &NodeScoreStore{db: db}
}

// List returns all node score rows ordered by node key.
func (s *NodeScoreStore) List() ([]model.NodeScore, error) {
	var rows []model.NodeScore
	if err := s.db.Select(&rows, `SELECT * FROM node_scores ORDER BY node_key`); err != nil {
		return nil, fmt.Errorf("list node scores: %w", err)
	}
	return rows, nil
}

// Get returns one row by node key, or ErrNotFound.
func (s *NodeScoreStore) Get(nodeKey string) (*model.NodeScore, error) {
	var row model.NodeScore
	err := s.db.Get(&row, `SELECT * FROM node_scores WHERE node_key = ?`, nodeKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get node score: %w", err)
	}
	return &row, nil
}

// Upsert inserts or replaces a node score row (keeping created_at stable).
func (s *NodeScoreStore) Upsert(row *model.NodeScore) error {
	now := model.NewNullTime(time.Now().UTC())
	row.UpdatedAt = now
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	_, err := s.db.Exec(
		`INSERT INTO node_scores (
			node_key, source, node_name, provider_name, status, fingerprint, claude_score,
			claude_ip_match, claude_response_ip, lookup_match, egress_ip, exit_ip_source,
			ip_info, error_code, checked_at, last_success_at, next_due, attempts, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(node_key) DO UPDATE SET
			source = excluded.source, node_name = excluded.node_name, provider_name = excluded.provider_name,
			status = excluded.status, fingerprint = excluded.fingerprint, claude_score = excluded.claude_score,
			claude_ip_match = excluded.claude_ip_match, claude_response_ip = excluded.claude_response_ip,
			lookup_match = excluded.lookup_match, egress_ip = excluded.egress_ip,
			exit_ip_source = excluded.exit_ip_source, ip_info = excluded.ip_info,
			error_code = excluded.error_code, checked_at = excluded.checked_at,
			last_success_at = excluded.last_success_at, next_due = excluded.next_due,
			attempts = excluded.attempts, last_error = excluded.last_error, updated_at = excluded.updated_at`,
		row.NodeKey, row.Source, row.NodeName, row.ProviderName, row.Status, row.Fingerprint, row.ClaudeScore,
		row.ClaudeIPMatch, row.ClaudeResponseIP, row.LookupMatch, row.EgressIP, row.ExitIPSource,
		row.IPInfo, row.ErrorCode, row.CheckedAt, row.LastSuccessAt,
		row.NextDue, row.Attempts, row.LastError, row.CreatedAt, row.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert node score: %w", err)
	}
	return nil
}

// Delete removes a node score row.
func (s *NodeScoreStore) Delete(nodeKey string) error {
	_, err := s.db.Exec(`DELETE FROM node_scores WHERE node_key = ?`, nodeKey)
	if err != nil {
		return fmt.Errorf("delete node score: %w", err)
	}
	return nil
}

// DeleteByProvider removes all score rows belonging to one provider.
func (s *NodeScoreStore) DeleteByProvider(providerName string) error {
	_, err := s.db.Exec(`DELETE FROM node_scores WHERE source = 'provider' AND provider_name = ?`, providerName)
	if err != nil {
		return fmt.Errorf("delete node scores by provider: %w", err)
	}
	return nil
}

// ListDue returns up to limit rows whose next_due has passed and that are not
// currently being tested, oldest due first.
func (s *NodeScoreStore) ListDue(now time.Time, limit int) ([]model.NodeScore, error) {
	var rows []model.NodeScore
	err := s.db.Select(&rows,
		`SELECT * FROM node_scores
		 WHERE status IN ('pending', 'failed') AND (next_due IS NULL OR next_due <= ?)
		 ORDER BY next_due ASC LIMIT ?`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list due node scores: %w", err)
	}
	return rows, nil
}

// Claim atomically flips a row from pending/failed to testing. Returns false
// if the row no longer qualifies (e.g. another worker claimed it).
func (s *NodeScoreStore) Claim(nodeKey string, now time.Time) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE node_scores SET status = 'testing', checked_at = ?, updated_at = ?
		 WHERE node_key = ? AND status IN ('pending', 'failed')`,
		now, now, nodeKey,
	)
	if err != nil {
		return false, fmt.Errorf("claim node score: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ResetTesting flips rows stuck in "testing" (e.g. after a crash) back to
// pending, due immediately. Returns the number of rows reset.
func (s *NodeScoreStore) ResetTesting(now time.Time) (int64, error) {
	res, err := s.db.Exec(
		`UPDATE node_scores SET status = 'pending', next_due = ?, updated_at = ? WHERE status = 'testing'`,
		now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("reset testing node scores: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// Count returns (total, scored) row counts.
func (s *NodeScoreStore) Count() (total int, scored int, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN claude_score IS NOT NULL THEN 1 ELSE 0 END), 0) FROM node_scores`).Scan(&total, &scored)
	if err != nil {
		return 0, 0, fmt.Errorf("count node scores: %w", err)
	}
	return total, scored, nil
}

// ── Egress IP cache ──────────────────────────────────────────────────────────

// EgressIPScore is a cached scoring result keyed by the actual egress IP.
type EgressIPScore struct {
	EgressIP    string         `json:"egress_ip" db:"egress_ip"`
	ClaudeScore *int           `json:"claude_score,omitempty" db:"claude_score"`
	IPInfo      model.NullJSON `json:"ip_info,omitempty" db:"ip_info"`
	FetchedAt   model.NullTime `json:"fetched_at" db:"fetched_at"`
}

// GetEgressIPScore returns the cached score for an egress IP, or ErrNotFound.
func (s *NodeScoreStore) GetEgressIPScore(ip string) (*EgressIPScore, error) {
	var row EgressIPScore
	err := s.db.Get(&row, `SELECT * FROM egress_ip_scores WHERE egress_ip = ?`, ip)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get egress ip score: %w", err)
	}
	return &row, nil
}

// PutEgressIPScore inserts or refreshes the cache entry for an egress IP.
func (s *NodeScoreStore) PutEgressIPScore(row *EgressIPScore) error {
	row.FetchedAt = model.NewNullTime(time.Now().UTC())
	_, err := s.db.Exec(
		`INSERT INTO egress_ip_scores (egress_ip, claude_score, ip_info, fetched_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(egress_ip) DO UPDATE SET claude_score = excluded.claude_score, ip_info = excluded.ip_info, fetched_at = excluded.fetched_at`,
		row.EgressIP, row.ClaudeScore, row.IPInfo, row.FetchedAt,
	)
	if err != nil {
		return fmt.Errorf("put egress ip score: %w", err)
	}
	return nil
}

// NewNodeKey builds the stable queue key for a node.
func NewNodeKey(source, providerName, nodeName string) string {
	if source == "provider" {
		return "provider/" + providerName + "/" + nodeName
	}
	return "static/" + nodeName
}
