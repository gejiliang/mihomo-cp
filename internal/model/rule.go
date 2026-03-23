package model

import (
	"encoding/json"
	"time"
)

type Rule struct {
	ID        string          `json:"id" db:"id"`
	Type      string          `json:"type" db:"type"`
	Payload   string          `json:"payload" db:"payload"`
	Target    string          `json:"target" db:"target"`
	Params    json.RawMessage `json:"params" db:"params"`
	SortOrder int             `json:"sort_order" db:"sort_order"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}

var ValidRuleTypes = []string{
	"DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD",
	"IP-CIDR", "IP-CIDR6", "SRC-IP-CIDR",
	"DST-PORT", "PROCESS-NAME", "GEOIP",
	"RULE-SET", "MATCH",
}
