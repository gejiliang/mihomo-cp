package service

import (
	"fmt"
	"time"
)

// ResetPolicy describes a monthly quota reset schedule in an explicit
// timezone. Day is 1-31 (clamped to the target month's last day); Hour and
// Minute are interpreted in Timezone. An empty Timezone means the server's
// local timezone — the legacy behavior before per-subscription timezones
// existed — and 0:00 with local tz reproduces the old CycleStart result
// exactly.
type ResetPolicy struct {
	Day      int
	Hour     int
	Minute   int
	Timezone string // IANA name, "" = server local
}

// Normalize validates and returns a corrected copy of the policy.
func (p ResetPolicy) Normalize() (ResetPolicy, error) {
	out := p
	if out.Day < 1 || out.Day > 31 {
		return out, fmt.Errorf("reset day must be between 1 and 31, got %d", out.Day)
	}
	if out.Hour < 0 || out.Hour > 23 {
		return out, fmt.Errorf("reset hour must be between 0 and 23, got %d", out.Hour)
	}
	if out.Minute < 0 || out.Minute > 59 {
		return out, fmt.Errorf("reset minute must be between 0 and 59, got %d", out.Minute)
	}
	if out.Timezone != "" {
		if _, err := time.LoadLocation(out.Timezone); err != nil {
			return out, fmt.Errorf("invalid reset timezone %q: %w", out.Timezone, err)
		}
	}
	return out, nil
}

// Location resolves the policy timezone; empty means time.Local (legacy).
func (p ResetPolicy) Location() *time.Location {
	if p.Timezone == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return time.Local
	}
	return loc
}

// clampDayIn returns day clamped to the last day of year/month.
func clampDayIn(year int, month time.Month, day int) int {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day > last {
		return last
	}
	return day
}

// cycleStartInTZ returns the most recent reset instant at or before now,
// computed in the policy timezone.
func cycleStartInTZ(now time.Time, p ResetPolicy) time.Time {
	loc := p.Location()
	n := now.In(loc)
	year, month := n.Year(), n.Month()
	day := clampDayIn(year, month, p.Day)
	if n.Day() < day || (n.Day() == day && beforeTimeOfDay(n, p.Hour, p.Minute)) {
		// This month's reset hasn't happened yet → cycle started last month.
		prev := time.Date(year, month, 1, 0, 0, 0, 0, loc).AddDate(0, -1, 0)
		year, month = prev.Year(), prev.Month()
		day = clampDayIn(year, month, p.Day)
	}
	return time.Date(year, month, day, p.Hour, p.Minute, 0, 0, loc)
}

// nextResetInTZ returns the next reset instant strictly after now.
func nextResetInTZ(now time.Time, p ResetPolicy) time.Time {
	start := cycleStartInTZ(now, p)
	// Candidate: add one month to the cycle start, clamping the day.
	next := addMonthClamped(start, p)
	if !next.After(now) {
		next = addMonthClamped(next, p)
	}
	return next
}

// addMonthClamped adds one month to t and places the reset day (clamped) at
// the policy's time of day, in the policy timezone.
func addMonthClamped(t time.Time, p ResetPolicy) time.Time {
	loc := p.Location()
	local := t.In(loc)
	// First day of the following month in the policy timezone.
	first := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, 1, 0)
	day := clampDayIn(first.Year(), first.Month(), p.Day)
	return time.Date(first.Year(), first.Month(), day, p.Hour, p.Minute, 0, 0, loc)
}

// beforeTimeOfDay reports whether t is earlier than hour:minute on its own day.
func beforeTimeOfDay(t time.Time, hour, minute int) bool {
	h, m, _ := t.Clock()
	if h != hour {
		return h < hour
	}
	return m < minute
}

// CycleStartAt returns the start of the billing cycle containing now, for the
// given policy. It is the tz/hour/minute-aware generalization of CycleStart.
func CycleStartAt(now time.Time, p ResetPolicy) time.Time {
	return cycleStartInTZ(now, p)
}

// NextResetAt returns the next reset instant strictly after now.
func NextResetAt(now time.Time, p ResetPolicy) time.Time {
	return nextResetInTZ(now, p)
}
