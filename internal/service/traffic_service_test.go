package service

import (
	"testing"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.Local)
}

func TestCycleStart(t *testing.T) {
	cases := []struct {
		now      time.Time
		resetDay int
		want     time.Time
	}{
		// Before reset day → cycle started last month.
		{date(2026, time.July, 18), 20, time.Date(2026, time.June, 20, 0, 0, 0, 0, time.Local)},
		{date(2026, time.July, 18), 19, time.Date(2026, time.June, 19, 0, 0, 0, 0, time.Local)},
		// On/after reset day → cycle started this month.
		{date(2026, time.July, 20), 20, time.Date(2026, time.July, 20, 0, 0, 0, 0, time.Local)},
		{date(2026, time.July, 25), 20, time.Date(2026, time.July, 20, 0, 0, 0, 0, time.Local)},
		// Reset on the 1st → cycle is the calendar month.
		{date(2026, time.July, 1), 1, time.Date(2026, time.July, 1, 0, 0, 0, 0, time.Local)},
		{date(2026, time.July, 31), 1, time.Date(2026, time.July, 1, 0, 0, 0, 0, time.Local)},
		// Reset day beyond month length is clamped.
		{date(2026, time.March, 15), 31, time.Date(2026, time.February, 28, 0, 0, 0, 0, time.Local)},
		{date(2026, time.March, 31), 31, time.Date(2026, time.March, 31, 0, 0, 0, 0, time.Local)},
		// January → previous cycle crosses the year boundary.
		{date(2026, time.January, 5), 20, time.Date(2025, time.December, 20, 0, 0, 0, 0, time.Local)},
	}
	for _, c := range cases {
		got := CycleStart(c.now, c.resetDay)
		if !got.Equal(c.want) {
			t.Errorf("CycleStart(%s, %d) = %s, want %s", c.now.Format("2006-01-02"), c.resetDay, got, c.want)
		}
	}
}

func TestParseUserinfo(t *testing.T) {
	info, err := ParseUserinfo("upload=299786119; download=2192554340; total=429496729600; expire=1789718381")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Upload != 299786119 || info.Download != 2192554340 || info.Total != 429496729600 || info.Expire != 1789718381 {
		t.Errorf("unexpected parse result: %+v", info)
	}

	if _, err := ParseUserinfo("garbage"); err == nil {
		t.Error("expected error for unparsable header")
	}
}

func TestMatchProvider(t *testing.T) {
	providers := []model.TrafficProvider{
		{ID: "ps", Enabled: true, NodePatternList: []string{"PS-*"}},
		{ID: "gdsxrk", Enabled: true, NodePatternList: []string{"HK-*", "TW-*", "JP-*", "SG-*", "KR-*", "US-*"}},
		{ID: "off", Enabled: false, NodePatternList: []string{"*"}},
	}
	cases := map[string]string{
		"PS-VM-c21s4":           "ps",
		"PS-SS-c21s1":           "ps",
		"HK-01":                 "gdsxrk",
		"HK-Edge-VN":            "gdsxrk",
		"JP-02":                 "gdsxrk",
		"Static-Residential-US": "", // disabled provider's catch-all must not match
		"CN-HZ-Aliyun":          "",
		"DIRECT":                "",
	}
	for node, want := range cases {
		if got := MatchProvider(node, providers); got != want {
			t.Errorf("MatchProvider(%q) = %q, want %q", node, got, want)
		}
	}
}

func TestIngestDeltas(t *testing.T) {
	s := NewTrafficService(nil, nil, nil)
	now := time.Date(2026, time.July, 18, 10, 30, 0, 0, time.UTC)
	bucket := now.Truncate(time.Hour).Unix()

	// First poll establishes baselines only — no deltas recorded.
	s.ingest([]mihomoConn{
		{ID: "a", Upload: 100, Download: 1000, Chains: []string{"PS-VM-c21s4", "Default"}},
	}, now)
	if len(s.pending) != 0 {
		t.Fatalf("first poll must only set baselines, got %v", s.pending)
	}

	// Second poll: existing conn grew, new conn appeared.
	s.ingest([]mihomoConn{
		{ID: "a", Upload: 150, Download: 1500, Chains: []string{"PS-VM-c21s4", "Default"}},
		{ID: "b", Upload: 20, Download: 200, Chains: []string{"HK-01", "Gemini"}},
	}, now)
	if d := s.pending[bucketKey{bucket, "PS-VM-c21s4"}]; d.upload != 50 || d.download != 500 {
		t.Errorf("existing conn delta = %+v, want {50 500}", d)
	}
	if d := s.pending[bucketKey{bucket, "HK-01"}]; d.upload != 20 || d.download != 200 {
		t.Errorf("new conn delta = %+v, want {20 200}", d)
	}

	// Third poll: conn "a" gone (closed), conn "b" counter reset (id reuse).
	s.ingest([]mihomoConn{
		{ID: "b", Upload: 5, Download: 50, Chains: []string{"HK-01", "Gemini"}},
	}, now)
	if d := s.pending[bucketKey{bucket, "HK-01"}]; d.upload != 25 || d.download != 250 {
		t.Errorf("after counter reset delta = %+v, want {25 250}", d)
	}
	if _, ok := s.lastSeen["a"]; ok {
		t.Error("closed connection should be dropped from lastSeen")
	}
}

func TestEffectiveUsage(t *testing.T) {
	now := time.Now()
	fresh := &model.TrafficUserinfo{Upload: 100, Download: 900, FetchedAt: now.Add(-10 * time.Minute)}
	stale := &model.TrafficUserinfo{Upload: 100, Download: 900, FetchedAt: now.Add(-5 * time.Hour)}

	if got := effectiveUsage(500, fresh, now); got != 1000 {
		t.Errorf("fresh vendor report should win: got %d", got)
	}
	if got := effectiveUsage(2000, fresh, now); got != 2000 {
		t.Errorf("larger local count should win: got %d", got)
	}
	if got := effectiveUsage(500, stale, now); got != 500 {
		t.Errorf("stale vendor report must be ignored: got %d", got)
	}
	if got := effectiveUsage(500, nil, now); got != 500 {
		t.Errorf("no vendor report: got %d", got)
	}
}
