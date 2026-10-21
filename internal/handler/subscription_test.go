package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gejiliang/mihomo-cp/internal/service"
)

func TestWriteSubErrorReferencedReturns409WithGroups(t *testing.T) {
	rec := httptest.NewRecorder()
	writeSubError(rec, &service.ErrReferenced{References: []string{"AI-Group", "Manual-Group"}})

	if rec.Code != 409 {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	var body struct {
		Error struct {
			Code       string   `json:"code"`
			References []string `json:"references"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error envelope: %v; body=%s", err, rec.Body.String())
	}
	if body.Error.Code != "referenced" {
		t.Errorf("error code = %q, want referenced", body.Error.Code)
	}
	if len(body.Error.References) != 2 || body.Error.References[0] != "AI-Group" || body.Error.References[1] != "Manual-Group" {
		t.Errorf("references = %#v, want both proxy groups", body.Error.References)
	}
}
