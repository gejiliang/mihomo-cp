package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gejiliang/mihomo-cp/internal/store"
)

func testPublisher(t *testing.T, controller http.HandlerFunc) (*PublishService, *store.PublishStore) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(controller)
	t.Cleanup(server.Close)
	ps := store.NewPublishStore(db)
	return NewPublishService(ps, nil, nil, NewMihomoClient(server.URL, "")), ps
}

func TestPublishAndRollbackPrivateConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	calls := 0
	svc, _ := testPublisher(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Path != path {
			t.Errorf("reload path=%q want %q", body.Path, path)
		}
		assertPrivateConfig(t, path, "new-secret")
		w.WriteHeader(204)
	})
	req := PublishRequest{ConfigPath: path, ConfigYAML: []byte("new-secret")}
	for _, existing := range []bool{false, true} {
		if existing {
			if err := os.Chmod(path, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		record, err := svc.Publish(req)
		if err != nil || record.Status != "success" {
			t.Fatalf("publish=%v %v", record, err)
		}
		assertPrivateConfig(t, path, "new-secret")
	}
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := svc.Rollback(path, filepath.Dir(path), "test")
	if err != nil || record.Status != "success" {
		t.Fatalf("rollback=%v %v", record, err)
	}
	assertPrivateConfig(t, path, "new-secret")
	if calls != 3 {
		t.Fatalf("reload calls=%d", calls)
	}
}

func TestPublishFailureRestoresPrivateConfig(t *testing.T) {
	for _, original := range []string{"old-secret", ""} {
		t.Run(map[bool]string{false: "nonempty", true: "empty"}[original == ""], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}
			calls := 0
			svc, _ := testPublisher(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body struct {
					Path string `json:"path"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Path != path {
					t.Errorf("recovery reloaded %q instead of config path", body.Path)
				}
				if calls == 1 {
					assertPrivateConfig(t, path, "rejected-secret")
					w.WriteHeader(500)
				} else {
					assertPrivateConfig(t, path, original)
					w.WriteHeader(204)
				}
			})
			record, err := svc.Publish(PublishRequest{ConfigPath: path, ConfigDir: filepath.Dir(path), ConfigYAML: []byte("rejected-secret")})
			if err != nil || record.Status != "failed" || calls != 2 {
				t.Fatalf("publish=%v %v calls=%d", record, err, calls)
			}
			assertPrivateConfig(t, path, original)
		})
	}
}

func TestFailedFirstPublishRestoresAbsence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	svc, _ := testPublisher(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	record, err := svc.Publish(PublishRequest{ConfigPath: path, ConfigYAML: []byte("rejected")})
	if err != nil || record.Status != "failed" {
		t.Fatalf("publish=%v %v", record, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed first publish left config: %v", err)
	}
}

func TestPublishReportsRestoreFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged directory access checks")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	svc, ps := testPublisher(t, func(w http.ResponseWriter, r *http.Request) {
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Error(err)
		}
		w.WriteHeader(500)
	})
	record, err := svc.Publish(PublishRequest{ConfigPath: path, ConfigYAML: []byte("rejected")})
	if err != nil || record.Status != "failed" || !strings.Contains(record.ErrorMsg, "restore config:") {
		t.Fatalf("publish=%v %v", record, err)
	}
	assertPrivateConfig(t, path, "rejected")
	saved, err := ps.GetByID(record.ID)
	if err != nil || saved.ErrorMsg != record.ErrorMsg {
		t.Fatalf("restore error not persisted: %v %v", saved, err)
	}
}

func TestPublishWriteFailureDoesNotReload(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged directory access checks")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, ps := testPublisher(t, func(w http.ResponseWriter, r *http.Request) { t.Error("reload on failed write") })
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	if _, err := svc.Publish(PublishRequest{ConfigPath: path, ConfigYAML: []byte("new")}); err == nil {
		t.Fatal("expected write failure")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "original" {
		t.Fatal("original modified")
	}
	records, err := ps.List(10)
	if err != nil || len(records) != 0 {
		t.Fatalf("unexpected publish record: %v %v", records, err)
	}
}

func TestValidateWithMihomoPrivateConfig(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-mihomo")
	// A fake local executable inspects the private validation file only.
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nstat -c '%a' \"$3/config.yaml\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	output, err := (&PublishService{}).ValidateWithMihomo([]byte("secret: fake\n"), dir, binary)
	if err != nil || strings.TrimSpace(output) != "600" {
		t.Fatalf("validation config permissions = %q, %v", output, err)
	}
}
