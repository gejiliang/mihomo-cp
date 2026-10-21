package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"encoding/json"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/store"
)

const (
	testSubToken      = "SUB_TOKEN_DO_NOT_LEAK"
	testTrafficToken  = "TRAFFIC_TOKEN_DO_NOT_LEAK"
	testSettingsToken = "SETTINGS_SECRET_DO_NOT_LEAK"
)

type subscriptionAuthEnv struct {
	srv   *Server
	token string
}

func newSubscriptionAuthEnv(t *testing.T) *subscriptionAuthEnv {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	s := New(Config{Host: "127.0.0.1", Port: 0}, db)
	token, err := s.authSvc.GenerateAccessToken("readonly-user", "readonly")
	if err != nil {
		t.Fatalf("generate readonly token: %v", err)
	}

	configs := store.NewConfigStore(db)
	if err := configs.Update(json.RawMessage(`{
		"proxy-providers": {
			"example-provider": {
				"type": "http",
				"url": "https://sub.example.com/` + testSubToken + `",
				"path": "providers/example.yaml",
				"interval": 86400
			}
		},
		"rules": ["MATCH,DIRECT"]
	}`)); err != nil {
		t.Fatalf("seed config draft: %v", err)
	}

	settings := store.NewSettingsStore(db)
	st, err := settings.Get()
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	st.ExtSecret = testSettingsToken
	st.RawConfigDraft = "proxies: []\n# " + testSubToken
	if err := settings.Update(st); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	if err := settings.SetRawConfigDraft(st.RawConfigDraft); err != nil {
		t.Fatalf("set raw draft: %v", err)
	}

	subs := store.NewSubscriptionStore(db)
	if err := subs.Create(&model.Subscription{
		ID: "sub-1", Name: "Example Sub", Enabled: true,
		ProviderName: "example-provider", ProviderSource: "created",
		ResetDay: 10, ResetHour: 12, ResetTimezone: "Asia/Shanghai",
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	traffic := store.NewTrafficStore(db)
	if err := traffic.CreateProvider(&model.TrafficProvider{
		ID: "tp-1", Name: "Example Traffic", QuotaBytes: 200 * 1024 * 1024 * 1024,
		ResetDay: 10, ResetHour: 12, ResetTimezone: "Asia/Shanghai",
		NodePatternList: []string{"*"}, Enabled: true,
		SubscriptionURL: "https://traffic.example.com/" + testTrafficToken,
	}); err != nil {
		t.Fatalf("seed traffic provider: %v", err)
	}
	if err := traffic.UpdateSettings(&model.TrafficSettings{
		ID: 1, AlertChannel: "webhook",
		AlertTarget:     "https://hooks.example.com/" + testTrafficToken,
		AlertThresholds: "[80,95]",
	}); err != nil {
		t.Fatalf("seed traffic settings: %v", err)
	}

	scores := store.NewNodeScoreStore(db)
	if err := scores.Upsert(&model.NodeScore{
		NodeKey: "static/Example-Node", Source: "static", NodeName: "Example-Node",
		Status: model.ScoreStatusOK, ClaudeScore: intPtr(80),
		NextDue: model.NewNullTime(time.Now().Add(7 * 24 * time.Hour)),
	}); err != nil {
		t.Fatalf("seed node score: %v", err)
	}

	publish := store.NewPublishStore(db)
	if err := publish.Create(&model.PublishRecord{
		ID: "hist-1", Version: "v1", Status: "success",
		ConfigYAML: "proxies: []\n# " + testSubToken,
		DiffText:   "secret: " + testSubToken,
	}); err != nil {
		t.Fatalf("seed publish history: %v", err)
	}

	return &subscriptionAuthEnv{srv: s, token: token}
}

func intPtr(v int) *int { return &v }

func (e *subscriptionAuthEnv) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+e.token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	e.srv.mux.ServeHTTP(rec, req)
	return rec
}

func assertNoSecrets(t *testing.T, body string) {
	t.Helper()
	for _, token := range []string{testSubToken, testTrafficToken, testSettingsToken} {
		if strings.Contains(body, token) {
			t.Errorf("response leaked secret %q: %s", token, body)
		}
	}
}

func TestReadonlyCannotMutateSubscriptionsScoresOrAlternatePaths(t *testing.T) {
	env := newSubscriptionAuthEnv(t)
	cases := []struct {
		method string
		path   string
	}{
		{"POST", "/api/subscriptions"},
		{"PUT", "/api/subscriptions/sub-1"},
		{"DELETE", "/api/subscriptions/sub-1"},
		{"POST", "/api/subscriptions/sub-1/refresh"},
		{"POST", "/api/node-scores/refresh"},
		{"POST", "/api/node-scores/static%2FExample-Node/refresh"},

		{"POST", "/api/proxies"},
		{"PUT", "/api/proxies/proxy-1"},
		{"DELETE", "/api/proxies/proxy-1"},
		{"POST", "/api/proxies/proxy-1/copy"},
		{"POST", "/api/proxies/reorder"},
		{"POST", "/api/proxy-groups"},
		{"PUT", "/api/proxy-groups/group-1"},
		{"DELETE", "/api/proxy-groups/group-1"},
		{"POST", "/api/proxy-groups/group-1/copy"},
		{"POST", "/api/proxy-groups/reorder"},
		{"POST", "/api/rules"},
		{"PUT", "/api/rules/rule-1"},
		{"DELETE", "/api/rules/rule-1"},
		{"POST", "/api/rules/reorder"},
		{"POST", "/api/rule-providers"},
		{"PUT", "/api/rule-providers/rp-1"},
		{"DELETE", "/api/rule-providers/rp-1"},
		{"POST", "/api/rule-providers/rp-1/refresh"},
		{"PUT", "/api/rule-providers/rp-1/content"},
		{"PUT", "/api/system-config"},

		{"PUT", "/api/settings"},
		{"PUT", "/api/settings/config-yaml"},
		{"DELETE", "/api/settings/config-yaml"},
		{"POST", "/api/import/config"},
		{"POST", "/api/publish/validate"},
		{"POST", "/api/publish"},
		{"POST", "/api/publish/discard"},
		{"POST", "/api/publish/rollback"},

		{"POST", "/api/traffic/providers"},
		{"PUT", "/api/traffic/providers/tp-1"},
		{"DELETE", "/api/traffic/providers/tp-1"},
		{"PUT", "/api/traffic/settings"},
		{"POST", "/api/traffic/test-notify"},
		{"DELETE", "/api/runtime/connections/conn-1"},
		{"PUT", "/api/runtime/proxies/group/selected"},
		{"PUT", "/api/runtime/providers/rules/name"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := env.do(t, tc.method, tc.path, `{}`)
			if rec.Code != http.StatusForbidden {
				t.Errorf("readonly %s %s = %d, want 403; body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestReadonlySensitiveReadsAreForbiddenOrRedacted(t *testing.T) {
	env := newSubscriptionAuthEnv(t)
	paths := []string{
		"/api/system-config",
		"/api/settings",
		"/api/settings/config-yaml",
		"/api/publish/preview",
		"/api/publish/history",
		"/api/publish/history/hist-1",
		"/api/traffic/providers",
		"/api/traffic/settings",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			rec := env.do(t, "GET", path, "")
			switch rec.Code {
			case http.StatusForbidden:
				return
			case http.StatusOK:
				assertNoSecrets(t, rec.Body.String())
			default:
				t.Errorf("readonly GET %s = %d, want 403 or redacted 200; body=%s", path, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestReadonlySubscriptionScoreAndTrafficStatusRemainSafe(t *testing.T) {
	env := newSubscriptionAuthEnv(t)
	paths := []string{
		"/api/subscriptions",
		"/api/subscriptions/sub-1",
		"/api/subscriptions/providers",
		"/api/node-scores",
		"/api/node-scores/static%2FExample-Node",
		"/api/traffic/summary",
		"/api/traffic/history",
		"/api/publish/status",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			rec := env.do(t, "GET", path, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("readonly GET %s = %d, want readable 200; body=%s", path, rec.Code, rec.Body.String())
			}
			assertNoSecrets(t, rec.Body.String())
		})
	}
}
