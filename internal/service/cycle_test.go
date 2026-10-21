package service

import (
	"testing"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load location %q: %v", name, err)
	}
	return loc
}

func shanghaiTime(t *testing.T, y int, m time.Month, d, h, min, s int) time.Time {
	t.Helper()
	return time.Date(y, m, d, h, min, s, 0, mustLocation(t, "Asia/Shanghai"))
}

func TestCycleStartAtExactMonthlyNoonAsiaShanghai(t *testing.T) {
	loc := mustLocation(t, "Asia/Shanghai")
	policy := ResetPolicy{Day: 10, Hour: 12, Minute: 0, Timezone: "Asia/Shanghai"}

	now := time.Date(2026, time.October, 20, 8, 0, 0, 0, loc)
	start := CycleStartAt(now, policy)
	next := NextResetAt(now, policy)

	wantStart := time.Date(2026, time.October, 10, 12, 0, 0, 0, loc)
	wantNext := time.Date(2026, time.November, 10, 12, 0, 0, 0, loc)
	if !start.Equal(wantStart) {
		t.Errorf("cycle start = %s, want %s", start, wantStart)
	}
	if !next.Equal(wantNext) {
		t.Errorf("next reset = %s, want %s", next, wantNext)
	}
	// The same instants expressed in UTC: 04:00Z, not 12:00Z.
	if got, want := start.UTC().Format(time.RFC3339), "2026-10-10T04:00:00Z"; got != want {
		t.Errorf("cycle start UTC = %s, want %s", got, want)
	}
	if got, want := next.UTC().Format(time.RFC3339), "2026-11-10T04:00:00Z"; got != want {
		t.Errorf("next reset UTC = %s, want %s", got, want)
	}
}

func TestCycleStartAtResetBoundary(t *testing.T) {
	loc := mustLocation(t, "Asia/Shanghai")
	policy := ResetPolicy{Day: 10, Hour: 12, Minute: 0, Timezone: "Asia/Shanghai"}
	oct10 := time.Date(2026, time.October, 10, 12, 0, 0, 0, loc)
	nov10 := time.Date(2026, time.November, 10, 12, 0, 0, 0, loc)
	sep10 := time.Date(2026, time.September, 10, 12, 0, 0, 0, loc)
	dec10 := time.Date(2026, time.December, 10, 12, 0, 0, 0, loc)

	cases := []struct {
		name      string
		now       time.Time
		wantStart time.Time
		wantNext  time.Time
	}{
		{
			name:      "one second before October reset",
			now:       oct10.Add(-time.Second),
			wantStart: sep10,
			wantNext:  oct10,
		},
		{
			name:      "exactly at October reset",
			now:       oct10,
			wantStart: oct10,
			wantNext:  nov10,
		},
		{
			name:      "one second before November reset",
			now:       nov10.Add(-time.Second),
			wantStart: oct10,
			wantNext:  nov10,
		},
		{
			name:      "exactly at November reset",
			now:       nov10,
			wantStart: nov10,
			wantNext:  dec10,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CycleStartAt(tc.now, policy); !got.Equal(tc.wantStart) {
				t.Errorf("cycle start = %s, want %s", got, tc.wantStart)
			}
			if got := NextResetAt(tc.now, policy); !got.Equal(tc.wantNext) {
				t.Errorf("next reset = %s, want %s", got, tc.wantNext)
			}
		})
	}
}

func TestCycleStartAtClampsShortMonthsAndLeapYear(t *testing.T) {
	utc := time.UTC
	policy := ResetPolicy{Day: 31, Hour: 0, Minute: 0, Timezone: "UTC"}

	cases := []struct {
		name      string
		now       time.Time
		wantStart time.Time
		wantNext  time.Time
	}{
		{
			name:      "February 2026 before clamped reset",
			now:       time.Date(2026, time.February, 1, 0, 0, 0, 0, utc),
			wantStart: time.Date(2026, time.January, 31, 0, 0, 0, 0, utc),
			wantNext:  time.Date(2026, time.February, 28, 0, 0, 0, 0, utc),
		},
		{
			name:      "February 2026 at clamped reset",
			now:       time.Date(2026, time.February, 28, 0, 0, 0, 0, utc),
			wantStart: time.Date(2026, time.February, 28, 0, 0, 0, 0, utc),
			wantNext:  time.Date(2026, time.March, 31, 0, 0, 0, 0, utc),
		},
		{
			name:      "leap February 2028",
			now:       time.Date(2028, time.February, 1, 0, 0, 0, 0, utc),
			wantStart: time.Date(2028, time.January, 31, 0, 0, 0, 0, utc),
			wantNext:  time.Date(2028, time.February, 29, 0, 0, 0, 0, utc),
		},
		{
			name:      "leap February 2028 at reset",
			now:       time.Date(2028, time.February, 29, 0, 0, 0, 0, utc),
			wantStart: time.Date(2028, time.February, 29, 0, 0, 0, 0, utc),
			wantNext:  time.Date(2028, time.March, 31, 0, 0, 0, 0, utc),
		},
		{
			name:      "year boundary before reset day",
			now:       time.Date(2026, time.December, 20, 0, 0, 0, 0, utc),
			wantStart: time.Date(2026, time.November, 30, 0, 0, 0, 0, utc),
			wantNext:  time.Date(2026, time.December, 31, 0, 0, 0, 0, utc),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CycleStartAt(tc.now, policy); !got.Equal(tc.wantStart) {
				t.Errorf("cycle start = %s, want %s", got, tc.wantStart)
			}
			if got := NextResetAt(tc.now, policy); !got.Equal(tc.wantNext) {
				t.Errorf("next reset = %s, want %s", got, tc.wantNext)
			}
		})
	}
}

func TestCycleStartAtLateEveningReset(t *testing.T) {
	loc := mustLocation(t, "Asia/Shanghai")
	policy := ResetPolicy{Day: 30, Hour: 23, Minute: 59, Timezone: "Asia/Shanghai"}
	before := time.Date(2026, time.April, 30, 23, 58, 0, 0, loc)
	at := time.Date(2026, time.April, 30, 23, 59, 0, 0, loc)

	if got, want := NextResetAt(before, policy), at; !got.Equal(want) {
		t.Errorf("next reset before boundary = %s, want %s", got, want)
	}
	if got, want := CycleStartAt(before, policy), time.Date(2026, time.March, 30, 23, 59, 0, 0, loc); !got.Equal(want) {
		t.Errorf("cycle start before boundary = %s, want %s", got, want)
	}
	if got, want := CycleStartAt(at, policy), at; !got.Equal(want) {
		t.Errorf("cycle start at boundary = %s, want %s", got, want)
	}
	if got, want := NextResetAt(at, policy), time.Date(2026, time.May, 30, 23, 59, 0, 0, loc); !got.Equal(want) {
		t.Errorf("next reset at boundary = %s, want %s", got, want)
	}
}

func TestProviderResetPolicyLegacyMidnight(t *testing.T) {
	now := time.Date(2026, time.July, 18, 10, 30, 0, 0, time.Local)
	legacy := model.TrafficProvider{ResetDay: 20}
	policy := ProviderResetPolicy(legacy)

	if got, want := CycleStartAt(now, policy), CycleStart(now, 20); !got.Equal(want) {
		t.Errorf("legacy provider policy cycle start = %s, want %s", got, want)
	}
	if got, want := NextResetAt(now, policy), time.Date(2026, time.July, 20, 0, 0, 0, 0, time.Local); !got.Equal(want) {
		t.Errorf("legacy provider next reset = %s, want %s", got, want)
	}
}

func TestResetPolicyNormalize(t *testing.T) {
	valid := []ResetPolicy{
		{Day: 1, Hour: 0, Minute: 0},
		{Day: 31, Hour: 23, Minute: 59, Timezone: "Asia/Shanghai"},
		{Day: 10, Hour: 12, Minute: 0, Timezone: "UTC"},
	}
	for _, p := range valid {
		if _, err := p.Normalize(); err != nil {
			t.Errorf("Normalize(%+v) error: %v", p, err)
		}
	}

	invalid := []ResetPolicy{
		{Day: 0},
		{Day: 32},
		{Day: 10, Hour: -1},
		{Day: 10, Hour: 24},
		{Day: 10, Minute: -1},
		{Day: 10, Minute: 60},
		{Day: 10, Timezone: "Not/AZone"},
	}
	for _, p := range invalid {
		if _, err := p.Normalize(); err == nil {
			t.Errorf("Normalize(%+v) expected error, got nil", p)
		}
	}
}

func TestCycleStartAtInvalidTimezoneFallsBackToLocal(t *testing.T) {
	// Location() is documented to fall back to time.Local for an invalid name;
	// Normalize is the validation boundary. This guards accidental panics.
	policy := ResetPolicy{Day: 10, Hour: 12, Timezone: "Invalid/Zone"}
	if got := policy.Location(); got != time.Local {
		t.Fatalf("Location() = %v, want time.Local", got)
	}
}
