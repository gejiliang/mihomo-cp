package model

import (
	"encoding/json"
	"time"
)

type ProxyGroup struct {
	ID        string          `json:"id" db:"id"`
	Name      string          `json:"name" db:"name"`
	Type      string          `json:"type" db:"type"`
	Config    json.RawMessage `json:"config" db:"config"`
	Members   json.RawMessage `json:"members" db:"members"`
	SortOrder int             `json:"sort_order" db:"sort_order"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}

var ValidGroupTypes = []string{"select", "fallback", "url-test", "load-balance", "relay"}
