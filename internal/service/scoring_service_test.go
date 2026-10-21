package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/store"
	"gopkg.in/yaml.v3"
)

type scoringTestEnv struct {
	db       *store.DB
	scores   *store.NodeScoreStore
	proxies  *store.ProxyStore
	groups   *store.ProxyGroupStore
	subs     *store.SubscriptionStore
	configs  *store.ConfigStore
	settings *store.SettingsStore
	svc      *ScoringService
	dir      string
	now      time.Time
}

func newScoringTestEnv(t *testing.T) *scoringTestEnv {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "scoring.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	env := &scoringTestEnv{
		db:       db,
		scores:   store.NewNodeScoreStore(db),
		proxies:  store.NewProxyStore(db),
		groups:   store.NewProxyGroupStore(db),
		subs:     store.NewSubscriptionStore(db),
		configs:  store.NewConfigStore(db),
		settings: store.NewSettingsStore(db),
		dir:      dir,
		now:      time.Date(2026, time.October, 21, 0, 0, 0, 0, time.UTC),
	}
	if err := env.settings.Update(&model.AppSettings{
		MihomoConfig: filepath.Join(dir, "config.yaml"),
		MihomoDir:    dir,
		MihomoBinary: "mihomo",
	}); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	if err := env.configs.Update(json.RawMessage(`{}`)); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	env.svc = NewScoringService(
		env.scores, env.proxies, env.groups, env.subs, env.configs, env.settings, nil,
	)
	env.svc.nowFn = func() time.Time { return env.now }
	return env
}

func (e *scoringTestEnv) setDraft(t *testing.T, cfg any) {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.configs.Update(raw); err != nil {
		t.Fatal(err)
	}
}

func (e *scoringTestEnv) setProvider(t *testing.T, name, path string, override map[string]any) {
	t.Helper()
	entry := map[string]any{
		"type": "http", "url": "https://sub.example.com/" + name,
		"path": path, "interval": 86400,
	}
	if override != nil {
		entry["override"] = override
	}
	e.setDraft(t, map[string]any{"proxy-providers": map[string]any{name: entry}})
}

func (e *scoringTestEnv) writeProviderCache(t *testing.T, path string, nodes []map[string]any) {
	t.Helper()
	full := filepath.Join(e.dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(map[string]any{"proxies": nodes})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (e *scoringTestEnv) addStaticProxy(t *testing.T, name string) *model.Proxy {
	t.Helper()
	p := &model.Proxy{
		ID: "static-" + name, Name: name, Type: "ss",
		Config: json.RawMessage(`{"server":"203.0.113.10","port":8388,"cipher":"aes-128-gcm","password":"example-pass"}`),
	}
	if err := e.proxies.Create(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func nullTimeAt(t model.NullTime, want time.Time) bool {
	return t.Valid && t.Time.Equal(want)
}

func scoringTestNode(name, server string) map[string]any {
	return map[string]any{
		"name": name, "type": "ss", "server": server, "port": 8388,
		"cipher": "aes-128-gcm", "password": "example-pass",
	}
}

func TestScanAndEnqueueTracksInventoryAndFingerprints(t *testing.T) {
	env := newScoringTestEnv(t)
	env.addStaticProxy(t, "Static-A")
	env.setProvider(t, "ExampleProvider", "providers/example.yaml", map[string]any{"additional-prefix": "P-"})
	env.writeProviderCache(t, "providers/example.yaml", []map[string]any{
		scoringTestNode("HK-01", "203.0.113.10"),
		scoringTestNode("JP-01", "198.51.100.10"),
	})

	env.svc.ScanAndEnqueue()
	rows, err := env.scores.List()
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]model.NodeScore{}
	for _, r := range rows {
		byKey[r.NodeKey] = r
	}
	staticKey := store.NewNodeKey("static", "", "Static-A")
	hkKey := store.NewNodeKey("provider", "ExampleProvider", "P-HK-01")
	jpKey := store.NewNodeKey("provider", "ExampleProvider", "P-JP-01")
	for _, key := range []string{staticKey, hkKey, jpKey} {
		row, ok := byKey[key]
		if !ok {
			t.Fatalf("missing queued row %q; rows=%v", key, keysOf(byKey))
		}
		if row.Status != model.ScoreStatusPending {
			t.Errorf("%s status = %q, want pending", key, row.Status)
		}
		if !nullTimeAt(row.NextDue, env.now) {
			t.Errorf("%s next_due = %+v, want now", key, row.NextDue)
		}
		if row.Fingerprint == "" {
			t.Errorf("%s fingerprint is empty", key)
		}
	}

	// A changed static proxy config must also requeue promptly while keeping
	// its previous successful timestamps.
	staticRow := byKey[staticKey]
	staticSuccess := env.now.Add(-72 * time.Hour)
	staticRow.Status = model.ScoreStatusOK
	staticRow.LastSuccessAt = model.NewNullTime(staticSuccess)
	staticRow.NextDue = model.NewNullTime(env.now.Add(7 * 24 * time.Hour))
	if err := env.scores.Upsert(&staticRow); err != nil {
		t.Fatal(err)
	}
	oldStaticFP := staticRow.Fingerprint
	staticProxy, err := env.proxies.GetByName("Static-A")
	if err != nil {
		t.Fatal(err)
	}
	staticProxy.Config = json.RawMessage(`{"server":"203.0.113.99","port":8388,"cipher":"aes-128-gcm","password":"example-pass"}`)
	if err := env.proxies.Update(staticProxy); err != nil {
		t.Fatal(err)
	}
	env.svc.ScanAndEnqueue()
	requeuedStatic, err := env.scores.Get(staticKey)
	if err != nil {
		t.Fatal(err)
	}
	if requeuedStatic.Status != model.ScoreStatusPending || !nullTimeAt(requeuedStatic.NextDue, env.now) {
		t.Errorf("changed static proxy was not requeued: status=%s next=%+v", requeuedStatic.Status, requeuedStatic.NextDue)
	}
	if requeuedStatic.Fingerprint == oldStaticFP {
		t.Error("changed static proxy config did not change its fingerprint")
	}
	if !nullTimeAt(requeuedStatic.LastSuccessAt, staticSuccess) {
		t.Errorf("static proxy requeue lost last success history: %+v", requeuedStatic.LastSuccessAt)
	}

	// Mark the provider row as successfully scored, then change its node
	// config in the cache. The scan must requeue it promptly while preserving
	// the last successful timestamps/history.
	hk := byKey[hkKey]
	lastSuccess := env.now.Add(-48 * time.Hour)
	hk.Status = model.ScoreStatusOK
	hk.CheckedAt = model.NewNullTime(lastSuccess)
	hk.LastSuccessAt = model.NewNullTime(lastSuccess)
	hk.NextDue = model.NewNullTime(env.now.Add(7 * 24 * time.Hour))
	hk.Attempts = 0
	if err := env.scores.Upsert(&hk); err != nil {
		t.Fatal(err)
	}
	oldFP := hk.Fingerprint
	env.writeProviderCache(t, "providers/example.yaml", []map[string]any{
		scoringTestNode("HK-01", "203.0.113.99"),
		scoringTestNode("JP-01", "198.51.100.10"),
	})
	env.svc.InvalidateProviderCache()
	env.svc.ScanAndEnqueue()

	requeued, err := env.scores.Get(hkKey)
	if err != nil {
		t.Fatal(err)
	}
	if requeued.Status != model.ScoreStatusPending {
		t.Errorf("changed provider node status = %q, want pending", requeued.Status)
	}
	if !nullTimeAt(requeued.NextDue, env.now) {
		t.Errorf("changed provider node next_due = %+v, want now", requeued.NextDue)
	}
	if requeued.Fingerprint == oldFP {
		t.Error("changed node config did not change the fingerprint")
	}
	if !nullTimeAt(requeued.LastSuccessAt, lastSuccess) {
		t.Errorf("changed node lost last success history: %+v", requeued.LastSuccessAt)
	}

	// An unchanged scan must not reset the schedule again.
	requeued.Status = model.ScoreStatusOK
	requeued.NextDue = model.NewNullTime(env.now.Add(7 * 24 * time.Hour))
	if err := env.scores.Upsert(requeued); err != nil {
		t.Fatal(err)
	}
	env.svc.ScanAndEnqueue()
	unchanged, err := env.scores.Get(hkKey)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Status != model.ScoreStatusOK || !nullTimeAt(unchanged.NextDue, env.now.Add(7*24*time.Hour)) {
		t.Errorf("unchanged scan reset the schedule: status=%s next=%+v", unchanged.Status, unchanged.NextDue)
	}

	// New provider inventory is discovered after invalidation.
	env.writeProviderCache(t, "providers/example.yaml", []map[string]any{
		scoringTestNode("HK-01", "203.0.113.99"),
		scoringTestNode("JP-01", "198.51.100.10"),
		scoringTestNode("SG-01", "192.0.2.30"),
	})
	env.svc.InvalidateProviderCache()
	env.svc.ScanAndEnqueue()
	newKey := store.NewNodeKey("provider", "ExampleProvider", "P-SG-01")
	if _, err := env.scores.Get(newKey); err != nil {
		t.Errorf("new provider node was not enqueued: %v", err)
	}

	// Removing a node prunes its row; removing the provider prunes all rows.
	env.writeProviderCache(t, "providers/example.yaml", []map[string]any{
		scoringTestNode("HK-01", "203.0.113.99"),
	})
	env.svc.InvalidateProviderCache()
	env.svc.ScanAndEnqueue()
	if _, err := env.scores.Get(jpKey); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("removed provider node row still exists: %v", err)
	}
	env.setDraft(t, map[string]any{"proxy-providers": map[string]any{}})
	env.svc.ScanAndEnqueue()
	if _, err := env.scores.Get(hkKey); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("removed provider row still exists: %v", err)
	}
	if _, err := env.scores.Get(staticKey); err != nil {
		t.Errorf("static row must not be pruned with the provider: %v", err)
	}
}

func keysOf(m map[string]model.NodeScore) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestScanAndEnqueuePausesDisabledSubscriptionProvider(t *testing.T) {
	env := newScoringTestEnv(t)
	env.setProvider(t, "DisabledProvider", "providers/disabled.yaml", nil)
	env.writeProviderCache(t, "providers/disabled.yaml", []map[string]any{
		scoringTestNode("HK-01", "203.0.113.10"),
	})
	sub := &model.Subscription{
		ID: "sub-disabled", Name: "Disabled Sub", Enabled: false,
		ProviderName: "DisabledProvider", ProviderSource: "created",
	}
	if err := env.subs.Create(sub); err != nil {
		t.Fatal(err)
	}
	env.svc.ScanAndEnqueue()
	if rows, err := env.scores.List(); err != nil {
		t.Fatalf("list scores: %v", err)
	} else if len(rows) != 0 {
		t.Errorf("disabled subscription provider must not be auto-enqueued, got %v", rows)
	}

	stored, err := env.subs.Get(sub.ID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	stored.Enabled = true
	if err := env.subs.Update(stored); err != nil {
		t.Fatal(err)
	}
	env.svc.ScanAndEnqueue()
	if _, err := env.scores.Get(store.NewNodeKey("provider", "DisabledProvider", "HK-01")); err != nil {
		t.Errorf("enabled subscription provider was not enqueued: %v", err)
	}
}

func TestRunResetsStuckTestingRowsOnStartup(t *testing.T) {
	env := newScoringTestEnv(t)
	env.addStaticProxy(t, "Static-A")
	staticKey := store.NewNodeKey("static", "", "Static-A")
	if err := env.scores.Upsert(&model.NodeScore{
		NodeKey: staticKey, Source: "static", NodeName: "Static-A",
		Status: model.ScoreStatusTesting, NextDue: model.NewNullTime(env.now.Add(-time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // startup work must still run, then the loop exits
	env.svc.Run(ctx)

	// NodeScoreStore.Get currently cannot scan ip_info; this assertion is
	// about the startup reset, so read the two columns directly.
	var status string
	var nextDue model.NullTime
	if err := env.db.QueryRow(`SELECT status, next_due FROM node_scores WHERE node_key = ?`, staticKey).Scan(&status, &nextDue); err != nil {
		t.Fatal(err)
	}
	if status != model.ScoreStatusPending {
		t.Errorf("stuck testing row status = %q, want pending after startup reset", status)
	}
	if !nullTimeAt(nextDue, env.now) {
		t.Errorf("reset row next_due = %+v, want now", nextDue)
	}
}

func TestRefreshNodesCounts(t *testing.T) {
	env := newScoringTestEnv(t)
	for _, key := range []string{"static/A", "static/B"} {
		if err := env.scores.Upsert(&model.NodeScore{
			NodeKey: key, Source: "static", NodeName: key,
			Status: model.ScoreStatusPending, NextDue: model.NewNullTime(env.now.Add(-time.Hour)),
		}); err != nil {
			t.Fatal(err)
		}
	}
	running := &model.NodeScore{
		NodeKey: "static/R", Source: "static", NodeName: "R",
		Status: model.ScoreStatusTesting, NextDue: model.NewNullTime(env.now.Add(time.Hour)),
	}
	if err := env.scores.Upsert(running); err != nil {
		t.Fatal(err)
	}

	queued, runningCount, skipped, err := env.svc.RefreshNodes([]string{"static/A", "static/R", "missing"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 || runningCount != 1 || skipped != 1 {
		t.Errorf("counts = queued %d running %d skipped %d, want 1/1/1", queued, runningCount, skipped)
	}
	a, err := env.scores.Get("static/A")
	if err != nil {
		t.Fatalf("get queued node: %v", err)
	}
	if !nullTimeAt(a.NextDue, env.now) {
		t.Errorf("queued node next_due = %+v, want now", a.NextDue)
	}
	r, err := env.scores.Get("static/R")
	if err != nil {
		t.Fatalf("get running node: %v", err)
	}
	if r.Status != model.ScoreStatusTesting {
		t.Errorf("running node status changed to %q", r.Status)
	}
}

func TestFailBackoffIncrementsAndCaps(t *testing.T) {
	env := newScoringTestEnv(t)
	key := "static/A"
	row := model.NodeScore{NodeKey: key, Source: "static", NodeName: "A", Status: model.ScoreStatusPending}
	want := []time.Duration{
		30 * time.Minute,
		1 * time.Hour,
		2 * time.Hour,
		4 * time.Hour,
		8 * time.Hour,
		16 * time.Hour,
		24 * time.Hour,
		24 * time.Hour,
	}
	for i, delay := range want {
		row.Attempts = i
		env.svc.fail(row, ErrCodeTestFailed, "example failure", env.now)
		var status string
		var attempts int
		var nextDue model.NullTime
		if err := env.db.QueryRow(
			`SELECT status, attempts, next_due FROM node_scores WHERE node_key = ?`, key,
		).Scan(&status, &attempts, &nextDue); err != nil {
			t.Fatal(err)
		}
		if status != model.ScoreStatusFailed {
			t.Errorf("attempt %d status = %q, want failed", i+1, status)
		}
		if attempts != i+1 {
			t.Errorf("attempt %d attempts = %d", i+1, attempts)
		}
		if !nextDue.Valid {
			t.Fatalf("attempt %d next_due is NULL", i+1)
		}
		if got := nextDue.Time.Sub(env.now); got != delay {
			t.Errorf("attempt %d backoff = %s, want %s", i+1, got, delay)
		}
	}
}

func TestCollectScoresDedupAndRateLimit(t *testing.T) {
	env := newScoringTestEnv(t)
	var calls int32
	var rateMode atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if rateMode.Load() {
			w.Header().Set("Retry-After", "300")
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		switch r.URL.Path {
		case "/api/iprisk/203.0.113.10":
			writeJSON(w, http.StatusOK, map[string]any{"ip": "203.0.113.10", "trust_score": 88})
		case "/api/ip/lookup/203.0.113.10":
			writeJSON(w, http.StatusOK, map[string]any{
				"ip": "203.0.113.10", "cidr": "203.0.113.0/24", "trust_score": 40,
				"is_datacenter": true,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	env.svc.netcoffeeBase = srv.URL

	cachedInfo := mustJSON(map[string]any{
		"query_ip": "203.0.113.10",
		"claude": map[string]any{
			"trust_score": 77, "ip_match": model.IPMatchExact,
			"ip": "203.0.113.10", "query_ip": "203.0.113.10",
		},
		"lookup": map[string]any{"ip": "203.0.113.10", "ip_match": model.IPMatchExact},
	})
	if err := env.scores.PutEgressIPScore(&store.EgressIPScore{
		EgressIP: "203.0.113.10", ClaudeScore: intPtr(77),
		IPInfo: model.NullJSON{Bytes: cachedInfo},
	}); err != nil {
		t.Fatal(err)
	}
	res, info, rate, err := env.svc.collectScores(context.Background(), srv.Client(), "203.0.113.10", true)
	if err != nil || rate != nil {
		t.Fatalf("cached collectScores: err=%v rate=%v", err, rate)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("cached egress IP triggered %d upstream calls, want 0", got)
	}
	if res.claudeScore == nil || *res.claudeScore != 77 || res.lookupMatch != model.IPMatchExact {
		t.Errorf("cached result = %+v, want 77/exact", res)
	}
	if len(info) == 0 {
		t.Error("cached ip_info was not returned")
	}

	// Expire the cache and verify both independent sources are fetched.
	env.now = time.Now().Add(7 * 24 * time.Hour)
	res, _, rate, err = env.svc.collectScores(context.Background(), srv.Client(), "203.0.113.10", true)
	if err != nil || rate != nil {
		t.Fatalf("fresh collectScores: err=%v rate=%v", err, rate)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expired cache triggered %d upstream calls, want 2", got)
	}
	if res.claudeScore == nil || *res.claudeScore != 88 {
		t.Errorf("fresh Claude score = %v, want 88", res.claudeScore)
	}

	// A 429 must surface Retry-After and must not be retried silently.
	env.now = time.Now().Add(14 * 24 * time.Hour)
	rateMode.Store(true)
	_, _, rate, err = env.svc.collectScores(context.Background(), srv.Client(), "203.0.113.10", true)
	if err != nil {
		t.Fatalf("rate limited collectScores returned error: %v", err)
	}
	if rate == nil || rate.RetryAfter != 5*time.Minute {
		t.Errorf("rate limit = %+v, want 5m Retry-After", rate)
	}
}

func intPtr(v int) *int { return &v }

func TestConfiguredUnderlayReachesScoring(t *testing.T) {
	env := newScoringTestEnv(t)
	if err := env.settings.Update(&model.AppSettings{
		MihomoConfig:       filepath.Join(env.dir, "config.yaml"),
		MihomoDir:          env.dir,
		MihomoBinary:       "mihomo",
		IsolationInterface: "test-underlay0",
		IsolationDNS:       "https://223.5.5.5/dns-query,https://1.1.1.1/dns-query",
	}); err != nil {
		t.Fatal(err)
	}
	cfg := env.svc.isolationConfig()
	if cfg.Interface != "test-underlay0" {
		t.Errorf("scoring isolation interface = %q, want configured value", cfg.Interface)
	}
	if len(cfg.DNSServers) != 2 {
		t.Errorf("scoring isolation DNS = %#v, want configured servers", cfg.DNSServers)
	}
}

func TestScoringCadenceConstants(t *testing.T) {
	if RefreshInterval != 7*24*time.Hour {
		t.Errorf("RefreshInterval = %s, want 7 days", RefreshInterval)
	}
	if ReconcileInterval <= 0 || ReconcileInterval > 15*time.Minute {
		t.Errorf("ReconcileInterval = %s, want a periodic sweep (<=15m)", ReconcileInterval)
	}
	if DispatchInterval <= 0 || DispatchInterval > time.Minute {
		t.Errorf("DispatchInterval = %s, want timely dispatch", DispatchInterval)
	}
}
