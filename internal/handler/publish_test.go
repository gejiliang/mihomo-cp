package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
)

func TestDiscardPrivateConfig(t *testing.T) {
	for _, failWrite := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "write-failure"}[failWrite], func(t *testing.T) {
			if failWrite && os.Geteuid() == 0 {
				t.Skip("requires unprivileged directory access checks")
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			original := "mixed-port: 7890\nsecret: test-only-secret\n"
			if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}
			db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := db.Migrate(); err != nil {
				t.Fatal(err)
			}
			settings := store.NewSettingsStore(db)
			st, err := settings.Get()
			if err != nil {
				t.Fatal(err)
			}
			st.MihomoConfig = path
			if err := settings.Update(st); err != nil {
				t.Fatal(err)
			}
			if err := settings.SetRawConfigDraft("secret: discarded-draft"); err != nil {
				t.Fatal(err)
			}
			h := NewPublishHandler(store.NewProxyStore(db), store.NewProxyGroupStore(db), store.NewRuleStore(db), store.NewRuleProviderStore(db), store.NewConfigStore(db), store.NewPublishStore(db), settings, service.NewConfigService(), nil, nil, service.NewImportService())
			if failWrite {
				if err := os.Chmod(dir, 0o500); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(dir, 0o700)
			}
			rec := httptest.NewRecorder()
			h.Discard(rec, httptest.NewRequest(http.MethodPost, "/api/publish/discard", nil))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if failWrite {
				if rec.Code != 500 || string(data) != original || info.Mode().Perm() != 0o644 {
					t.Fatalf("failed discard changed file: status %d content %q mode %v", rec.Code, data, info.Mode())
				}
			} else {
				if rec.Code != 200 || info.Mode().Perm() != 0o600 {
					t.Fatalf("discard status %d mode %v", rec.Code, info.Mode())
				}
				parsed, err := service.NewImportService().ParseConfig(data)
				if err != nil || len(parsed.SystemConfig) == 0 {
					t.Fatalf("invalid rewritten config: %v %v", parsed, err)
				}
			}
		})
	}
}
