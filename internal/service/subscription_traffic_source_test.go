package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// ── helpers ──────────────────────────────────────────────────────────────────

var trafficProviderSeq int

func createTrafficProvider(t *testing.T, env *subscriptionTestEnv, mutate func(*model.TrafficProvider)) *model.TrafficProvider {
	t.Helper()
	trafficProviderSeq++
	tp := &model.TrafficProvider{
		ID:              "tp-default-" + strconv.Itoa(trafficProviderSeq),
		Name:            "Legacy Traffic",
		QuotaBytes:      1000,
		ResetDay:        20,
		ResetTimezone:   "",
		NodePatternList: []string{"Legacy-*"},
		Enabled:         true,
	}
	if mutate != nil {
		mutate(tp)
	}
	if err := env.traffic.CreateProvider(tp); err != nil {
		t.Fatalf("create traffic provider: %v", err)
	}
	return tp
}

func createStaticProxies(t *testing.T, env *subscriptionTestEnv, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := env.proxies.Create(&model.Proxy{ID: "px-" + n, Name: n, Type: "ss", Config: json.RawMessage(`{"type":"ss"}`)}); err != nil {
			t.Fatalf("create static proxy %s: %v", n, err)
		}
	}
}

func createTrafficSubscription(t *testing.T, env *subscriptionTestEnv, tpID string, extra string) (*SubscriptionView, bool) {
	t.Helper()
	body := `{"name":"Legacy Source","source_type":"traffic","traffic_provider_id":"` + tpID + `"` + extra + `}`
	var req SubscriptionRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	view, draftChanged, err := env.svc.Create(&req)
	if err != nil {
		t.Fatalf("create traffic subscription: %v", err)
	}
	return view, draftChanged
}

func draftRaw(t *testing.T, env *subscriptionTestEnv) string {
	t.Helper()
	raw, err := env.configs.Get()
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	return string(raw)
}

// countingJSONServer serves a fixed JSON body and counts requests.
type countingJSONServer struct {
	mu    sync.Mutex
	body  string
	code  int
	calls int
}

func (s *countingJSONServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	body, code := s.body, s.code
	s.calls++
	s.mu.Unlock()
	if code != 0 {
		w.WriteHeader(code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func newCountingJSONServer(t *testing.T, body string) (*countingJSONServer, *httptest.Server) {
	t.Helper()
	h := &countingJSONServer{body: body}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return h, srv
}

// blockingJSONServer holds the first request until released.
type blockingJSONServer struct {
	started chan struct{}
	release chan struct{}
	body    string
	once    sync.Once
}

func (s *blockingJSONServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.once.Do(func() { close(s.started) })
	<-s.release
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(s.body))
}

// ── A) adoption: create / inheritance / node count / draft untouched ─────────

func TestTrafficSourceCreateAdoptsExistingTrafficProvider(t *testing.T) {
	env := newSubscriptionTestEnv(t)
	before := draftRaw(t, env)
	createStaticProxies(t, env, "Legacy-01", "Legacy-02", "Legacy-03", "Legacy-04", "Legacy-05", "Legacy-06", "Other-HK", "Other-JP")
	tp := createTrafficProvider(t, env, func(p *model.TrafficProvider) {
		p.ID = "tp-legacy"
		p.QuotaBytes = 1000 * 1024 * 1024 * 1024
		p.ResetDay = 20
		p.ResetTimezone = "" // server-local fallback stays server-local
		p.SubscriptionURL = ""
	})

	view, draftChanged := createTrafficSubscription(t, env, tp.ID, `,"enabled":true`)
	if draftChanged {
		t.Error("traffic-source create must never change the draft")
	}
	if view.SourceType != "traffic" || view.ProviderName != "" || view.ProviderSource != "" || view.ProviderExists {
		t.Errorf("adoption fields = source_type:%q provider:%q source:%q exists:%v",
			view.SourceType, view.ProviderName, view.ProviderSource, view.ProviderExists)
	}
	if view.QuotaBytes == nil || *view.QuotaBytes != tp.QuotaBytes {
		t.Errorf("quota = %v, want inherited %d", view.QuotaBytes, tp.QuotaBytes)
	}
	if view.ResetDay != 20 || view.ResetHour != 0 || view.ResetMinute != 0 {
		t.Errorf("reset = %d/%d/%d, want inherited day 20", view.ResetDay, view.ResetHour, view.ResetMinute)
	}
	if view.Timezone != nil {
		t.Errorf("timezone = %v, want server-local (null)", view.Timezone)
	}
	if view.NodeCount == nil || *view.NodeCount != 6 {
		t.Errorf("node count = %v, want 6 matched static proxies", view.NodeCount)
	}
	if view.URLMasked != "" {
		t.Errorf("url_masked = %q, want empty (source has no subscription URL)", view.URLMasked)
	}
	if view.Status != "local" {
		t.Errorf("status = %q, want local for a URL-less traffic source", view.Status)
	}
	if view.TrafficProviderName != tp.Name {
		t.Errorf("traffic provider name = %q, want %q", view.TrafficProviderName, tp.Name)
	}
	if after := draftRaw(t, env); after != before {
		t.Errorf("draft config changed:\nbefore: %s\nafter:  %s", before, after)
	}
	stored, err := env.subs.Get(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SourceType != model.SubscriptionSourceTraffic || stored.ProviderName != "" || stored.ProviderSource != "" {
		t.Errorf("stored row = %+v", stored)
	}
}

func TestTrafficSourceCreateInheritsTimezoneAndOverrides(t *testing.T) {
	env := newSubscriptionTestEnv(t)
	tp := createTrafficProvider(t, env, func(p *model.TrafficProvider) {
		p.ID = "tp-tz"
		p.ResetDay, p.ResetHour, p.ResetMinute, p.ResetTimezone = 20, 8, 30, "Asia/Shanghai"
	})

	view, _ := createTrafficSubscription(t, env, tp.ID, "")
	if view.Timezone == nil || *view.Timezone != "Asia/Shanghai" {
		t.Errorf("timezone = %v, want inherited Asia/Shanghai", view.Timezone)
	}
	if view.ResetHour != 8 || view.ResetMinute != 30 {
		t.Errorf("reset time = %d:%d, want inherited 08:30", view.ResetHour, view.ResetMinute)
	}

	// Explicit overrides win; quota 0 is an explicit "unknown" override.
	// (A second traffic-mode subscription for the SAME provider is rejected,
	// so the override case adopts its own provider.)
	tp2 := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-tz-2"; p.Name = "Second Zone Traffic" })
	view2, _ := createTrafficSubscription(t, env, tp2.ID, `,"name":"Overridden","quota_bytes":0,"reset_day":1,"reset_hour":0,"reset_minute":0,"timezone":"UTC"`)
	if view2.QuotaBytes != nil {
		t.Errorf("explicit quota_bytes 0 = %v, want unknown (null)", view2.QuotaBytes)
	}
	if view2.ResetDay != 1 || view2.Timezone == nil || *view2.Timezone != "UTC" {
		t.Errorf("explicit reset = %d/%v, want 1/UTC", view2.ResetDay, view2.Timezone)
	}
}

func TestTrafficSourceCreateValidation(t *testing.T) {
	env := newSubscriptionTestEnv(t)
	tp := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-v" })

	cases := []struct {
		name string
		body string
		want string // error substring
	}{
		{"missing link", `{"name":"S1","source_type":"traffic"}`, "traffic_provider_id is required"},
		{"unknown link", `{"name":"S2","source_type":"traffic","traffic_provider_id":"nope"}`, "not found"},
		{"provider name rejected", `{"name":"S3","source_type":"traffic","traffic_provider_id":"tp-v","provider_name":"Native-1"}`, "provider_name must be empty"},
		{"url rejected", `{"name":"S4","source_type":"traffic","traffic_provider_id":"tp-v","url":"https://sub.example.com/token"}`, "url is owned by the linked traffic provider"},
		{"bad source type", `{"name":"S5","source_type":"hybrid","traffic_provider_id":"tp-v"}`, `source_type must be`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req SubscriptionRequest
			if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
				t.Fatal(err)
			}
			_, _, err := env.svc.Create(&req)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}

	// Duplicate traffic-mode adoption is a conflict (pre-check and unique index).
	if _, _, err := env.svc.Create(&SubscriptionRequest{Name: strPtr("First"), SourceType: strPtr("traffic"), TrafficProviderID: strPtr(tp.ID)}); err != nil {
		t.Fatalf("first adoption failed: %v", err)
	}
	var dup SubscriptionRequest
	_ = json.Unmarshal([]byte(`{"name":"Second","source_type":"traffic","traffic_provider_id":"tp-v"}`), &dup)
	if _, _, err := env.svc.Create(&dup); err == nil {
		t.Fatal("expected conflict adopting the same traffic provider twice")
	} else if _, ok := err.(*ErrConflict); !ok {
		t.Fatalf("error = %T %v, want ErrConflict", err, err)
	}

	// A native subscription may still reference the same traffic provider for
	// local accounting — uniqueness is only among traffic-mode subscriptions.
	if err := env.configs.Update(json.RawMessage(`{"proxy-providers":{"native-1":{"type":"http","url":"https://native.example.com/sub","path":"providers/native-1.yaml","interval":86400}}}`)); err != nil {
		t.Fatal(err)
	}
	var nativeReq SubscriptionRequest
	_ = json.Unmarshal([]byte(`{"name":"Native Sub","provider_name":"native-1","traffic_provider_id":"tp-v"}`), &nativeReq)
	if _, _, err := env.svc.Create(&nativeReq); err != nil {
		t.Fatalf("native subscription linking the same traffic provider must succeed: %v", err)
	}
}

func strPtr(s string) *string { return &s }

func trafficIntPtr(i int) *int { return &i }

func trafficInt64Ptr(i int64) *int64 { return &i }

// ── B/C) refresh: aggregate JSON semantics ───────────────────────────────────

func TestTrafficSourceRefreshJSONAggregate(t *testing.T) {
	_, srv := newCountingJSONServer(t, `{"monthly_bw_limit_b":1000000000000,"bw_counter_b":123456789,"bw_reset_day_of_month":20}`)
	env := newSubscriptionTestEnv(t)
	tp := createTrafficProvider(t, env, func(p *model.TrafficProvider) {
		p.ID = "tp-jms"
		p.SubscriptionURL = srv.URL
		p.QuotaBytes = 1000 // stored supplement must lose to the vendor limit
	})
	view, _ := createTrafficSubscription(t, env, tp.ID, "")

	// No native /providers/proxies call may happen for traffic sources.
	mihomoCalls := 0
	mihomoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mihomoCalls++
		http.NotFound(w, r)
	}))
	t.Cleanup(mihomoSrv.Close)
	env.svc.mihomo = NewMihomoClient(mihomoSrv.URL, "secret")

	before := draftRaw(t, env)
	got, refErr := env.refresh(t, view.ID)
	if refErr != nil {
		t.Fatalf("refresh: %+v", refErr)
	}
	if mihomoCalls != 0 {
		t.Errorf("native runtime calls = %d, want 0", mihomoCalls)
	}
	if after := draftRaw(t, env); after != before {
		t.Error("traffic-source refresh must not touch the draft")
	}
	if got.UsedBytes == nil || *got.UsedBytes != 123456789 {
		t.Errorf("vendor used = %v, want 123456789 (aggregate, no directions)", got.UsedBytes)
	}
	if got.UsageSource != "vendor" {
		t.Errorf("usage source = %q, want vendor", got.UsageSource)
	}
	if got.QuotaBytes == nil || *got.QuotaBytes != 1000000000000 {
		t.Errorf("quota = %v, want vendor limit 1000000000000", got.QuotaBytes)
	}
	if got.RemainingBytes == nil || *got.RemainingBytes != 1000000000000-123456789 {
		t.Errorf("remaining = %v, want quota-used", got.RemainingBytes)
	}
	if got.ExpiresAt != nil || got.ExpiresSource != "unknown" {
		t.Errorf("expiry = %v/%q, want unknown (aggregate format has none)", got.ExpiresAt, got.ExpiresSource)
	}
	if got.SupplierResetDay == nil || *got.SupplierResetDay != 20 {
		t.Errorf("supplier reset day = %v, want 20 (observed, informational)", got.SupplierResetDay)
	}
	if got.Status != "ok" {
		t.Errorf("status = %q, want ok", got.Status)
	}
	// The raw userinfo projection must not fabricate directions.
	if got.Userinfo != nil {
		t.Errorf("userinfo = %+v, want nil for aggregate snapshots", got.Userinfo)
	}
	stored, err := env.subs.Get(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.UserinfoIsAggregate || stored.UserinfoAggregateUsed != 123456789 || stored.UserinfoCountersSeen {
		t.Errorf("stored snapshot = aggregate:%v used:%d counters:%v", stored.UserinfoIsAggregate, stored.UserinfoAggregateUsed, stored.UserinfoCountersSeen)
	}
}

func TestTrafficSourceRefreshJSONRealZeroCounter(t *testing.T) {
	_, srv := newCountingJSONServer(t, `{"monthly_bw_limit_b":1000,"bw_counter_b":0}`)
	env := newSubscriptionTestEnv(t)
	tp := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-zero"; p.SubscriptionURL = srv.URL })
	view, _ := createTrafficSubscription(t, env, tp.ID, "")

	got, refErr := env.refresh(t, view.ID)
	if refErr != nil {
		t.Fatalf("refresh: %+v", refErr)
	}
	if got.UsedBytes == nil || *got.UsedBytes != 0 {
		t.Errorf("used = %v, want genuine vendor zero", got.UsedBytes)
	}
	if got.UsageSource != "vendor" {
		t.Errorf("usage source = %q, want vendor", got.UsageSource)
	}
	if got.RemainingBytes == nil || *got.RemainingBytes != 1000 {
		t.Errorf("remaining = %v, want full quota", got.RemainingBytes)
	}
}

func TestTrafficSourceRefreshJSONMissingOrInvalidCounterFailsAndKeepsLastGood(t *testing.T) {
	handler, srv := newCountingJSONServer(t, `{"monthly_bw_limit_b":1000000000000,"bw_counter_b":123456789}`)
	env := newSubscriptionTestEnv(t)
	tp := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-bad"; p.SubscriptionURL = srv.URL })
	view, _ := createTrafficSubscription(t, env, tp.ID, "")

	// Establish a last-good snapshot first, then move the clock so the
	// failed attempt lands strictly after the last success.
	if _, refErr := env.refresh(t, view.ID); refErr != nil {
		t.Fatalf("initial refresh: %+v", refErr)
	}
	env.now = env.now.Add(1 * time.Hour)
	first, err := env.subs.Get(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	firstFetched := first.UserinfoFetchedAt.Time

	cases := []struct {
		name string
		body string
	}{
		{"quota only", `{"monthly_bw_limit_b":1000}`},
		{"reset day only", `{"bw_reset_day_of_month":20}`},
		{"negative counter", `{"monthly_bw_limit_b":1000,"bw_counter_b":-5}`},
		{"garbage", `not-json`},
		{"empty object", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler.mu.Lock()
			handler.body = tc.body
			handler.mu.Unlock()
			got, refErr := env.refresh(t, view.ID)
			if refErr == nil {
				t.Fatal("expected refresh failure for a payload without a valid counter")
			}
			if refErr.Code != "userinfo_failed" {
				t.Errorf("code = %q, want userinfo_failed", refErr.Code)
			}
			if strings.Contains(refErr.Message, "http://") || strings.Contains(refErr.Message, srv.URL) {
				t.Errorf("error leaked the endpoint: %q", refErr.Message)
			}
			if got == nil {
				t.Fatal("refresh should still return the preserved view")
			}
			if got.UsedBytes == nil || *got.UsedBytes != 123456789 {
				t.Errorf("last-good vendor used lost: %v", got.UsedBytes)
			}
			if got.Status != "error" {
				t.Errorf("status = %q, want error while keeping snapshot", got.Status)
			}
			stored, err := env.subs.Get(view.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !stored.UserinfoIsAggregate || stored.UserinfoAggregateUsed != 123456789 {
				t.Errorf("stored snapshot mutated: %+v", stored)
			}
			if !stored.UserinfoFetchedAt.Time.Equal(firstFetched) {
				t.Errorf("fetched_at changed on failure: %v vs %v", stored.UserinfoFetchedAt.Time, firstFetched)
			}
		})
	}
}

func TestTrafficSourceRefreshNoURLIsLocalOnlySuccess(t *testing.T) {
	env := newSubscriptionTestEnv(t)
	tp := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-nourl"; p.SubscriptionURL = "" })
	view, _ := createTrafficSubscription(t, env, tp.ID, "")

	got, refErr := env.refresh(t, view.ID)
	if refErr != nil {
		t.Fatalf("local-only refresh must succeed: %+v", refErr)
	}
	if got.Status != "local" {
		t.Errorf("status = %q, want local", got.Status)
	}
	if got.LastSuccessAt == nil || got.LastRefreshAt == nil {
		t.Errorf("refresh bookkeeping = success:%v attempt:%v, want both set", got.LastSuccessAt, got.LastRefreshAt)
	}
	if got.ExpiresAt != nil || got.ExpiresSource != "unknown" {
		t.Errorf("supplier expiry = %v/%q, want unknown", got.ExpiresAt, got.ExpiresSource)
	}
	if got.LastError != "" {
		t.Errorf("last error = %q, want empty", got.LastError)
	}
	stored, err := env.subs.Get(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.UserinfoFetchedAt.IsZero() {
		t.Errorf("local-only refresh must not fabricate a vendor fetch: %v", stored.UserinfoFetchedAt)
	}
}

func TestTrafficSourceRefreshSourceRebindDiscardsSnapshot(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseNow := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseNow) // safety net; the body releases explicitly below
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"monthly_bw_limit_b":1000,"bw_counter_b":999}`))
	}))
	t.Cleanup(srv.Close)

	env := newSubscriptionTestEnv(t)
	tp := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-rebind"; p.SubscriptionURL = srv.URL })
	view, _ := createTrafficSubscription(t, env, tp.ID, "")

	var refErr *RefreshError
	done := make(chan struct{})
	go func() {
		_, refErr = env.svc.Refresh(view.ID)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("refresh never reached the vendor endpoint")
	}

	// Rebind the source to a different URL while the fetch is in flight,
	// then release the in-flight request (once-safe; cleanup is a no-op).
	tp.SubscriptionURL = srv.URL + "/moved"
	tp.NodePatternList = []string{"Legacy-*"}
	if err := env.traffic.UpdateProvider(tp); err != nil {
		t.Fatal(err)
	}
	releaseNow()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("refresh did not finish")
	}
	if refErr == nil {
		t.Error("rebind mid-flight must surface a refresh error to the caller")
	}

	stored, err := env.subs.Get(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The stale attempt must leave NO trace on the row: no snapshot, no
	// failure state, no refresh timestamps (the discard is signaled through
	// the refresh error, not persisted onto the rebound source).
	if stored.UserinfoIsAggregate || stored.UserinfoAggregateUsed == 999 {
		t.Errorf("stale-source snapshot adopted: aggregate:%v used:%d", stored.UserinfoIsAggregate, stored.UserinfoAggregateUsed)
	}
	if stored.LastRefreshError != "" || stored.LastRefreshAt.Valid || stored.LastSuccessAt.Valid {
		t.Errorf("stale attempt wrote refresh state: err=%q at=%v success=%v",
			stored.LastRefreshError, stored.LastRefreshAt, stored.LastSuccessAt)
	}
}

func TestTrafficSourceAutoRefreshHonorsEnabled(t *testing.T) {
	handler, srv := newCountingJSONServer(t, `{"monthly_bw_limit_b":1000,"bw_counter_b":50}`)
	env := newSubscriptionTestEnv(t)
	tp := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-auto"; p.SubscriptionURL = srv.URL })
	view, _ := createTrafficSubscription(t, env, tp.ID, `,"enabled":false`)

	env.svc.autoRefreshUserinfo()
	if n := handler.calls; n != 0 {
		t.Errorf("paused subscription polled %d times, want 0", n)
	}

	// Re-enable: the next auto poll fetches and persists the snapshot.
	env.update(t, view.ID, `{"name":"Legacy Source","enabled":true}`)
	env.svc.autoRefreshUserinfo()
	if n := handler.calls; n < 1 {
		t.Fatal("enabled subscription was never polled")
	}
	stored, err := env.subs.Get(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.UserinfoIsAggregate || stored.UserinfoAggregateUsed != 50 {
		t.Errorf("auto refresh did not persist the vendor snapshot: %+v", stored)
	}
}

// ── usage: vendor aggregate vs local estimate ────────────────────────────────

func TestTrafficSourceVendorAggregateVsLocalEstimate(t *testing.T) {
	_, srv := newCountingJSONServer(t, `{"monthly_bw_limit_b":10000,"bw_counter_b":3000}`)
	env := newSubscriptionTestEnv(t)
	tp := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-usage"; p.SubscriptionURL = srv.URL })
	view, _ := createTrafficSubscription(t, env, tp.ID, "")

	// Local sampling inside the configured cycle (day 20, server-local).
	env.now = time.Date(2026, time.November, 5, 0, 0, 0, 0, time.UTC)
	rows := []model.TrafficHourly{
		{Bucket: env.now.Unix(), Node: "Legacy-01", Upload: 100, Download: 200},
		{Bucket: env.now.Unix(), Node: "Unmatched-1", Upload: 9999},
	}
	if err := env.traffic.AddHourly(rows); err != nil {
		t.Fatal(err)
	}

	got, refErr := env.refresh(t, view.ID)
	if refErr != nil {
		t.Fatalf("refresh: %+v", refErr)
	}
	if got.UsedBytes == nil || *got.UsedBytes != 3000 {
		t.Errorf("vendor used = %v, want 3000", got.UsedBytes)
	}
	if got.UsageSource != "vendor" {
		t.Errorf("usage source = %q, want vendor", got.UsageSource)
	}
	if got.LocalUsedBytes == nil || *got.LocalUsedBytes != 300 {
		t.Errorf("local estimate = %v, want 300 (Legacy-01 only)", got.LocalUsedBytes)
	}
	if got.RemainingBytes == nil || *got.RemainingBytes != 7000 {
		t.Errorf("remaining = %v, want 7000", got.RemainingBytes)
	}
}

func TestTrafficSourceNeverRefreshedUsesTrafficProviderCacheForQuotaExpiry(t *testing.T) {
	env := newSubscriptionTestEnv(t)
	tp := createTrafficProvider(t, env, func(p *model.TrafficProvider) {
		p.ID = "tp-cache"
		p.SubscriptionURL = "https://sub.example.com/tokened"
		p.QuotaBytes = 0 // nothing stored; the cache must supply quota/expiry
	})
	view, _ := createTrafficSubscription(t, env, tp.ID, "")
	if err := env.traffic.UpsertUserinfo(&model.TrafficUserinfo{
		ProviderID: tp.ID, Upload: 11, Download: 22, Total: 5000, Expire: 1794000000,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := env.svc.GetView(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Quota/expiry come from the cache; vendor COUNTERS never (the cache
	// cannot tell a real zero from absent counters), so usage stays local.
	if got.QuotaBytes == nil || *got.QuotaBytes != 5000 {
		t.Errorf("quota = %v, want cached vendor total", got.QuotaBytes)
	}
	if got.ExpiresAt == nil || got.ExpiresSource != "vendor" {
		t.Errorf("expiry = %v/%q, want cached vendor expiry", got.ExpiresAt, got.ExpiresSource)
	}
	if got.UsageSource != "local" {
		t.Errorf("usage source = %q, want local (cache counters not adopted)", got.UsageSource)
	}
	if got.LocalUsedBytes == nil || *got.LocalUsedBytes != 0 {
		t.Errorf("local estimate = %v, want sampled 0 for the configured cycle", got.LocalUsedBytes)
	}
	if got.Status != "stale" {
		t.Errorf("status = %q, want stale until the first refresh", got.Status)
	}
}

// ── CRUD stays metadata-only; deletion preserves everything ─────────────────

func TestTrafficSourceUpdateAndDeleteAreMetadataOnly(t *testing.T) {
	env := newSubscriptionTestEnv(t)
	before := draftRaw(t, env)
	createStaticProxies(t, env, "Legacy-01", "Legacy-02")
	tp := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-crud"; p.SubscriptionURL = "" })
	view, _ := createTrafficSubscription(t, env, tp.ID, "")

	// Rebind to a second traffic provider + rename: metadata only.
	tp2 := createTrafficProvider(t, env, func(p *model.TrafficProvider) {
		p.ID = "tp-crud-2"
		p.Name = "Second Legacy"
	})
	updated, draftChanged, err := env.svc.Update(view.ID, &SubscriptionRequest{
		Name:              strPtr("Renamed Source"),
		TrafficProviderID: strPtr("tp-crud-2"),
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if draftChanged {
		t.Error("traffic-source update must never change the draft")
	}
	if updated.Name != "Renamed Source" || updated.TrafficProviderName != tp2.Name {
		t.Errorf("update result = %q/%q", updated.Name, updated.TrafficProviderName)
	}
	if after := draftRaw(t, env); after != before {
		t.Errorf("draft changed on update:\nbefore: %s\nafter:  %s", before, after)
	}

	// Rejections.
	var req SubscriptionRequest
	_ = json.Unmarshal([]byte(`{"name":"Renamed Source","url":"https://sub.example.com/x"}`), &req)
	if _, _, err := env.svc.Update(view.ID, &req); err == nil || !strings.Contains(err.Error(), "url is owned") {
		t.Errorf("url edit = %v, want ownership error", err)
	}
	_ = json.Unmarshal([]byte(`{"name":"Renamed Source","provider_name":"Native-1"}`), &req)
	if _, _, err := env.svc.Update(view.ID, &req); err == nil || !strings.Contains(err.Error(), "provider_name must be empty") {
		t.Errorf("provider_name edit = %v, want rejection", err)
	}
	_ = json.Unmarshal([]byte(`{"name":"Renamed Source","source_type":"native"}`), &req)
	if _, _, err := env.svc.Update(view.ID, &req); err == nil || !strings.Contains(err.Error(), "source_type cannot be changed") {
		t.Errorf("source_type edit = %v, want rejection", err)
	}

	// Delete: traffic provider + accounting + static proxies survive; the
	// group-reference guard does not apply (no native provider exists).
	group := &model.ProxyGroup{
		ID: "grp-1", Name: "Static Group", Type: "select",
		Config: json.RawMessage(`{"proxies":["Legacy-01"]}`), Members: json.RawMessage(`["Legacy-01"]`),
	}
	if err := env.groups.Create(group); err != nil {
		t.Fatal(err)
	}
	if err := env.traffic.AddHourly([]model.TrafficHourly{{Bucket: env.now.Unix(), Node: "Legacy-01", Upload: 5, Download: 6}}); err != nil {
		t.Fatal(err)
	}

	result, err := env.svc.Delete(view.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if result["draft_changed"] != false || result["provider_removed_from_draft"] != false {
		t.Errorf("delete result = %#v, want metadata-only", result)
	}
	if result["source_type"] != model.SubscriptionSourceTraffic {
		t.Errorf("delete result source_type = %v", result["source_type"])
	}
	if after := draftRaw(t, env); after != before {
		t.Errorf("draft changed on delete:\nbefore: %s\nafter:  %s", before, after)
	}
	if _, err := env.subs.Get(view.ID); err == nil {
		t.Error("subscription row should be gone")
	}
	if _, err := env.traffic.GetProvider(tp2.ID); err != nil {
		t.Errorf("traffic provider must survive: %v", err)
	}
	if rows, err := env.traffic.HourlySince(0); err != nil || len(rows) != 1 {
		t.Errorf("historical accounting must survive: rows=%v err=%v", rows, err)
	}
	if _, err := env.proxies.GetByName("Legacy-01"); err != nil {
		t.Errorf("static proxies must survive: %v", err)
	}
	if _, err := env.groups.GetByID(group.ID); err != nil {
		t.Errorf("groups must survive: %v", err)
	}
}

// The partial unique index backs the service pre-check under concurrency.
func TestTrafficSourceUniqueIndexRejectsConcurrentAdoption(t *testing.T) {
	env := newSubscriptionTestEnv(t)
	tp := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-uniq" })
	first := &model.Subscription{
		ID: "s-1", Name: "First", Enabled: true,
		SourceType: model.SubscriptionSourceTraffic, TrafficProviderID: tp.ID,
	}
	if err := env.subs.Create(first); err != nil {
		t.Fatal(err)
	}
	second := &model.Subscription{
		ID: "s-2", Name: "Second", Enabled: true,
		SourceType: model.SubscriptionSourceTraffic, TrafficProviderID: tp.ID,
	}
	if err := env.subs.Create(second); err == nil {
		t.Fatal("expected unique index violation")
	} else if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		t.Fatalf("error = %v, want unique violation", err)
	}
	// A native-mode row referencing the same provider is unaffected.
	native := &model.Subscription{
		ID: "s-3", Name: "Native", Enabled: true, ProviderName: "native-1",
		SourceType: model.SubscriptionSourceNative, TrafficProviderID: tp.ID,
	}
	if err := env.subs.Create(native); err != nil {
		t.Fatalf("native row with the same traffic link must be allowed: %v", err)
	}
	// GetByTrafficProvider only resolves the traffic-mode manager.
	got, err := env.subs.GetByTrafficProvider(tp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "s-1" {
		t.Errorf("GetByTrafficProvider = %q, want the traffic-mode manager", got.ID)
	}
}

// ── rebind: old-source vendor data must never attribute to the new source ────

// refreshAggregate seeds a refreshed aggregate snapshot onto the subscription
// bound to tpURLServer and returns the subscription id.
func refreshAggregate(t *testing.T, env *subscriptionTestEnv, tpID string, subName string) string {
	t.Helper()
	var req SubscriptionRequest
	body := `{"name":"` + subName + `","source_type":"traffic","traffic_provider_id":"` + tpID + `"}`
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	view, _, err := env.svc.Create(&req)
	if err != nil {
		t.Fatalf("create %s: %v", subName, err)
	}
	if _, refErr := env.svc.Refresh(view.ID); refErr != nil {
		t.Fatalf("refresh %s: %+v", subName, refErr)
	}
	return view.ID
}

func assertNoVendorSnapshot(t *testing.T, v *SubscriptionView, label string) {
	t.Helper()
	// Local sampled usage (usage_source "local", possibly a sampled 0) is
	// current-source data and legitimate; any vendor-sourced usage is not.
	if v.UsageSource == "vendor" {
		t.Errorf("%s: usage source = vendor, want local/none after clearing", label)
	}
	// Vendor expiry must be gone; a preserved MANUAL supplement is fine.
	if v.ExpiresSource == "vendor" {
		t.Errorf("%s: expiry source = vendor, want unknown/manual after clearing", label)
	}
	if v.SupplierResetDay != nil {
		t.Errorf("%s: supplier reset day = %d, want cleared", label, *v.SupplierResetDay)
	}
	if v.UserinfoFetchedAt != nil {
		t.Errorf("%s: vendor fetch marker survived: %v", label, v.UserinfoFetchedAt)
	}
	// LastRefreshAt/LastSuccessAt are refresh bookkeeping, not vendor data;
	// a successful local-only refresh legitimately sets them afterwards.
	if v.LastError != "" {
		t.Errorf("%s: last error = %q, want cleared", label, v.LastError)
	}
	if v.Userinfo != nil {
		t.Errorf("%s: raw userinfo = %+v, want nil", label, v.Userinfo)
	}
}

func TestTrafficSourceRebindClearsOldSourceVendorSnapshot(t *testing.T) {
	_, srvA := newCountingJSONServer(t, `{"monthly_bw_limit_b":5555,"bw_counter_b":111,"bw_reset_day_of_month":11}`)
	_, srvB := newCountingJSONServer(t, `{"monthly_bw_limit_b":7777,"bw_counter_b":222,"bw_reset_day_of_month":22}`)
	env := newSubscriptionTestEnv(t)
	tpA := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-a"; p.Name = "Source A"; p.SubscriptionURL = srvA.URL })
	tpB := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-b"; p.Name = "Source B"; p.SubscriptionURL = srvB.URL })

	subID := refreshAggregate(t, env, tpA.ID, "Migrating Source")
	before, err := env.subs.Get(subID)
	if err != nil {
		t.Fatal(err)
	}
	if !before.UserinfoIsAggregate || before.UserinfoAggregateUsed != 111 {
		t.Fatalf("precondition: A snapshot missing: %+v", before)
	}

	// Rebind A → B (different provider AND different URL).
	updated, draftChanged, err := env.svc.Update(subID, &SubscriptionRequest{
		Name:              strPtr("Migrating Source"),
		TrafficProviderID: strPtr(tpB.ID),
	})
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if draftChanged {
		t.Error("rebind must stay metadata-only")
	}
	if updated.TrafficProviderName != tpB.Name {
		t.Errorf("rebound view names %q, want %q", updated.TrafficProviderName, tpB.Name)
	}
	assertNoVendorSnapshot(t, updated, "rebind view")

	stored, err := env.subs.Get(subID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.TrafficProviderID != tpB.ID || stored.UserinfoTotal != 0 || stored.UserinfoIsAggregate ||
		stored.UserinfoAggregateUsed != 0 || stored.VendorResetDay != 0 || stored.LastRefreshError != "" ||
		stored.UserinfoFetchedAt.Valid || stored.LastSuccessAt.Valid || stored.LastProviderUpdate.Valid {
		t.Errorf("stored row kept old-source vendor state: %+v", stored)
	}

	// GET projection agrees with the returned view.
	got, err := env.svc.GetView(subID)
	if err != nil {
		t.Fatal(err)
	}
	assertNoVendorSnapshot(t, got, "GET after rebind")

	// An ordinary same-source metadata save must not recover old data.
	env.update(t, subID, `{"name":"Migrated Source"}`)
	again, err := env.svc.GetView(subID)
	if err != nil {
		t.Fatal(err)
	}
	assertNoVendorSnapshot(t, again, "GET after same-source save")

	// Refreshing B replaces (not resurrects) vendor data with B's own.
	if _, refErr := env.svc.Refresh(subID); refErr != nil {
		t.Fatalf("refresh B: %+v", refErr)
	}
	bAfter, err := env.svc.GetView(subID)
	if err != nil {
		t.Fatal(err)
	}
	if bAfter.UsedBytes == nil || *bAfter.UsedBytes != 222 || bAfter.QuotaBytes == nil || *bAfter.QuotaBytes != 7777 {
		t.Errorf("B refresh = used:%v quota:%v, want B's own 222/7777", bAfter.UsedBytes, bAfter.QuotaBytes)
	}
	if bAfter.SupplierResetDay == nil || *bAfter.SupplierResetDay != 22 {
		t.Errorf("B supplier day = %v, want 22", bAfter.SupplierResetDay)
	}
}

func TestTrafficSourceRebindSameURLDifferentProviderIsDifferentSource(t *testing.T) {
	body := `{"monthly_bw_limit_b":4000,"bw_counter_b":44}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	env := newSubscriptionTestEnv(t)
	tpA := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-same-a"; p.Name = "Same A"; p.SubscriptionURL = srv.URL })
	tpB := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-same-b"; p.Name = "Same B"; p.SubscriptionURL = srv.URL })

	subID := refreshAggregate(t, env, tpA.ID, "Same URL Source")

	// Identical URL, different provider ID: the old snapshot must still go.
	updated, _, err := env.svc.Update(subID, &SubscriptionRequest{
		Name:              strPtr("Same URL Source"),
		TrafficProviderID: strPtr(tpB.ID),
	})
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	assertNoVendorSnapshot(t, updated, "same-URL rebind view")

	// B can fetch the same URL and own the resulting snapshot.
	if _, refErr := env.svc.Refresh(subID); refErr != nil {
		t.Fatalf("refresh B: %+v", refErr)
	}
	stored, err := env.subs.Get(subID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.UserinfoIsAggregate || stored.UserinfoAggregateUsed != 44 {
		t.Errorf("B snapshot = %+v, want B's own aggregate 44", stored)
	}
}

func TestTrafficSourceDelayedFetchAfterSameURLRebindDiscarded(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseNow := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseNow)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"monthly_bw_limit_b":9000,"bw_counter_b":999}`))
	}))
	t.Cleanup(srv.Close)

	env := newSubscriptionTestEnv(t)
	createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-dl-a"; p.Name = "Delayed A"; p.SubscriptionURL = srv.URL })
	tpB := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-dl-b"; p.Name = "Delayed B"; p.SubscriptionURL = srv.URL })

	var req SubscriptionRequest
	_ = json.Unmarshal([]byte(`{"name":"Delayed Source","source_type":"traffic","traffic_provider_id":"tp-dl-a"}`), &req)
	view, _, err := env.svc.Create(&req)
	if err != nil {
		t.Fatal(err)
	}

	// Manual refresh of A is in flight; the row rebinds to B (same URL).
	var refErr *RefreshError
	done := make(chan struct{})
	go func() {
		_, refErr = env.svc.Refresh(view.ID)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("refresh never started")
	}
	if _, _, err := env.svc.Update(view.ID, &SubscriptionRequest{
		Name:              strPtr("Delayed Source"),
		TrafficProviderID: strPtr(tpB.ID),
	}); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	releaseNow()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("refresh did not finish")
	}
	if refErr == nil {
		t.Error("delayed A fetch must surface a source-changed error")
	}

	// B must be clean: no A data, no A failure state, no refresh timestamps.
	stored, err := env.subs.Get(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.TrafficProviderID != tpB.ID || stored.UserinfoIsAggregate || stored.UserinfoAggregateUsed != 0 ||
		stored.UserinfoTotal != 0 || stored.VendorResetDay != 0 || stored.LastRefreshError != "" ||
		stored.LastRefreshAt.Valid || stored.LastSuccessAt.Valid {
		t.Errorf("B corrupted by delayed A fetch: %+v", stored)
	}
}

func TestTrafficSourceDelayedAutoFetchAfterSameURLRebindDiscarded(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseNow := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseNow)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"monthly_bw_limit_b":6000,"bw_counter_b":600}`))
	}))
	t.Cleanup(srv.Close)

	env := newSubscriptionTestEnv(t)
	createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-auto-a"; p.Name = "Auto A"; p.SubscriptionURL = srv.URL })
	tpB := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-auto-b"; p.Name = "Auto B"; p.SubscriptionURL = srv.URL })

	var req SubscriptionRequest
	_ = json.Unmarshal([]byte(`{"name":"Auto Source","source_type":"traffic","traffic_provider_id":"tp-auto-a"}`), &req)
	view, _, err := env.svc.Create(&req)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		env.svc.autoRefreshUserinfo()
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("auto refresh never started")
	}
	if _, _, err := env.svc.Update(view.ID, &SubscriptionRequest{
		Name:              strPtr("Auto Source"),
		TrafficProviderID: strPtr(tpB.ID),
	}); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	releaseNow()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("auto refresh did not finish")
	}

	stored, err := env.subs.Get(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.UserinfoIsAggregate || stored.UserinfoAggregateUsed != 0 || stored.UserinfoTotal != 0 ||
		stored.UserinfoFetchedAt.Valid || stored.LastSuccessAt.Valid || stored.LastRefreshError != "" {
		t.Errorf("auto fetch of A leaked onto B: %+v", stored)
	}
}

func TestTrafficSourceRebindToURLlessSourceDoesNotRecoverA(t *testing.T) {
	_, srvA := newCountingJSONServer(t, `{"monthly_bw_limit_b":5000,"bw_counter_b":500}`)
	env := newSubscriptionTestEnv(t)
	createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-x-a"; p.Name = "X A"; p.SubscriptionURL = srvA.URL })
	tpB := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-x-b"; p.Name = "X B"; p.SubscriptionURL = "" })

	subID := refreshAggregate(t, env, "tp-x-a", "To Local Source")

	updated, _, err := env.svc.Update(subID, &SubscriptionRequest{
		Name:              strPtr("To Local Source"),
		TrafficProviderID: strPtr(tpB.ID),
	})
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	assertNoVendorSnapshot(t, updated, "URL-less rebind view")
	if updated.Status != "local" {
		t.Errorf("status = %q, want local (B has no URL)", updated.Status)
	}

	// A local-only refresh must succeed and still not recover A's data.
	if _, refErr := env.svc.Refresh(subID); refErr != nil {
		t.Fatalf("local-only refresh: %+v", refErr)
	}
	got, err := env.svc.GetView(subID)
	if err != nil {
		t.Fatal(err)
	}
	assertNoVendorSnapshot(t, got, "URL-less after refresh")
}

func TestTrafficSourceRebindFailureStateNotWrittenToNewSource(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseNow := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseNow)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	env := newSubscriptionTestEnv(t)
	createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-fail-a"; p.Name = "Fail A"; p.SubscriptionURL = srv.URL })
	tpB := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-fail-b"; p.Name = "Fail B"; p.SubscriptionURL = "" })

	var req SubscriptionRequest
	_ = json.Unmarshal([]byte(`{"name":"Failing Source","source_type":"traffic","traffic_provider_id":"tp-fail-a"}`), &req)
	view, _, err := env.svc.Create(&req)
	if err != nil {
		t.Fatal(err)
	}

	var refErr *RefreshError
	done := make(chan struct{})
	go func() {
		_, refErr = env.svc.Refresh(view.ID)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("refresh never started")
	}
	if _, _, err := env.svc.Update(view.ID, &SubscriptionRequest{
		Name:              strPtr("Failing Source"),
		TrafficProviderID: strPtr(tpB.ID),
	}); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	releaseNow()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("refresh did not finish")
	}
	if refErr == nil {
		t.Error("the failed stale attempt must surface an error to the caller")
	}

	stored, err := env.subs.Get(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LastRefreshError != "" || stored.LastRefreshAt.Valid || stored.LastSuccessAt.Valid {
		t.Errorf("A's failure state leaked onto B: err=%q at=%v success=%v",
			stored.LastRefreshError, stored.LastRefreshAt, stored.LastSuccessAt)
	}
}

func TestTrafficSourceRebindPreservesManualSupplements(t *testing.T) {
	_, srvA := newCountingJSONServer(t, `{"monthly_bw_limit_b":1000,"bw_counter_b":10}`)
	env := newSubscriptionTestEnv(t)
	createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-m-a"; p.Name = "Manual A"; p.SubscriptionURL = srvA.URL })
	tpB := createTrafficProvider(t, env, func(p *model.TrafficProvider) { p.ID = "tp-m-b"; p.Name = "Manual B"; p.SubscriptionURL = "" })

	var req SubscriptionRequest
	_ = json.Unmarshal([]byte(`{"name":"Supplemented","source_type":"traffic","traffic_provider_id":"tp-m-a","quota_bytes":777,"manual_expire":"2027-01-09T00:00:00Z","reset_day":5,"reset_hour":3,"reset_minute":15}`), &req)
	view, _, err := env.svc.Create(&req)
	if err != nil {
		t.Fatal(err)
	}
	if _, refErr := env.svc.Refresh(view.ID); refErr != nil {
		t.Fatalf("refresh: %+v", refErr)
	}

	updated, _, err := env.svc.Update(view.ID, &SubscriptionRequest{
		Name:              strPtr("Supplemented"),
		TrafficProviderID: strPtr(tpB.ID),
	})
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if updated.QuotaBytes == nil || *updated.QuotaBytes != 777 {
		t.Errorf("manual quota = %v, want preserved 777", updated.QuotaBytes)
	}
	if updated.ExpiresSource != "manual" || updated.ExpiresAt == nil {
		t.Errorf("manual expiry = %v/%q, want preserved", updated.ExpiresAt, updated.ExpiresSource)
	}
	if updated.ResetDay != 5 || updated.ResetHour != 3 || updated.ResetMinute != 15 {
		t.Errorf("reset = %d/%d/%d, want preserved override", updated.ResetDay, updated.ResetHour, updated.ResetMinute)
	}
	assertNoVendorSnapshot(t, updated, "supplement rebind view")
}

// ── per-field inheritance on adoption ────────────────────────────────────────

func TestTrafficSourceCreatePerFieldInheritance(t *testing.T) {
	env := newSubscriptionTestEnv(t)

	newTP := func(id string) *model.TrafficProvider {
		tp := &model.TrafficProvider{
			ID: id, Name: "Provider " + id,
			QuotaBytes:      1000,
			ResetDay:        20, ResetHour: 8, ResetMinute: 30,
			ResetTimezone:   "Asia/Shanghai",
			NodePatternList: []string{"Legacy-*"},
			Enabled:         true,
		}
		if err := env.traffic.CreateProvider(tp); err != nil {
			t.Fatal(err)
		}
		return tp
	}

	cases := []struct {
		name           string
		req            SubscriptionRequest
		wantQuota      *int64
		wantDay        int
		wantHour       int
		wantMinute     int
		wantTimezone   string // "" = server-local (null in view)
		wantQuotaNilOK bool
		wantErr        string
	}{
		{
			name:         "timezone-only override keeps provider day/hour/minute",
			req:          SubscriptionRequest{Name: strPtr("S-tz"), SourceType: strPtr("traffic"), TrafficProviderID: strPtr("tp-inh-1"), Timezone: strPtr("UTC")},
			wantQuota:    trafficInt64Ptr(1000), wantDay: 20, wantHour: 8, wantMinute: 30, wantTimezone: "UTC",
		},
		{
			name:         "hour-only override inherits day/minute/timezone",
			req:          SubscriptionRequest{Name: strPtr("S-hour"), SourceType: strPtr("traffic"), TrafficProviderID: strPtr("tp-inh-2"), ResetHour: trafficIntPtr(0)},
			wantQuota:    trafficInt64Ptr(1000), wantDay: 20, wantHour: 0, wantMinute: 30, wantTimezone: "Asia/Shanghai",
		},
		{
			name:         "minute-only override inherits day/hour/timezone",
			req:          SubscriptionRequest{Name: strPtr("S-minute"), SourceType: strPtr("traffic"), TrafficProviderID: strPtr("tp-inh-3"), ResetMinute: trafficIntPtr(0)},
			wantQuota:    trafficInt64Ptr(1000), wantDay: 20, wantHour: 8, wantMinute: 0, wantTimezone: "Asia/Shanghai",
		},
		{
			name:         "day-only override inherits hour/minute/timezone",
			req:          SubscriptionRequest{Name: strPtr("S-day"), SourceType: strPtr("traffic"), TrafficProviderID: strPtr("tp-inh-4"), ResetDay: trafficIntPtr(1)},
			wantQuota:    trafficInt64Ptr(1000), wantDay: 1, wantHour: 8, wantMinute: 30, wantTimezone: "Asia/Shanghai",
		},
		{
			name:         "explicit zero hour is preserved, not inherited",
			req:          SubscriptionRequest{Name: strPtr("S-zeroh"), SourceType: strPtr("traffic"), TrafficProviderID: strPtr("tp-inh-5"), ResetHour: trafficIntPtr(0), ResetMinute: trafficIntPtr(0)},
			wantQuota:    trafficInt64Ptr(1000), wantDay: 20, wantHour: 0, wantMinute: 0, wantTimezone: "Asia/Shanghai",
		},
		{
			name:         "explicit empty timezone keeps server-local while inheriting schedule",
			req:          SubscriptionRequest{Name: strPtr("S-emptytz"), SourceType: strPtr("traffic"), TrafficProviderID: strPtr("tp-inh-6"), Timezone: strPtr("")},
			wantQuota:    trafficInt64Ptr(1000), wantDay: 20, wantHour: 8, wantMinute: 30, wantTimezone: "",
		},
		{
			name:         "explicit zero quota means unknown, not inherited",
			req:          SubscriptionRequest{Name: strPtr("S-zeroq"), SourceType: strPtr("traffic"), TrafficProviderID: strPtr("tp-inh-7"), QuotaBytes: trafficInt64Ptr(0)},
			wantDay:      20, wantHour: 8, wantMinute: 30, wantTimezone: "Asia/Shanghai",
			wantQuotaNilOK: true,
		},
		{
			name:    "explicit invalid day errors instead of inheriting",
			req:     SubscriptionRequest{Name: strPtr("S-badday"), SourceType: strPtr("traffic"), TrafficProviderID: strPtr("tp-inh-8"), ResetDay: trafficIntPtr(45)},
			wantErr: "reset day",
		},
		{
			name:    "explicit invalid timezone errors instead of being replaced",
			req:     SubscriptionRequest{Name: strPtr("S-badtz"), SourceType: strPtr("traffic"), TrafficProviderID: strPtr("tp-inh-9"), Timezone: strPtr("Bogus/Zone")},
			wantErr: "timezone",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tp := newTP("tp-inh-" + strings.ReplaceAll(tc.name, " ", "-"))
			tc.req.TrafficProviderID = strPtr(tp.ID)

			view, _, err := env.svc.Create(&tc.req)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			if tc.wantQuotaNilOK {
				if view.QuotaBytes != nil {
					t.Errorf("quota = %d, want explicit unknown (null)", *view.QuotaBytes)
				}
			} else if view.QuotaBytes == nil || *view.QuotaBytes != *tc.wantQuota {
				t.Errorf("quota = %v, want %d", view.QuotaBytes, *tc.wantQuota)
			}
			if view.ResetDay != tc.wantDay || view.ResetHour != tc.wantHour || view.ResetMinute != tc.wantMinute {
				t.Errorf("reset = %d/%d/%d, want %d/%d/%d",
					view.ResetDay, view.ResetHour, view.ResetMinute, tc.wantDay, tc.wantHour, tc.wantMinute)
			}
			if tc.wantTimezone == "" {
				if view.Timezone != nil {
					t.Errorf("timezone = %q, want server-local (null)", *view.Timezone)
				}
			} else if view.Timezone == nil || *view.Timezone != tc.wantTimezone {
				t.Errorf("timezone = %v, want %q", view.Timezone, tc.wantTimezone)
			}
		})
	}
}
