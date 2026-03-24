package model

import "time"

type PublishRecord struct {
	ID         string    `json:"id" db:"id"`
	Version    string    `json:"version" db:"version"`
	ConfigYAML string    `json:"config_yaml" db:"config_yaml"`
	DiffText   string    `json:"diff_text" db:"diff_text"`
	Status     string    `json:"status" db:"status"`
	ErrorMsg   string    `json:"error_msg" db:"error_msg"`
	Operator   string    `json:"operator" db:"operator"`
	Note       string    `json:"note" db:"note"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}
