package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"encoding/json"
	"fmt"

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

func (e *subscriptionAuthEnv) doAs(t *testing.T, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	e.srv.mux.ServeHTTP(rec, req)
	return rec
}

func (e *subscriptionAuthEnv) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return e.doAs(t, e.token, method, path, body)
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

func TestDuplicateProviderCreateAndUpdateConflict(t *testing.T) {
	env := newSubscriptionAuthEnv(t)
	admin, err := env.srv.authSvc.GenerateAccessToken("admin-user", "admin")
	if err != nil {
		t.Fatal(err)
	}

	rec := env.doAs(t, admin, "POST", "/api/subscriptions", `{
		"name":"Other Sub","url":"https://sub.example.com/other",
		"provider_name":"other-provider","reset_day":10,"reset_hour":12,"timezone":"Asia/Shanghai"
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create other subscription = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Data.ID == "" {
		t.Fatalf("decode created subscription: %v; body=%s", err, rec.Body.String())
	}

	// Rebinding an existing subscription to a provider already linked to
	// another subscription must conflict, not create a second link.
	rec = env.doAs(t, admin, "PUT", "/api/subscriptions/"+created.Data.ID, `{"provider_name":"example-provider"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("update to already-linked provider = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}

	// Creating a second subscription against the same provider without a URL
	// must also conflict instead of silently sharing it.
	rec = env.doAs(t, admin, "POST", "/api/subscriptions", `{"name":"Duplicate Link","provider_name":"other-provider"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("duplicate provider link create = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}

	// Race two creates for a brand-new provider. The requests use DIFFERENT
	// dummy URLs: exactly one may win, and the losing request must not
	// overwrite the winner's native provider URL.
	type raceResult struct {
		name string
		url  string
		code int
		body string
	}
	results := make(chan raceResult, 2)
	var wg sync.WaitGroup
	for i, name := range []string{"Race One", "Race Two"} {
		url := fmt.Sprintf("https://sub.example.com/race-%d", i+1)
		wg.Add(1)
		go func(name, url string) {
			defer wg.Done()
			body := `{"name":"` + name + `","url":"` + url + `","provider_name":"race-provider","reset_day":10,"reset_hour":12,"timezone":"Asia/Shanghai"}`
			res := env.doAs(t, admin, "POST", "/api/subscriptions", body)
			results <- raceResult{name: name, url: url, code: res.Code, body: res.Body.String()}
		}(name, url)
	}
	wg.Wait()
	close(results)

	createdCount, conflictCount := 0, 0
	winnerURL, loserURL := "", ""
	for result := range results {
		switch result.code {
		case http.StatusCreated:
			createdCount++
			winnerURL = result.url
		case http.StatusConflict:
			conflictCount++
			loserURL = result.url
		default:
			t.Errorf("concurrent duplicate create %s = %d %s, want 201 or 409", result.name, result.code, result.body)
		}
	}
	if createdCount != 1 || conflictCount != 1 {
		t.Errorf("concurrent duplicate provider creates = %d created, %d conflicts; want 1/1", createdCount, conflictCount)
	}
	if winnerURL != "" {
		draftURL, ok := draftProviderURL(t, env, "race-provider")
		if !ok {
			t.Error("winning create has no native provider entry in the draft")
		} else if draftURL != winnerURL {
			t.Errorf("draft provider URL = %q, want winner %q", draftURL, winnerURL)
		}
		if loserURL != "" && draftURL == loserURL {
			t.Errorf("losing create overwrote native provider URL with %q", loserURL)
		}
		subs := store.NewSubscriptionStore(env.srv.db)
		sub, err := subs.GetByProvider("race-provider")
		if err != nil {
			t.Errorf("winning subscription metadata missing: %v", err)
		} else if sub.Name != "Race One" && sub.Name != "Race Two" {
			t.Errorf("unexpected metadata owner %q", sub.Name)
		}
	}

	// Two distinct-provider creates racing each other must both persist their
	// native provider entry AND their subscription metadata.
	distinct := make(chan raceResult, 2)
	for i, name := range []string{"Distinct One", "Distinct Two"} {
		providerName := fmt.Sprintf("distinct-provider-%d", i+1)
		url := fmt.Sprintf("https://sub.example.com/distinct-%d", i+1)
		wg.Add(1)
		go func(name, providerName, url string) {
			defer wg.Done()
			body := `{"name":"` + name + `","url":"` + url + `","provider_name":"` + providerName + `","reset_day":10,"reset_hour":12,"timezone":"Asia/Shanghai"}`
			res := env.doAs(t, admin, "POST", "/api/subscriptions", body)
			distinct <- raceResult{name: name, url: url, code: res.Code, body: res.Body.String()}
		}(name, providerName, url)
	}
	wg.Wait()
	close(distinct)
	subs := store.NewSubscriptionStore(env.srv.db)
	for result := range distinct {
		if result.code != http.StatusCreated {
			t.Errorf("distinct create %s = %d %s, want 201", result.name, result.code, result.body)
			continue
		}
		providerName := ""
		switch result.name {
		case "Distinct One":
			providerName = "distinct-provider-1"
		case "Distinct Two":
			providerName = "distinct-provider-2"
		}
		if got, ok := draftProviderURL(t, env, providerName); !ok || got != result.url {
			t.Errorf("distinct provider %s draft URL = %q/%v, want %q", providerName, got, ok, result.url)
		}
		if sub, err := subs.GetByProvider(providerName); err != nil {
			t.Errorf("distinct provider %s metadata missing: %v", providerName, err)
		} else if sub.Name != result.name {
			t.Errorf("distinct provider %s metadata owner = %q, want %q", providerName, sub.Name, result.name)
		}
	}

	// A failed create with an invalid traffic_provider_id must leave no
	// orphan native provider entry or subscription metadata.
	rec = env.doAs(t, admin, "POST", "/api/subscriptions", `{
		"name":"Orphan Test","url":"https://sub.example.com/orphan",
		"provider_name":"orphan-provider","traffic_provider_id":"missing-traffic-provider"
	}`)
	if rec.Code < 400 {
		t.Errorf("invalid traffic_provider_id create = %d, want failure; body=%s", rec.Code, rec.Body.String())
	}
	if url, ok := draftProviderURL(t, env, "orphan-provider"); ok {
		t.Errorf("failed create left an orphan provider entry with URL %q", url)
	}
	if sub, err := subs.GetByName("Orphan Test"); err == nil {
		t.Errorf("failed create left orphan subscription metadata: %+v", sub)
	}
}

func draftProviderURL(t *testing.T, env *subscriptionAuthEnv, name string) (string, bool) {
	t.Helper()
	raw, err := store.NewConfigStore(env.srv.db).Get()
	if err != nil {
		t.Fatalf("read draft config: %v", err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("decode draft config: %v", err)
	}
	var providers map[string]map[string]any
	if rawProviders, ok := root["proxy-providers"]; ok {
		if err := json.Unmarshal(rawProviders, &providers); err != nil {
			t.Fatalf("decode proxy-providers: %v", err)
		}
	}
	entry, ok := providers[name]
	if !ok {
		return "", false
	}
	url, _ := entry["url"].(string)
	return url, true
}
