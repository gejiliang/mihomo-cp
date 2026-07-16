package service

import (
	"encoding/json"
	"testing"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

func findError(errs []ValidationError, code string) *ValidationError {
	for i := range errs {
		if errs[i].Code == code {
			return &errs[i]
		}
	}
	return nil
}

func TestValidateDanglingReference(t *testing.T) {
	v := NewValidator()

	groups := []model.ProxyGroup{
		{
			Name:    "Auto",
			Type:    "url-test",
			Members: json.RawMessage(`["nonexistent-proxy"]`),
		},
	}

	errs := v.Validate(nil, groups, nil, nil)

	e := findError(errs, "DANGLING_REFERENCE")
	if e == nil {
		t.Fatalf("expected DANGLING_REFERENCE error, got: %v", errs)
	}
	if e.Level != "error" {
		t.Errorf("expected level error, got %q", e.Level)
	}
}

func TestValidateEmptyGroup(t *testing.T) {
	v := NewValidator()

	groups := []model.ProxyGroup{
		{
			Name:    "Empty",
			Type:    "select",
			Members: json.RawMessage(`[]`),
		},
	}

	errs := v.Validate(nil, groups, nil, nil)

	e := findError(errs, "EMPTY_GROUP")
	if e == nil {
		t.Fatalf("expected EMPTY_GROUP warning, got: %v", errs)
	}
	if e.Level != "warning" {
		t.Errorf("expected level warning, got %q", e.Level)
	}
}

func TestValidateMissingMatch(t *testing.T) {
	v := NewValidator()

	rules := []model.Rule{
		{Type: "DOMAIN-SUFFIX", Payload: "google.com", Target: "DIRECT"},
	}

	errs := v.Validate(nil, nil, rules, nil)

	e := findError(errs, "MISSING_MATCH")
	if e == nil {
		t.Fatalf("expected MISSING_MATCH warning, got: %v", errs)
	}
	if e.Level != "warning" {
		t.Errorf("expected level warning, got %q", e.Level)
	}
}

func TestValidateCircularRef(t *testing.T) {
	v := NewValidator()

	// Group A → Group B → Group A
	groups := []model.ProxyGroup{
		{
			Name:    "GroupA",
			Type:    "select",
			Members: json.RawMessage(`["GroupB"]`),
		},
		{
			Name:    "GroupB",
			Type:    "select",
			Members: json.RawMessage(`["GroupA"]`),
		},
	}

	errs := v.Validate(nil, groups, nil, nil)

	e := findError(errs, "CIRCULAR_REFERENCE")
	if e == nil {
		t.Fatalf("expected CIRCULAR_REFERENCE error, got: %v", errs)
	}
	if e.Level != "error" {
		t.Errorf("expected level error, got %q", e.Level)
	}
}

func TestValidateNoErrors(t *testing.T) {
	v := NewValidator()

	proxies := []model.Proxy{
		{Name: "jp-01", Type: "ss"},
	}
	groups := []model.ProxyGroup{
		{
			Name:    "Auto",
			Type:    "url-test",
			Members: json.RawMessage(`["jp-01"]`),
		},
	}
	rules := []model.Rule{
		{Type: "DOMAIN-SUFFIX", Payload: "google.com", Target: "Auto"},
		{Type: "MATCH", Target: "DIRECT"},
	}

	errs := v.Validate(proxies, groups, rules, nil)
	for _, e := range errs {
		if e.Level == "error" {
			t.Errorf("unexpected error: %+v", e)
		}
	}
}
