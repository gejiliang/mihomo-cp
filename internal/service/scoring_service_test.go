package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
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
	if err := env.subs.UpdateMetadata(stored); err != nil {
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

type fakeEgressProber struct {
	client *http.Client
	calls  int32
}

func (f *fakeEgressProber) Probe(ctx context.Context, topo *IsolationTopology, iso IsolationConfig) (*EgressSession, error) {
	atomic.AddInt32(&f.calls, 1)
	return &EgressSession{Client: f.client, Close: func() {}}, nil
}

func TestAutomaticWeeklyRefreshDispatchesOKRowAfterRestart(t *testing.T) {
	env := newScoringTestEnv(t)
	// The isolation guard now requires a verified non-loopback underlay even
	// when the managed draft has no TUN/fake-ip. Use a real local interface.
	iface := findNonLoopbackInterface(t)
	st, err := env.settings.Get()
	if err != nil {
		t.Fatal(err)
	}
	st.IsolationInterface = iface
	if err := env.settings.Update(st); err != nil {
		t.Fatal(err)
	}
	proxy := env.addStaticProxy(t, "Weekly-Refresh")
	due := env.now.Add(RefreshInterval)

	var upstreamCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/claude-trace":
			atomic.AddInt32(&upstreamCalls, 1)
			_, _ = w.Write([]byte("ip=203.0.113.10\n"))
		case "/api/iprisk/203.0.113.10":
			atomic.AddInt32(&upstreamCalls, 1)
			writeJSON(w, http.StatusOK, map[string]any{"ip": "203.0.113.10", "trust_score": 88})
		case "/api/ip/lookup/203.0.113.10":
			atomic.AddInt32(&upstreamCalls, 1)
			writeJSON(w, http.StatusOK, map[string]any{
				"ip": "203.0.113.10", "cidr": "203.0.113.0/24", "trust_score": 40,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	row := &model.NodeScore{
		NodeKey: store.NewNodeKey("static", "", "Weekly-Refresh"), Source: "static",
		NodeName: "Weekly-Refresh", Status: model.ScoreStatusOK,
		Fingerprint:   staticFingerprint(proxy),
		ClaudeScore:   intPtr(88),
		LastSuccessAt: model.NewNullTime(env.now),
		NextDue:       model.NewNullTime(due),
	}
	if err := env.scores.Upsert(row); err != nil {
		t.Fatal(err)
	}

	prober := &fakeEgressProber{client: srv.Client()}
	restarted := NewScoringService(
		env.scores, env.proxies, env.groups, env.subs, env.configs, env.settings, prober,
	)
	restarted.nowFn = func() time.Time { return env.now }
	restarted.claudeTraceURL = srv.URL + "/claude-trace"
	restarted.cfTraceURL = srv.URL + "/cf-trace"
	restarted.netcoffeeBase = srv.URL

	// Simulate a service restart: startup reconciliation must not requeue an
	// unchanged OK row, but its persisted next_due must remain effective.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	restarted.Run(ctx)

	restarted.dispatch(context.Background())
	if got := atomic.LoadInt32(&prober.calls); got != 0 {
		t.Errorf("automatic dispatch probed the successful row before its weekly due time: %d", got)
	}
	if got := atomic.LoadInt32(&upstreamCalls); got != 0 {
		t.Errorf("upstream calls before due time = %d, want 0", got)
	}

	env.now = due
	restarted.dispatch(context.Background())
	if got := atomic.LoadInt32(&prober.calls); got != 1 {
		t.Errorf("automatic dispatch probes at weekly boundary = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&upstreamCalls); got == 0 {
		t.Error("automatic dispatch at weekly boundary made no scoring requests")
	}

	updated, err := env.scores.Get(row.NodeKey)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != model.ScoreStatusOK {
		t.Errorf("refreshed status = %q, want ok", updated.Status)
	}
	if updated.ClaudeScore == nil || *updated.ClaudeScore != 88 {
		t.Errorf("refreshed score = %v, want 88", updated.ClaudeScore)
	}
	if !nullTimeAt(updated.NextDue, due.Add(RefreshInterval)) {
		t.Errorf("next weekly due = %+v, want %s", updated.NextDue, due.Add(RefreshInterval))
	}
	if !nullTimeAt(updated.LastSuccessAt, due) {
		t.Errorf("last success = %+v, want boundary", updated.LastSuccessAt)
	}
}

func TestScanAndEnqueueDetectsProviderCacheChangesWithoutInvalidate(t *testing.T) {
	env := newScoringTestEnv(t)
	env.setProvider(t, "PeriodicProvider", "providers/periodic.yaml", nil)
	env.writeProviderCache(t, "providers/periodic.yaml", []map[string]any{
		scoringTestNode("HK-01", "203.0.113.10"),
		scoringTestNode("JP-01", "198.51.100.10"),
	})

	env.svc.ScanAndEnqueue()
	hkKey := store.NewNodeKey("provider", "PeriodicProvider", "HK-01")
	jpKey := store.NewNodeKey("provider", "PeriodicProvider", "JP-01")
	sgKey := store.NewNodeKey("provider", "PeriodicProvider", "SG-01")
	hk, err := env.scores.Get(hkKey)
	if err != nil {
		t.Fatalf("initial provider inventory missing HK row: %v", err)
	}
	if _, err := env.scores.Get(jpKey); err != nil {
		t.Fatalf("initial provider inventory missing JP row: %v", err)
	}
	oldFP := hk.Fingerprint
	lastSuccess := env.now.Add(-24 * time.Hour)
	hk.Status = model.ScoreStatusOK
	hk.LastSuccessAt = model.NewNullTime(lastSuccess)
	hk.NextDue = model.NewNullTime(env.now.Add(7 * 24 * time.Hour))
	if err := env.scores.Upsert(hk); err != nil {
		t.Fatal(err)
	}

	// Periodic reconciliation must notice the on-disk cache change on its own:
	// no InvalidateProviderCache call here.
	env.writeProviderCache(t, "providers/periodic.yaml", []map[string]any{
		scoringTestNode("HK-01", "203.0.113.99"),
		scoringTestNode("SG-01", "192.0.2.30"),
	})
	env.svc.ScanAndEnqueue()

	changedHK, err := env.scores.Get(hkKey)
	if err != nil {
		t.Fatal(err)
	}
	if changedHK.Fingerprint == oldFP {
		t.Error("periodic scan kept the stale provider cache; changed HK fingerprint not detected")
	}
	if changedHK.Status != model.ScoreStatusPending || !nullTimeAt(changedHK.NextDue, env.now) {
		t.Errorf("changed HK node was not requeued: status=%s next=%+v", changedHK.Status, changedHK.NextDue)
	}
	if !nullTimeAt(changedHK.LastSuccessAt, lastSuccess) {
		t.Errorf("changed HK node lost last success history: %+v", changedHK.LastSuccessAt)
	}
	if _, err := env.scores.Get(jpKey); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("removed provider node JP-01 still has a row: %v", err)
	}
	if _, err := env.scores.Get(sgKey); err != nil {
		t.Errorf("new provider node SG-01 was not discovered by periodic scan: %v", err)
	}
}

func TestStartupWithoutMihomoBinaryThenConfiguredUpdate(t *testing.T) {
	env := newScoringTestEnv(t)
	iface := findNonLoopbackInterface(t)
	st, err := env.settings.Get()
	if err != nil {
		t.Fatal(err)
	}
	st.IsolationInterface = iface
	st.MihomoBinary = "" // startup with no configured runner binary
	if err := env.settings.Update(st); err != nil {
		t.Fatal(err)
	}

	// Startup reconciliation must not panic when no runner binary exists.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	env.svc.Run(ctx)

	proxy := env.addStaticProxy(t, "No-Binary-Node")
	key := store.NewNodeKey("static", "", "No-Binary-Node")
	if err := env.scores.Upsert(&model.NodeScore{
		NodeKey: key, Source: "static", NodeName: "No-Binary-Node",
		Status: model.ScoreStatusPending, Fingerprint: staticFingerprint(proxy),
		NextDue: model.NewNullTime(env.now),
	}); err != nil {
		t.Fatal(err)
	}

	// Missing runner config must surface as a visible row error, not a panic.
	env.svc.dispatch(context.Background())
	failed, err := env.scores.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != model.ScoreStatusFailed {
		t.Errorf("missing binary status = %q, want failed", failed.Status)
	}
	if failed.ErrorCode == "" || failed.LastError == "" {
		t.Errorf("missing binary error not visible: code=%q error=%q", failed.ErrorCode, failed.LastError)
	}

	// A later configured binary must be resolved from current settings, not
	// keep the stale empty runner from startup.
	configuredBinary := filepath.Join(t.TempDir(), "mihomo-configured")
	st, err = env.settings.Get()
	if err != nil {
		t.Fatal(err)
	}
	st.MihomoBinary = configuredBinary
	if err := env.settings.Update(st); err != nil {
		t.Fatal(err)
	}
	prober, err := env.svc.currentProber()
	if err != nil {
		t.Fatalf("currentProber after binary update: %v", err)
	}
	runner, ok := prober.(*MihomoIsolation)
	if !ok {
		t.Fatalf("currentProber type = %T, want *MihomoIsolation", prober)
	}
	if runner.Binary != configuredBinary {
		t.Errorf("runner binary = %q, want updated configured path %q", runner.Binary, configuredBinary)
	}

	secondBinary := filepath.Join(t.TempDir(), "mihomo-configured-2")
	st.MihomoBinary = secondBinary
	if err := env.settings.Update(st); err != nil {
		t.Fatal(err)
	}
	prober, err = env.svc.currentProber()
	if err != nil {
		t.Fatal(err)
	}
	runner, ok = prober.(*MihomoIsolation)
	if !ok || runner.Binary != secondBinary {
		t.Errorf("runner binary after second update = %v, want %q", prober, secondBinary)
	}
}

type recordingErrorProber struct {
	mu      sync.Mutex
	targets []string
}

func (p *recordingErrorProber) Probe(ctx context.Context, topo *IsolationTopology, iso IsolationConfig) (*EgressSession, error) {
	p.mu.Lock()
	p.targets = append(p.targets, topo.Target)
	p.mu.Unlock()
	return nil, errors.New("intentional test probe failure")
}

func (p *recordingErrorProber) count(target string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, got := range p.targets {
		if got == target {
			n++
		}
	}
	return n
}

func TestDisabledSubscriptionQueuedJobsDoNotAutoDispatchButManualRefreshDoes(t *testing.T) {
	env := newScoringTestEnv(t)
	iface := findNonLoopbackInterface(t)
	st, err := env.settings.Get()
	if err != nil {
		t.Fatal(err)
	}
	st.IsolationInterface = iface
	if err := env.settings.Update(st); err != nil {
		t.Fatal(err)
	}

	provider := func(path string) map[string]any {
		return map[string]any{
			"type": "http", "url": "https://sub.example.com/" + path,
			"path": path, "interval": 86400,
		}
	}
	env.setDraft(t, map[string]any{"proxy-providers": map[string]any{
		"PausedProvider": provider("providers/paused.yaml"),
		"ActiveProvider": provider("providers/active.yaml"),
	}})
	env.writeProviderCache(t, "providers/paused.yaml", []map[string]any{
		scoringTestNode("Paused-01", "203.0.113.10"),
		scoringTestNode("Paused-02", "203.0.113.11"),
		scoringTestNode("Paused-03", "203.0.113.12"),
	})
	env.writeProviderCache(t, "providers/active.yaml", []map[string]any{
		scoringTestNode("Active-01", "198.51.100.10"),
	})
	if err := env.subs.Create(&model.Subscription{
		ID: "sub-paused", Name: "Paused Sub", Enabled: false,
		ProviderName: "PausedProvider", ProviderSource: "created",
	}); err != nil {
		t.Fatal(err)
	}
	if err := env.subs.Create(&model.Subscription{
		ID: "sub-active", Name: "Active Sub", Enabled: true,
		ProviderName: "ActiveProvider", ProviderSource: "created",
	}); err != nil {
		t.Fatal(err)
	}

	pausedPending := store.NewNodeKey("provider", "PausedProvider", "Paused-01")
	pausedFailed := store.NewNodeKey("provider", "PausedProvider", "Paused-02")
	pausedWeeklyOK := store.NewNodeKey("provider", "PausedProvider", "Paused-03")
	activeKey := store.NewNodeKey("provider", "ActiveProvider", "Active-01")
	for _, row := range []*model.NodeScore{
		{NodeKey: pausedPending, Source: "provider", NodeName: "Paused-01", ProviderName: "PausedProvider",
			Status: model.ScoreStatusPending, NextDue: model.NewNullTime(env.now.Add(-3 * time.Hour))},
		{NodeKey: pausedFailed, Source: "provider", NodeName: "Paused-02", ProviderName: "PausedProvider",
			Status: model.ScoreStatusFailed, Attempts: 2, NextDue: model.NewNullTime(env.now.Add(-2 * time.Hour))},
		{NodeKey: pausedWeeklyOK, Source: "provider", NodeName: "Paused-03", ProviderName: "PausedProvider",
			Status: model.ScoreStatusOK, NextDue: model.NewNullTime(env.now.Add(-time.Hour))},
		{NodeKey: activeKey, Source: "provider", NodeName: "Active-01", ProviderName: "ActiveProvider",
			Fingerprint: NodeFingerprint(scoringTestNode("Active-01", "198.51.100.10")),
			Status:      model.ScoreStatusPending, NextDue: model.NewNullTime(env.now.Add(-30 * time.Minute))},
	} {
		if err := env.scores.Upsert(row); err != nil {
			t.Fatal(err)
		}
	}

	prober := &recordingErrorProber{}
	env.svc.prober = prober
	env.svc.dispatch(context.Background())

	if got := prober.count("Paused-01") + prober.count("Paused-02") + prober.count("Paused-03"); got != 0 {
		t.Fatalf("paused provider nodes were auto-dispatched %d time(s); targets=%v", got, prober.targets)
	}
	if got := prober.count("Active-01"); got != 1 {
		t.Fatalf("active provider row dispatches = %d, want 1; targets=%v", got, prober.targets)
	}
	for _, key := range []string{pausedPending, pausedFailed, pausedWeeklyOK} {
		row, err := env.scores.Get(key)
		if err != nil {
			t.Fatal(err)
		}
		if row.Status == model.ScoreStatusTesting || row.CheckedAt.Valid {
			t.Errorf("paused row %s was claimed/checked: status=%s checked=%+v", key, row.Status, row.CheckedAt)
		}
	}

	// Explicit manual refresh must requeue the paused row and remain durable
	// across a service restart.
	queued, _, _, err := env.svc.RefreshNodes([]string{pausedPending}, false)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("manual refresh queued = %d, want 1", queued)
	}
	restarted := NewScoringService(
		env.scores, env.proxies, env.groups, env.subs, env.configs, env.settings, prober,
	)
	restarted.nowFn = func() time.Time { return env.now }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	restarted.Run(ctx)
	restarted.dispatch(context.Background())

	if got := prober.count("Paused-01"); got != 1 {
		t.Errorf("manually refreshed paused row dispatches after restart = %d, want 1; targets=%v", got, prober.targets)
	}
}

func TestPartialClaudeFailureRetriesMissingSource(t *testing.T) {
	env := newScoringTestEnv(t)
	var claudeCalls, lookupCalls int32
	var claudeFail atomic.Bool
	claudeFail.Store(true)
	ip := "203.0.113.10"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/iprisk/" + ip:
			atomic.AddInt32(&claudeCalls, 1)
			if claudeFail.Load() {
				http.Error(w, "temporary claude failure", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ip": ip, "trust_score": 88})
		case "/api/ip/lookup/" + ip:
			atomic.AddInt32(&lookupCalls, 1)
			writeJSON(w, http.StatusOK, map[string]any{
				"ip": ip, "cidr": "203.0.113.0/24", "trust_score": 40,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	env.svc.netcoffeeBase = srv.URL

	res, _, _, err := env.svc.collectScores(context.Background(), srv.Client(), ip, true)
	if err == nil {
		t.Fatal("partial Claude failure should retain a visible error")
	}
	if res.claudeScore != nil || res.lookupMatch != model.IPMatchExact {
		t.Fatalf("partial result = %+v, want missing Claude score + successful lookup", res)
	}
	if atomic.LoadInt32(&claudeCalls) != 1 || atomic.LoadInt32(&lookupCalls) != 1 {
		t.Fatalf("first calls claude=%d lookup=%d, want 1/1", claudeCalls, lookupCalls)
	}

	claudeFail.Store(false)
	res, _, _, err = env.svc.collectScores(context.Background(), srv.Client(), ip, true)
	if err != nil {
		t.Fatalf("retry after Claude recovery: %v", err)
	}
	if res.claudeScore == nil || *res.claudeScore != 88 {
		t.Errorf("retry did not fill the missing Claude source: %+v", res)
	}
	if atomic.LoadInt32(&claudeCalls) != 2 {
		t.Errorf("missing-source retry Claude calls = %d, want 2", claudeCalls)
	}
}

func TestConcurrentIdenticalIPScoresCoalesce(t *testing.T) {
	env := newScoringTestEnv(t)
	ip := "203.0.113.20"
	var claudeCalls, lookupCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond) // keep both callers overlapping
		switch r.URL.Path {
		case "/api/iprisk/" + ip:
			atomic.AddInt32(&claudeCalls, 1)
			writeJSON(w, http.StatusOK, map[string]any{"ip": ip, "trust_score": 88})
		case "/api/ip/lookup/" + ip:
			atomic.AddInt32(&lookupCalls, 1)
			writeJSON(w, http.StatusOK, map[string]any{
				"ip": ip, "cidr": "203.0.113.0/24", "trust_score": 40,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	env.svc.netcoffeeBase = srv.URL

	type result struct {
		res probeResult
		err error
	}
	results := make(chan result, 2)
	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	call := func() {
		ready <- struct{}{}
		<-start
		res, _, _, err := env.svc.collectScores(context.Background(), srv.Client(), ip, true)
		results <- result{res: res, err: err}
	}
	go call()
	go call()
	<-ready
	<-ready
	close(start)
	for i := 0; i < 2; i++ {
		got := <-results
		if got.err != nil {
			t.Errorf("concurrent collectScores error: %v", got.err)
		}
		if got.res.claudeScore == nil || *got.res.claudeScore != 88 {
			t.Errorf("concurrent result = %+v, want score 88", got.res)
		}
	}

	if got := atomic.LoadInt32(&claudeCalls); got != 1 {
		t.Errorf("concurrent Claude calls = %d, want 1 (coalesced)", got)
	}
	if got := atomic.LoadInt32(&lookupCalls); got != 1 {
		t.Errorf("concurrent lookup calls = %d, want 1 (coalesced)", got)
	}
}

func TestPinGroupSelectionsUsesReadOnlyCurrentMember(t *testing.T) {
	env := newScoringTestEnv(t)
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodGet {
			t.Errorf("runtime selection mock saw a write: %s %s", r.Method, r.URL.Path)
		}
		switch r.URL.Path {
		case "/proxies/Relay-Group":
			writeJSON(w, http.StatusOK, map[string]any{
				"name": "Relay-Group", "type": "URLTest", "now": "B",
				"all": []string{"A", "B"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	st, err := env.settings.Get()
	if err != nil {
		t.Fatal(err)
	}
	st.ExtController = srv.URL
	st.ExtSecret = ""
	if err := env.settings.Update(st); err != nil {
		t.Fatal(err)
	}
	svc := &ScoringService{settings: env.settings}

	topo := &IsolationTopology{
		Target: "A",
		Nodes: []map[string]any{
			{"name": "A", "type": "ss", "server": "203.0.113.10", "port": 8388, "cipher": "aes-128-gcm", "password": "example-pass"},
			{"name": "B", "type": "ss", "server": "198.51.100.10", "port": 8388, "cipher": "aes-128-gcm", "password": "example-pass"},
		},
		Groups: []map[string]any{
			{"name": "Relay-Group", "type": "url-test", "proxies": []string{"A", "B"}},
		},
	}
	if err := svc.pinGroupSelections(topo); err != nil {
		t.Fatalf("pinGroupSelections: %v", err)
	}
	group := topo.Groups[0]
	if group["type"] != "select" {
		t.Errorf("pinned group type = %v, want select", group["type"])
	}
	proxies, ok := group["proxies"].([]string)
	if !ok || len(proxies) != 1 || proxies[0] != "B" {
		t.Fatalf("pinned group proxies = %#v, want only B", group["proxies"])
	}
	if len(methods) != 1 || methods[0] != "GET /proxies/Relay-Group" {
		t.Errorf("runtime mock requests = %v, want exactly one GET", methods)
	}
	cfg := buildIsolationConfig(topo, 34567, IsolationConfig{
		DNSServers: []string{"https://223.5.5.5/dns-query"},
	})
	var doc map[string]any
	if err := yaml.Unmarshal(cfg, &doc); err != nil {
		t.Fatal(err)
	}
	renderedGroups, _ := doc["proxy-groups"].([]any)
	if len(renderedGroups) != 1 {
		t.Fatalf("rendered groups = %#v", doc["proxy-groups"])
	}
	rendered, _ := renderedGroups[0].(map[string]any)
	if rendered["type"] != "select" {
		t.Errorf("rendered group type = %v, want select", rendered["type"])
	}
	renderedProxies, _ := rendered["proxies"].([]any)
	if len(renderedProxies) != 1 || renderedProxies[0] != "B" {
		t.Errorf("rendered group proxies = %#v, want only B", rendered["proxies"])
	}

	if bin := os.Getenv("MIHOMO_TEST_BINARY"); bin != "" {
		dir := t.TempDir()
		path := filepath.Join(dir, "pinned-config.yaml")
		if err := os.WriteFile(path, cfg, 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(bin, "-t", "-d", dir, "-f", path).CombinedOutput()
		if err != nil {
			t.Fatalf("mihomo -t rejected pinned selector config: %v\n%s\n%s", err, out, cfg)
		}
	}
}

func TestPinGroupSelectionsRejectsNoSingleRuntimeChoice(t *testing.T) {
	env := newScoringTestEnv(t)
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	st, err := env.settings.Get()
	if err != nil {
		t.Fatal(err)
	}
	st.ExtController = srv.URL
	st.ExtSecret = ""
	if err := env.settings.Update(st); err != nil {
		t.Fatal(err)
	}
	svc := &ScoringService{settings: env.settings}

	topo := &IsolationTopology{
		Target: "A",
		Nodes: []map[string]any{
			{"name": "A", "type": "ss", "server": "203.0.113.10", "port": 8388},
			{"name": "B", "type": "ss", "server": "198.51.100.10", "port": 8388},
		},
		Groups: []map[string]any{
			{"name": "LB", "type": "load-balance", "proxies": []string{"A", "B"}},
		},
	}
	if err := svc.pinGroupSelections(topo); err == nil {
		t.Fatal("load-balance group should be rejected, not faked as an actual choice")
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Errorf("unsupported group caused %d runtime requests, want 0", got)
	}

	// A supported group type with no current selection is also an explicit
	// error, never a fabricated actual answer.
	emptySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		writeJSON(w, http.StatusOK, map[string]any{"name": "Fallback-Group", "type": "Fallback"})
	}))
	t.Cleanup(emptySrv.Close)
	st.ExtController = emptySrv.URL
	if err := env.settings.Update(st); err != nil {
		t.Fatal(err)
	}
	topo.Groups = []map[string]any{
		{"name": "Fallback-Group", "type": "fallback", "proxies": []string{"A", "B"}},
	}
	if err := svc.pinGroupSelections(topo); err == nil {
		t.Fatal("group without a current selection should return a visible error")
	}
}
