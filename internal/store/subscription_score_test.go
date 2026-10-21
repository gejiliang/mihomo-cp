package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

func openStoreTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	return db
}

func TestMigrationsCreateSubscriptionAndScoreSchema(t *testing.T) {
	db := openStoreTestDB(t)
	if err := db.Migrate(); err != nil {
		t.Fatalf("second migrate must be a no-op: %v", err)
	}
	columns := []struct {
		query string
		name  string
	}{
		{`SELECT COUNT(userinfo_counters) FROM subscriptions`, "subscriptions.userinfo_counters"},
		{`SELECT COUNT(provider_source) FROM subscriptions`, "subscriptions.provider_source"},
		{`SELECT COUNT(fingerprint) FROM node_scores`, "node_scores.fingerprint"},
		{`SELECT COUNT(claude_ip_match) FROM node_scores`, "node_scores.claude_ip_match"},
		{`SELECT COUNT(lookup_match) FROM node_scores`, "node_scores.lookup_match"},
		{`SELECT COUNT(exit_ip_source) FROM node_scores`, "node_scores.exit_ip_source"},
		{`SELECT COUNT(isolation_interface) FROM app_settings`, "app_settings.isolation_interface"},
		{`SELECT COUNT(isolation_dns) FROM app_settings`, "app_settings.isolation_dns"},
	}
	for _, col := range columns {
		var n int
		if err := db.QueryRow(col.query).Scan(&n); err != nil {
			t.Errorf("missing schema %s: %v", col.name, err)
		}
	}
}

func TestSubscriptionStoreUserinfoPresenceRoundTrip(t *testing.T) {
	db := openStoreTestDB(t)
	store := NewSubscriptionStore(db)
	fetched := time.Date(2026, time.October, 21, 3, 0, 0, 0, time.UTC)

	sub := &model.Subscription{
		ID: "sub-1", Name: "Example Sub", Enabled: true, ProviderName: "example-provider",
		ProviderSource: "created", QuotaBytes: 0, ManualExpire: 0,
		ResetDay: 10, ResetHour: 12, ResetMinute: 0, ResetTimezone: "Asia/Shanghai",
		UserinfoUpload: 0, UserinfoDownload: 0, UserinfoTotal: 0,
		UserinfoCountersSeen: false, UserinfoFetchedAt: model.NewNullTime(fetched),
	}
	if err := store.Create(sub); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.Get(sub.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.UserinfoCountersSeen {
		t.Error("userinfo_counters must remain false when counters were not seen")
	}
	if !got.UserinfoFetchedAt.Valid || !got.UserinfoFetchedAt.Time.Equal(fetched) {
		t.Errorf("userinfo fetched = %+v, want %s", got.UserinfoFetchedAt, fetched)
	}

	got.UserinfoCountersSeen = true
	got.UserinfoUpload, got.UserinfoDownload = 0, 0
	if err := store.Update(got); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated, err := store.Get(sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.UserinfoCountersSeen {
		t.Error("explicit zero counters must be persisted as seen")
	}

	zero := &model.Subscription{ID: "sub-2", Name: "Never Fetched", Enabled: true}
	if err := store.Create(zero); err != nil {
		t.Fatal(err)
	}
	never, err := store.Get(zero.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !never.UserinfoFetchedAt.IsZero() {
		t.Errorf("never fetched timestamp = %+v, want zero", never.UserinfoFetchedAt)
	}
	if never.UserinfoCountersSeen {
		t.Error("never fetched counters must be false")
	}

	if err := store.Delete(sub.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Get(sub.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted subscription still readable: %v", err)
	}
	if err := store.Delete("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing = %v, want ErrNotFound", err)
	}
}

func TestNodeScoreStoreQueuePersistence(t *testing.T) {
	db := openStoreTestDB(t)
	store := NewNodeScoreStore(db)
	now := time.Date(2026, time.October, 21, 0, 0, 0, 0, time.UTC)

	pending := &model.NodeScore{
		NodeKey: "provider/example-provider/HK-01", Source: "provider",
		NodeName: "HK-01", ProviderName: "example-provider",
		Status: model.ScoreStatusPending, Fingerprint: "fp-1", NextDue: model.NewNullTime(now.Add(-time.Hour)),
	}
	ok := &model.NodeScore{
		NodeKey: "static/Static-A", Source: "static", NodeName: "Static-A",
		Status: model.ScoreStatusOK, Fingerprint: "fp-2", NextDue: model.NewNullTime(now.Add(time.Hour)),
		ClaudeScore: intPointer(0),
	}
	testingRow := &model.NodeScore{
		NodeKey: "static/Static-B", Source: "static", NodeName: "Static-B",
		Status: model.ScoreStatusTesting, NextDue: model.NewNullTime(now.Add(-time.Hour)),
	}
	for _, row := range []*model.NodeScore{pending, ok, testingRow} {
		if err := store.Upsert(row); err != nil {
			t.Fatalf("upsert %s: %v", row.NodeKey, err)
		}
	}

	due, err := store.ListDue(now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].NodeKey != pending.NodeKey {
		t.Errorf("ListDue = %#v, want only the due pending row", due)
	}

	claimed, err := store.Claim(pending.NodeKey, now)
	if err != nil || !claimed {
		t.Fatalf("claim pending = %v, %v; want true", claimed, err)
	}
	claimed, err = store.Claim(pending.NodeKey, now)
	if err != nil || claimed {
		t.Errorf("second claim = %v, %v; want false", claimed, err)
	}
	claimedRow, _ := store.Get(pending.NodeKey)
	if claimedRow.Status != model.ScoreStatusTesting {
		t.Errorf("claimed status = %q, want testing", claimedRow.Status)
	}

	reset, err := store.ResetTesting(now)
	if err != nil {
		t.Fatal(err)
	}
	if reset != 2 { // pending was claimed above, plus the pre-seeded testing row
		t.Errorf("ResetTesting count = %d, want 2", reset)
	}
	for _, key := range []string{pending.NodeKey, testingRow.NodeKey} {
		row, err := store.Get(key)
		if err != nil {
			t.Fatal(err)
		}
		if row.Status != model.ScoreStatusPending {
			t.Errorf("%s status after reset = %q, want pending", key, row.Status)
		}
		if !row.NextDue.Valid || !row.NextDue.Time.Equal(now) {
			t.Errorf("%s next_due after reset = %+v, want now", key, row.NextDue)
		}
	}

	total, scored, err := store.Count()
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || scored != 1 {
		t.Errorf("Count = total %d scored %d, want 3/1 (zero is a real score)", total, scored)
	}

	if err := store.DeleteByProvider("example-provider"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(pending.NodeKey); !errors.Is(err, ErrNotFound) {
		t.Errorf("provider row not deleted: %v", err)
	}
	if _, err := store.Get(ok.NodeKey); err != nil {
		t.Errorf("static row must not be deleted by provider cleanup: %v", err)
	}

	if err := store.Delete(ok.NodeKey); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ok.NodeKey); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted row still readable: %v", err)
	}
}

func TestNodeScoreStoreEgressCache(t *testing.T) {
	db := openStoreTestDB(t)
	store := NewNodeScoreStore(db)
	ip := "203.0.113.10"
	info := `{"query_ip":"203.0.113.10","claude":{"trust_score":77,"ip_match":"exact"}}`

	if err := store.PutEgressIPScore(&EgressIPScore{
		EgressIP: ip, ClaudeScore: nil, IPInfo: model.NullJSON{Bytes: []byte(info)},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}
	cached, err := store.GetEgressIPScore(ip)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cached.ClaudeScore != nil {
		t.Errorf("missing cached score must stay nil, got %d", *cached.ClaudeScore)
	}
	if cached.FetchedAt.IsZero() {
		t.Error("cached fetched_at must be set")
	}
	if string(cached.IPInfo.Bytes) != info {
		t.Errorf("cached ip_info = %s", cached.IPInfo.Bytes)
	}

	if err := store.PutEgressIPScore(&EgressIPScore{
		EgressIP: ip, ClaudeScore: intPointer(0),
		IPInfo: model.NullJSON{Bytes: []byte(`{"query_ip":"203.0.113.10","claude":{"trust_score":0}}`)},
	}); err != nil {
		t.Fatalf("update cache: %v", err)
	}
	cached, _ = store.GetEgressIPScore(ip)
	if cached.ClaudeScore == nil || *cached.ClaudeScore != 0 {
		t.Errorf("true zero score must be cached as 0, got %v", cached.ClaudeScore)
	}

	if _, err := store.GetEgressIPScore("198.51.100.7"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing cache = %v, want ErrNotFound", err)
	}
}

func TestNewNodeKey(t *testing.T) {
	if got := NewNodeKey("provider", "example-provider", "HK-01"); got != "provider/example-provider/HK-01" {
		t.Errorf("provider key = %q", got)
	}
	if got := NewNodeKey("static", "ignored", "Static-A"); got != "static/Static-A" {
		t.Errorf("static key = %q", got)
	}
	if got := NewNodeKey("", "", "Direct"); got != "static/Direct" {
		t.Errorf("default key = %q", got)
	}
}

func TestSettingsStorePersistsIsolationUnderlay(t *testing.T) {
	db := openStoreTestDB(t)
	store := NewSettingsStore(db)
	st := &model.AppSettings{
		MihomoConfig:       "/tmp/mihomo-config.yaml",
		MihomoDir:          "/tmp/mihomo-dir",
		MihomoBinary:       "mihomo",
		IsolationInterface: "test-underlay0",
		IsolationDNS:       "https://223.5.5.5/dns-query,https://1.1.1.1/dns-query",
	}
	if err := store.Update(st); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := store.Get()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.IsolationInterface != "test-underlay0" {
		t.Errorf("isolation interface = %q, want persisted value", got.IsolationInterface)
	}
	if got.IsolationDNS != st.IsolationDNS {
		t.Errorf("isolation DNS = %q, want persisted value", got.IsolationDNS)
	}
}

func intPointer(v int) *int { return &v }
