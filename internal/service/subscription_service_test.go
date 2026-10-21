package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/store"
)

type subscriptionTestEnv struct {
	db       *store.DB
	subs     *store.SubscriptionStore
	configs  *store.ConfigStore
	traffic  *store.TrafficStore
	scores   *store.NodeScoreStore
	groups   *store.ProxyGroupStore
	settings *store.SettingsStore
	svc      *SubscriptionService
	now      time.Time
}

func newSubscriptionTestEnv(t *testing.T) *subscriptionTestEnv {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "subscriptions.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	env := &subscriptionTestEnv{
		db:       db,
		subs:     store.NewSubscriptionStore(db),
		configs:  store.NewConfigStore(db),
		traffic:  store.NewTrafficStore(db),
		scores:   store.NewNodeScoreStore(db),
		groups:   store.NewProxyGroupStore(db),
		settings: store.NewSettingsStore(db),
		now:      time.Date(2026, time.October, 20, 0, 0, 0, 0, time.UTC),
	}
	env.svc = NewSubscriptionService(
		env.subs, env.configs, env.traffic, env.scores, env.groups, nil, env.settings, nil,
	)
	env.svc.nowFn = func() time.Time { return env.now }
	if err := env.configs.Update(json.RawMessage(`{"rules":["MATCH,DIRECT"]}`)); err != nil {
		t.Fatalf("seed config draft: %v", err)
	}
	return env
}

func (e *subscriptionTestEnv) create(t *testing.T, body string) *SubscriptionView {
	t.Helper()
	var req SubscriptionRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode create request: %v", err)
	}
	view, _, err := e.svc.Create(&req)
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	return view
}

func (e *subscriptionTestEnv) refresh(t *testing.T, id string) (*SubscriptionView, *RefreshError) {
	t.Helper()
	return e.svc.Refresh(id)
}

func (e *subscriptionTestEnv) update(t *testing.T, id, body string) *SubscriptionView {
	t.Helper()
	var req SubscriptionRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode update request: %v", err)
	}
	view, _, err := e.svc.Update(id, &req)
	if err != nil {
		t.Fatalf("update subscription: %v", err)
	}
	return view
}

func (e *subscriptionTestEnv) draftProviders(t *testing.T) map[string]any {
	t.Helper()
	raw, err := e.configs.Get()
	if err != nil {
		t.Fatalf("read draft config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("decode draft config: %v", err)
	}
	provs, _ := cfg["proxy-providers"].(map[string]any)
	return provs
}

func assertNoSecret(t *testing.T, encoded []byte, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("API projection leaked secret %q: %s", secret, encoded)
		}
	}
}

func TestSubscriptionCreateExactResetAndMaskedURL(t *testing.T) {
	env := newSubscriptionTestEnv(t)
	const rawURL = "https://user:pass@sub.example.com/token/secret-value?token=abc"
	view := env.create(t, `{
		"name":"Example Sub",
		"url":"`+rawURL+`",
		"reset_day":10,"reset_hour":12,"reset_minute":0,"timezone":"Asia/Shanghai",
		"quota_bytes":214748364800,
		"manual_expire":"2027-01-09T12:00:00+08:00",
		"enabled":true
	}`)

	if view.ProviderName != "Example Sub" {
		t.Errorf("default provider name = %q, want subscription name", view.ProviderName)
	}
	if view.URLMasked != "https://sub.example.com/…" {
		t.Errorf("masked URL = %q, want host-only mask", view.URLMasked)
	}
	encoded, _ := json.Marshal(view)
	assertNoSecret(t, encoded, "pass", "secret-value", "token=abc")

	if view.QuotaBytes == nil || *view.QuotaBytes != 214748364800 {
		t.Errorf("quota = %v, want manual supplement", view.QuotaBytes)
	}
	if view.UsedBytes != nil || view.RemainingBytes != nil {
		t.Errorf("unknown vendor usage must stay nil, got used=%v remaining=%v", view.UsedBytes, view.RemainingBytes)
	}
	if view.ExpiresSource != "manual" || view.ExpiresAt == nil || *view.ExpiresAt != "2027-01-09T04:00:00Z" {
		t.Errorf("manual expiry = %v/%v, want vendor-independent manual expiry", view.ExpiresSource, view.ExpiresAt)
	}
	if view.NextReset == nil || *view.NextReset != "2026-11-10T12:00:00+08:00" {
		t.Errorf("next reset = %v, want exact 10th 12:00 Asia/Shanghai", view.NextReset)
	}
	if view.CycleStart == nil || *view.CycleStart != "2026-10-10T12:00:00+08:00" {
		t.Errorf("cycle start = %v, want exact cycle", view.CycleStart)
	}

	provs := env.draftProviders(t)
	entry, ok := provs["Example Sub"].(map[string]any)
	if !ok {
		t.Fatalf("provider entry missing from draft: %#v", provs)
	}
	if entry["url"] != rawURL {
		t.Errorf("draft provider URL changed or dropped")
	}
	if entry["type"] != "http" || entry["path"] != "providers/proxies/Example Sub.yaml" {
		t.Errorf("unexpected native provider entry: %#v", entry)
	}

	raw, _ := env.configs.Get()
	var cfg map[string]any
	_ = json.Unmarshal(raw, &cfg)
	if rules, _ := cfg["rules"].([]any); len(rules) != 1 || rules[0] != "MATCH,DIRECT" {
		t.Errorf("subscription create must not rewrite unrelated draft config: %#v", cfg)
	}
}

func TestSubscriptionCreateHonorsRequestedProviderName(t *testing.T) {
	env := newSubscriptionTestEnv(t)
	if err := env.configs.Update(json.RawMessage(`{
		"proxy-providers":{
			"existing-provider":{"type":"http","url":"https://existing.example.com/sub","path":"providers/existing.yaml","interval":86400}
		}
	}`)); err != nil {
		t.Fatal(err)
	}

	view := env.create(t, `{"name":"New Sub","provider_name":"existing-provider"}`)
	if view.ProviderName != "existing-provider" {
		t.Errorf("provider_name = %q, want requested existing provider", view.ProviderName)
	}
	if view.ProviderSource != "linked" {
		t.Errorf("provider_source = %q, want linked", view.ProviderSource)
	}
	provs := env.draftProviders(t)
	entry, _ := provs["existing-provider"].(map[string]any)
	if entry["url"] != "https://existing.example.com/sub" {
		t.Errorf("existing provider URL changed: %#v", entry)
	}
	if _, duplicated := provs["New Sub"]; duplicated {
		t.Error("linking an existing provider must not create a duplicate provider entry")
	}
}

func TestSubscriptionCreateRejectsURLForExistingProvider(t *testing.T) {
	env := newSubscriptionTestEnv(t)
	if err := env.configs.Update(json.RawMessage(`{
		"proxy-providers":{
			"existing-provider":{"type":"http","url":"https://existing.example.com/sub","path":"providers/existing.yaml","interval":86400}
		}
	}`)); err != nil {
		t.Fatal(err)
	}
	var req SubscriptionRequest
	_ = json.Unmarshal([]byte(`{"name":"New Sub","provider_name":"existing-provider","url":"https://other.example.com/sub"}`), &req)
	if _, _, err := env.svc.Create(&req); err == nil {
		t.Fatal("expected error when supplying a URL for an existing provider")
	}
}

type userinfoTestServer struct {
	mu     sync.Mutex
	status int
	header string
	calls  int
}

func (s *userinfoTestServer) set(status int, header string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
	s.header = header
}

func (s *userinfoTestServer) snap() (int, string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, s.header, s.calls
}

func (s *userinfoTestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	status, header := s.status, s.header
	s.calls++
	s.mu.Unlock()
	if status != 0 {
		w.WriteHeader(status)
		return
	}
	if header != "" {
		w.Header().Set("subscription-userinfo", header)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{}`))
}

func newUserinfoTestServer(t *testing.T) (*userinfoTestServer, *httptest.Server) {
	t.Helper()
	handler := &userinfoTestServer{}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return handler, srv
}

func createRefreshableSubscription(t *testing.T, env *subscriptionTestEnv, url string) *SubscriptionView {
	t.Helper()
	return env.create(t, `{"name":"Refresh Sub","url":"`+url+`","reset_day":10,"reset_hour":12,"reset_minute":0,"timezone":"Asia/Shanghai"}`)
}

func TestSubscriptionVendorUsagePresence(t *testing.T) {
	handler, srv := newUserinfoTestServer(t)
	env := newSubscriptionTestEnv(t)
	view := createRefreshableSubscription(t, env, srv.URL)
	if view == nil {
		t.Fatal("create returned nil view")
	}

	cases := []struct {
		name         string
		header       string
		wantUsedNil  bool
		wantUsed     int64
		wantSource   string
		wantCounters bool
	}{
		{
			name:         "total/expire only must not mean zero used",
			header:       "total=214748364800; expire=1794000000",
			wantUsedNil:  true,
			wantSource:   "none",
			wantCounters: false,
		},
		{
			name:         "explicit zero counters are known usage",
			header:       "upload=0; download=0; total=214748364800; expire=1794000000",
			wantUsed:     0,
			wantSource:   "vendor",
			wantCounters: true,
		},
		{
			name:         "only upload is incomplete and unknown",
			header:       "upload=100; total=214748364800",
			wantUsedNil:  true,
			wantSource:   "none",
			wantCounters: false,
		},
		{
			name:         "both counters are known usage",
			header:       "upload=123; download=456; total=214748364800",
			wantUsed:     579,
			wantSource:   "vendor",
			wantCounters: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler.set(0, tc.header)
			got, refErr := env.refresh(t, view.ID)
			if refErr != nil {
				t.Fatalf("refresh: %+v", refErr)
			}
			if got == nil {
				t.Fatal("refresh returned nil view")
			}
			if tc.wantUsedNil {
				if got.UsedBytes != nil {
					t.Errorf("used bytes = %d, want unknown (nil)", *got.UsedBytes)
				}
				if got.RemainingBytes != nil {
					t.Errorf("remaining = %d, want unknown (nil)", *got.RemainingBytes)
				}
			} else {
				if got.UsedBytes == nil || *got.UsedBytes != tc.wantUsed {
					t.Errorf("used bytes = %v, want %d", got.UsedBytes, tc.wantUsed)
				}
			}
			if got.UsageSource != tc.wantSource {
				t.Errorf("usage source = %q, want %q", got.UsageSource, tc.wantSource)
			}
			stored, err := env.subs.Get(view.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.UserinfoCountersSeen != tc.wantCounters {
				t.Errorf("persisted userinfo counters seen = %v, want %v", stored.UserinfoCountersSeen, tc.wantCounters)
			}
		})
	}
}

func TestSubscriptionRefreshStaleKeepsSnapshot(t *testing.T) {
	handler, srv := newUserinfoTestServer(t)
	env := newSubscriptionTestEnv(t)
	view := createRefreshableSubscription(t, env, srv.URL)

	handler.set(0, "upload=1000; download=2000; total=214748364800; expire=1794000000")
	before, refErr := env.refresh(t, view.ID)
	if refErr != nil {
		t.Fatalf("initial refresh: %+v", refErr)
	}
	if before.UsedBytes == nil || *before.UsedBytes != 3000 || before.UserinfoFetchedAt == nil {
		t.Fatalf("initial snapshot incomplete: %+v", before)
	}
	firstFetched := *before.UserinfoFetchedAt
	firstSuccess := ""
	if before.LastSuccessAt != nil {
		firstSuccess = *before.LastSuccessAt
	}

	env.now = env.now.Add(25 * time.Hour)
	handler.set(http.StatusInternalServerError, "")
	after, refErr := env.refresh(t, view.ID)
	if refErr == nil {
		t.Fatal("expected refresh error for HTTP 500")
	}
	if refErr.Code != "userinfo_failed" {
		t.Errorf("refresh error code = %q, want userinfo_failed", refErr.Code)
	}
	if after == nil {
		t.Fatal("refresh should still return the preserved view")
	}
	if after.UsedBytes == nil || *after.UsedBytes != 3000 {
		t.Errorf("stale refresh discarded used snapshot: %v", after.UsedBytes)
	}
	if after.QuotaBytes == nil || *after.QuotaBytes != 214748364800 {
		t.Errorf("stale refresh discarded quota snapshot: %v", after.QuotaBytes)
	}
	if after.UserinfoFetchedAt == nil || *after.UserinfoFetchedAt != firstFetched {
		t.Errorf("userinfo fetched timestamp changed on failure: %v vs %s", after.UserinfoFetchedAt, firstFetched)
	}
	if after.LastSuccessAt == nil || *after.LastSuccessAt != firstSuccess {
		t.Errorf("last success changed on failure: %v vs %s", after.LastSuccessAt, firstSuccess)
	}
	if !after.Stale {
		t.Error("snapshot older than 24h must be marked stale")
	}
	if after.Status != "error" {
		t.Errorf("status = %q, want error while keeping snapshot", after.Status)
	}
	if after.LastError == "" {
		t.Error("last error should explain the failed refresh")
	}
	if strings.Contains(after.LastError, "http://") || strings.Contains(after.LastError, "https://") {
		t.Errorf("last error leaked an endpoint: %q", after.LastError)
	}
}

func TestSubscriptionUpdateEmptyURLPreservesSecret(t *testing.T) {
	env := newSubscriptionTestEnv(t)
	const rawURL = "https://user:pass@sub.example.com/token/secret-value"
	view := env.create(t, `{"name":"Original","url":"`+rawURL+`","reset_day":10,"reset_hour":12,"reset_minute":0,"timezone":"Asia/Shanghai"}`)

	updated := env.update(t, view.ID, `{"name":"Renamed","url":"","reset_day":10,"reset_hour":12,"reset_minute":0,"timezone":"Asia/Shanghai"}`)
	if updated.Name != "Renamed" {
		t.Errorf("name = %q, want Renamed", updated.Name)
	}
	if updated.URLMasked != "https://sub.example.com/…" {
		t.Errorf("masked URL = %q after empty URL update", updated.URLMasked)
	}
	provs := env.draftProviders(t)
	entry, _ := provs[updated.ProviderName].(map[string]any)
	if entry["url"] != rawURL {
		t.Error("empty URL update must preserve the stored provider URL")
	}
	encoded, _ := json.Marshal(updated)
	assertNoSecret(t, encoded, "pass", "secret-value")

	// Omitting url entirely must also preserve it.
	env.update(t, view.ID, `{"name":"Renamed Again"}`)
	provs = env.draftProviders(t)
	entry, _ = provs[updated.ProviderName].(map[string]any)
	if entry["url"] != rawURL {
		t.Error("omitted URL update must preserve the stored provider URL")
	}
}

func TestSubscriptionDeleteReferencedAndUnreferenced(t *testing.T) {
	env := newSubscriptionTestEnv(t)
	view := env.create(t, `{"name":"Delete Me","url":"https://sub.example.com/token/delete-secret","reset_day":10,"reset_hour":12,"reset_minute":0,"timezone":"Asia/Shanghai"}`)

	group := &model.ProxyGroup{
		ID:      "group-1",
		Name:    "AI-Group",
		Type:    "select",
		Config:  json.RawMessage(`{"use":["Delete Me"]}`),
		Members: json.RawMessage(`[]`),
	}
	if err := env.groups.Create(group); err != nil {
		t.Fatal(err)
	}
	if _, err := env.svc.Delete(view.ID); err == nil {
		t.Fatal("expected referenced deletion to fail")
	} else {
		var ref *ErrReferenced
		if !errors.As(err, &ref) {
			t.Fatalf("error = %T %v, want ErrReferenced", err, err)
		}
		if len(ref.References) != 1 || ref.References[0] != "AI-Group" {
			t.Errorf("references = %#v, want AI-Group", ref.References)
		}
	}
	if _, err := env.subs.Get(view.ID); err != nil {
		t.Errorf("referenced delete removed subscription row: %v", err)
	}
	if _, ok := env.draftProviders(t)[view.ProviderName]; !ok {
		t.Error("referenced delete removed provider from draft")
	}

	if err := env.groups.Delete(group.ID); err != nil {
		t.Fatal(err)
	}

	// Shared traffic accounting must survive subscription deletion.
	tp := &model.TrafficProvider{
		ID: "traffic-1", Name: "Example Traffic", QuotaBytes: 214748364800,
		ResetDay: 10, ResetHour: 12, ResetTimezone: "Asia/Shanghai",
		NodePatternList: []string{"*"}, Enabled: true,
	}
	if err := env.traffic.CreateProvider(tp); err != nil {
		t.Fatal(err)
	}
	if err := env.traffic.AddHourly([]model.TrafficHourly{{Bucket: env.now.Unix(), Node: "HK-01", Upload: 10, Download: 20}}); err != nil {
		t.Fatal(err)
	}
	if err := env.traffic.UpsertUserinfo(&model.TrafficUserinfo{
		ProviderID: tp.ID, Upload: 10, Download: 20, Total: 214748364800,
		FetchedAt: env.now,
	}); err != nil {
		t.Fatal(err)
	}
	env.update(t, view.ID, `{"name":"Delete Me","traffic_provider_id":"traffic-1","reset_day":10,"reset_hour":12,"reset_minute":0,"timezone":"Asia/Shanghai"}`)

	providerKey := store.NewNodeKey("provider", view.ProviderName, "HK-01")
	staticKey := store.NewNodeKey("static", "", "Static-A")
	if err := env.scores.Upsert(&model.NodeScore{
		NodeKey: providerKey, Source: "provider", ProviderName: view.ProviderName,
		NodeName: "HK-01", Status: model.ScoreStatusPending, NextDue: model.NewNullTime(env.now),
	}); err != nil {
		t.Fatal(err)
	}
	if err := env.scores.Upsert(&model.NodeScore{
		NodeKey: staticKey, Source: "static", NodeName: "Static-A",
		Status: model.ScoreStatusPending, NextDue: model.NewNullTime(env.now),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := env.svc.Delete(view.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if result["status"] != "deleted" {
		t.Errorf("status = %v, want deleted", result["status"])
	}
	if result["provider_removed_from_draft"] != true || result["draft_changed"] != true {
		t.Errorf("delete result = %#v, want draft removal", result)
	}
	if _, err := env.subs.Get(view.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("subscription still exists: %v", err)
	}
	if _, ok := env.draftProviders(t)[view.ProviderName]; ok {
		t.Error("unreferenced provider still present in draft")
	}
	// Read counts directly: NodeScoreStore.Get currently cannot scan ip_info
	// (covered by store tests), and this assertion is about deletion scope.
	var providerRows, staticRows int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM node_scores WHERE node_key = ?`, providerKey).Scan(&providerRows); err != nil {
		t.Fatal(err)
	}
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM node_scores WHERE node_key = ?`, staticKey).Scan(&staticRows); err != nil {
		t.Fatal(err)
	}
	if providerRows != 0 {
		t.Errorf("provider score rows were not removed: %d", providerRows)
	}
	if staticRows != 1 {
		t.Errorf("static score row must not be removed: %d", staticRows)
	}
	if _, err := env.traffic.GetProvider(tp.ID); err != nil {
		t.Errorf("shared traffic provider must be preserved: %v", err)
	}
	if rows, err := env.traffic.HourlySince(0); err != nil || len(rows) != 1 {
		t.Errorf("shared traffic hourly rows must be preserved: rows=%v err=%v", rows, err)
	}
	if _, err := env.traffic.GetUserinfo(tp.ID); err != nil {
		t.Errorf("shared traffic userinfo must be preserved: %v", err)
	}
}

func TestSubscriptionLocalUsageUsesSubscriptionResetAndProviderNodes(t *testing.T) {
	env := newSubscriptionTestEnv(t)
	tp := &model.TrafficProvider{
		ID: "tp-local", Name: "Local Traffic", QuotaBytes: 200 * 1024 * 1024 * 1024,
		ResetDay: 1, ResetHour: 0, ResetMinute: 0, ResetTimezone: "UTC",
		NodePatternList: []string{"Sub-*"}, Enabled: true,
	}
	if err := env.traffic.CreateProvider(tp); err != nil {
		t.Fatal(err)
	}
	view := env.create(t, `{
		"name":"Local Sub","url":"https://sub.example.com/local",
		"reset_day":10,"reset_hour":12,"reset_minute":0,"timezone":"Asia/Shanghai",
		"traffic_provider_id":"tp-local"
	}`)
	stored, err := env.subs.Get(view.ID)
	if err != nil {
		t.Fatal(err)
	}

	// now is inside the subscription cycle Oct 10 12:00 - Nov 10 12:00
	// (Asia/Shanghai), while the linked traffic provider resets on day 1.
	env.now = time.Date(2026, time.November, 5, 0, 0, 0, 0, time.UTC)
	bucket := func(month time.Month, day, hour int) int64 {
		return time.Date(2026, month, day, hour, 0, 0, 0, time.UTC).Unix()
	}
	rows := []model.TrafficHourly{
		// Traffic-provider cycle (day 1) includes this; the subscription cycle
		// (day 10 noon) does not.
		{Bucket: bucket(time.October, 5, 12), Node: "Sub-HK", Upload: 100},
		// Inside the subscription cycle and matching the provider nodes.
		{Bucket: bucket(time.October, 12, 12), Node: "Sub-HK", Upload: 10},
		{Bucket: bucket(time.November, 3, 12), Node: "Sub-JP", Download: 30},
		// Same cycle but not a linked-provider node.
		{Bucket: bucket(time.October, 12, 13), Node: "Other-HK", Upload: 1000},
	}
	if err := env.traffic.AddHourly(rows); err != nil {
		t.Fatal(err)
	}

	got, ok := env.svc.localUsed(stored, env.now)
	if !ok {
		t.Fatal("localUsed returned not-known")
	}
	const want = int64(40) // 10 + 30; excludes day-1-only and unmatched node
	if got != want {
		t.Fatalf("localUsed = %d, want %d (subscription cycle + linked provider nodes only)", got, want)
	}
}

type blockingUserinfoServer struct {
	started     chan struct{}
	release     chan struct{}
	header      string
	startOnce   sync.Once
	releaseOnce sync.Once
}

func newBlockingUserinfoServer(t *testing.T, header string) (*blockingUserinfoServer, *httptest.Server) {
	t.Helper()
	h := &blockingUserinfoServer{
		started: make(chan struct{}),
		release: make(chan struct{}),
		header:  header,
	}
	srv := httptest.NewServer(h)
	t.Cleanup(func() {
		h.releaseNow()
		srv.Close()
	})
	return h, srv
}

func (h *blockingUserinfoServer) releaseNow() {
	h.releaseOnce.Do(func() { close(h.release) })
}

func (h *blockingUserinfoServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.startOnce.Do(func() { close(h.started) })
	<-h.release
	w.Header().Set("subscription-userinfo", h.header)
	_, _ = w.Write([]byte(`{}`))
}

func TestAutoRefreshDoesNotOverwriteConcurrentConfigEdit(t *testing.T) {
	blocking, srv := newBlockingUserinfoServer(t, "upload=1000; download=2000; total=214748364800")
	env := newSubscriptionTestEnv(t)
	view := createRefreshableSubscription(t, env, srv.URL)

	done := make(chan struct{})
	go func() {
		env.svc.autoRefreshUserinfo()
		close(done)
	}()
	<-blocking.started

	env.update(t, view.ID, `{
		"name":"Edited During Refresh","enabled":false,
		"reset_day":0,"reset_hour":0,"reset_minute":0,"timezone":""
	}`)
	blocking.releaseNow()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("autoRefreshUserinfo did not finish")
	}

	stored, err := env.subs.Get(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "Edited During Refresh" {
		t.Errorf("concurrent name edit overwritten: %q", stored.Name)
	}
	if stored.Enabled {
		t.Error("concurrent enabled=false edit overwritten by stale refresh")
	}
	if stored.ResetDay != 0 || stored.ResetHour != 0 || stored.ResetMinute != 0 || stored.ResetTimezone != "" {
		t.Errorf("concurrent reset edit overwritten: day=%d hour=%d minute=%d tz=%q",
			stored.ResetDay, stored.ResetHour, stored.ResetMinute, stored.ResetTimezone)
	}
	// The vendor snapshot itself may still be written; only config fields must
	// survive. It is acceptable for the refresh result to be skipped entirely.
	if stored.UserinfoUpload != 0 && stored.UserinfoUpload != 1000 {
		t.Errorf("unexpected userinfo upload = %d", stored.UserinfoUpload)
	}
}

func TestAutoRefreshDoesNotAdoptResultFromOldProviderURL(t *testing.T) {
	blocking, oldSrv := newBlockingUserinfoServer(t, "upload=111; download=222; total=214748364800")
	env := newSubscriptionTestEnv(t)
	view := createRefreshableSubscription(t, env, oldSrv.URL)

	newSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("subscription-userinfo", "upload=999; download=999; total=214748364800")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(newSrv.Close)

	done := make(chan struct{})
	go func() {
		env.svc.autoRefreshUserinfo()
		close(done)
	}()
	<-blocking.started

	// Change the provider source while the old request is in flight.
	env.update(t, view.ID, `{"name":"Edited During Refresh","url":"`+newSrv.URL+`"}`)
	blocking.releaseNow()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("autoRefreshUserinfo did not finish")
	}

	stored, err := env.subs.Get(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.UserinfoUpload == 111 || stored.UserinfoDownload == 222 {
		t.Errorf("stale old-source result was adopted: upload=%d download=%d", stored.UserinfoUpload, stored.UserinfoDownload)
	}
	if stored.Name != "Edited During Refresh" {
		t.Errorf("concurrent name edit overwritten: %q", stored.Name)
	}
}

func TestSubscriptionRefreshPreservesRetryAfter48Hours(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "172800")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	env := newSubscriptionTestEnv(t)
	view := createRefreshableSubscription(t, env, srv.URL)

	_, refErr := env.svc.Refresh(view.ID)
	if refErr == nil || refErr.Code != "rate_limited" {
		t.Fatalf("refresh error = %+v, want rate_limited", refErr)
	}
	if refErr.RetryAfterSecond != 172800 {
		t.Errorf("RetryAfterSecond = %d, want 172800 (48h, not capped at 24h)", refErr.RetryAfterSecond)
	}
}

func TestSubscriptionManualRefreshUsesProxyProviderEndpoint(t *testing.T) {
	userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("subscription-userinfo", "upload=1; download=2; total=214748364800")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(userinfoSrv.Close)

	type recorded struct {
		method string
		path   string
		auth   string
	}
	var calls []recorded
	mihomoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, recorded{
			method: r.Method,
			path:   r.URL.EscapedPath(),
			auth:   r.Header.Get("Authorization"),
		})
		if r.URL.EscapedPath() != "/providers/proxies/Example%20Provider" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(mihomoSrv.Close)

	env := newSubscriptionTestEnv(t)
	view := env.create(t, `{
		"name":"Mock Sub","url":"`+userinfoSrv.URL+`",
		"provider_name":"Example Provider",
		"reset_day":10,"reset_hour":12,"reset_minute":0,"timezone":"Asia/Shanghai"
	}`)
	env.svc.mihomo = NewMihomoClient(mihomoSrv.URL, "mihomo-secret")

	_, refErr := env.svc.Refresh(view.ID)
	if refErr != nil {
		t.Fatalf("manual refresh: %+v", refErr)
	}
	if len(calls) != 1 {
		t.Fatalf("runtime calls = %#v, want exactly one proxy-provider refresh", calls)
	}
	if calls[0].method != http.MethodPut {
		t.Errorf("method = %q, want PUT", calls[0].method)
	}
	if calls[0].path != "/providers/proxies/Example%20Provider" {
		t.Errorf("path = %q, want escaped proxy-provider path", calls[0].path)
	}
	if calls[0].auth != "Bearer mihomo-secret" {
		t.Errorf("authorization = %q, want bearer secret", calls[0].auth)
	}
	for _, call := range calls {
		if strings.Contains(call.path, "/providers/rules/") {
			t.Errorf("subscription refresh called rule-provider endpoint: %s", call.path)
		}
	}
}
