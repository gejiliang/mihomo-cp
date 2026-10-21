package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// NullTime is a time column that may be NULL (e.g. "never refreshed").
// The zero value scans NULL cleanly and marshals to JSON null.
type NullTime struct {
	Time  time.Time
	Valid bool
}

// NewNullTime wraps a known time.
func NewNullTime(t time.Time) NullTime {
	return NullTime{Time: t, Valid: true}
}

// IsZero reports the "never / unset" state.
func (t NullTime) IsZero() bool {
	return !t.Valid || t.Time.IsZero()
}

// Scan implements sql.Scanner.
func (t *NullTime) Scan(v any) error {
	if v == nil {
		t.Time, t.Valid = time.Time{}, false
		return nil
	}
	switch val := v.(type) {
	case time.Time:
		t.Time, t.Valid = val, true
		return nil
	case []byte:
		return t.parse(string(val))
	case string:
		return t.parse(val)
	default:
		return fmt.Errorf("unsupported type for NullTime: %T", v)
	}
}

// SQLite stores datetimes as strings in a few accepted layouts.
func (t *NullTime) parse(s string) error {
	s = trim(s)
	if s == "" {
		t.Time, t.Valid = time.Time{}, false
		return nil
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	var lastErr error
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, s)
		if err == nil {
			t.Time, t.Valid = parsed, true
			return nil
		}
		lastErr = err
	}
	return lastErr
}

// utcStampLayout is fixed-width (nanosecond precision, always UTC) so SQLite
// DATETIME string comparison matches chronological order exactly.
const utcStampLayout = "2006-01-02T15:04:05.000000000Z07:00"

// UTCStamp renders a time in the canonical comparison format (always UTC).
// Use it for every SQL time comparison so parameter and column forms agree.
func UTCStamp(t time.Time) string {
	return t.UTC().Format(utcStampLayout)
}

// Value implements driver.Valuer: zero → NULL, otherwise the canonical UTC
// stamp. All SQL time comparisons in this codebase pass timestamps through
// utcStamp too, so stored and compared forms always agree.
func (t NullTime) Value() (driver.Value, error) {
	if t.IsZero() {
		return nil, nil
	}
	return UTCStamp(t.Time), nil
}

// MarshalJSON renders RFC3339 or null.
func (t NullTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.Time.Format(time.RFC3339))
}

// UnmarshalJSON accepts RFC3339 or null.
func (t *NullTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		t.Time, t.Valid = time.Time{}, false
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}
	t.Time, t.Valid = parsed, true
	return nil
}

// NullJSON is a nullable JSON document column. Nil Bytes scans/writes NULL
// and marshals to JSON null.
type NullJSON struct {
	Bytes []byte
}

// Scan implements sql.Scanner.
func (j *NullJSON) Scan(v any) error {
	if v == nil {
		j.Bytes = nil
		return nil
	}
	switch val := v.(type) {
	case []byte:
		if len(val) == 0 {
			j.Bytes = nil
			return nil
		}
		j.Bytes = append([]byte(nil), val...)
		return nil
	case string:
		if val == "" {
			j.Bytes = nil
			return nil
		}
		j.Bytes = []byte(val)
		return nil
	default:
		return fmt.Errorf("unsupported type for NullJSON: %T", v)
	}
}

// Value implements driver.Valuer: empty → NULL.
func (j NullJSON) Value() (driver.Value, error) {
	if len(j.Bytes) == 0 {
		return nil, nil
	}
	return string(j.Bytes), nil
}

// MarshalJSON renders the stored document or null.
func (j NullJSON) MarshalJSON() ([]byte, error) {
	if len(j.Bytes) == 0 {
		return []byte("null"), nil
	}
	out := make([]byte, len(j.Bytes))
	copy(out, j.Bytes)
	return out, nil
}

// UnmarshalJSON stores any JSON document verbatim.
func (j *NullJSON) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		j.Bytes = nil
		return nil
	}
	j.Bytes = append([]byte(nil), data...)
	return nil
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
