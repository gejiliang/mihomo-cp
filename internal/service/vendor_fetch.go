package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// VendorSnapshot is the parsed vendor usage payload of one subscription URL.
// Exactly one of the two vendor formats is present:
//
//   - Header format (subscription-userinfo): directional upload/download
//     counters + optional total/expire. Info is non-nil.
//   - Aggregate format (e.g. Just My Socks bandwidth-counter JSON): an
//     aggregate used counter + optional monthly limit + optional reset
//     day-of-month. No per-direction counters, no expiry, and the reset day
//     carries NO hour or timezone — those are never inferred from it.
type VendorSnapshot struct {
	// Info is the header-format payload; nil for the aggregate format.
	Info *model.TrafficUserinfo

	// Aggregate format. A nil pointer means the field was absent or invalid
	// (unknown); a non-nil pointer is the vendor-reported value, including a
	// genuine zero used counter.
	AggregateUsed  *int64
	AggregateLimit *int64
	// VendorResetDay is the vendor-OBSERVED reset day-of-month (1-31). It is
	// informational: cycle math keeps using the locally configured schedule.
	VendorResetDay *int
}

// IsAggregate reports whether the snapshot is the aggregate JSON format.
func (v *VendorSnapshot) IsAggregate() bool {
	return v.Info == nil && (v.AggregateUsed != nil || v.AggregateLimit != nil || v.VendorResetDay != nil)
}

// justMySocksJSON is the exact aggregate bandwidth-counter shape served by
// some vendors (e.g. Just My Socks):
//
//	{"monthly_bw_limit_b":1000000000000,"bw_counter_b":123456789,"bw_reset_day_of_month":20}
//
// Fields may be missing individually; each is optional. Nested/typed
// strictly: wrong types decode as absent, negative values are invalid and
// treated as absent — never silently converted to zero.
type justMySocksJSON struct {
	MonthlyBwLimitB   *int64 `json:"monthly_bw_limit_b"`
	BwCounterB        *int64 `json:"bw_counter_b"`
	BwResetDayOfMonth *int   `json:"bw_reset_day_of_month"`
}

func nonNegative(v *int64) *int64 {
	if v == nil || *v < 0 {
		return nil
	}
	return v
}

func validResetDay(v *int) *int {
	if v == nil || *v < 1 || *v > 31 {
		return nil
	}
	return v
}

// FetchVendorInfo GETs a subscription URL and parses the vendor payload.
// The subscription-userinfo header wins when present (native format); the
// aggregate JSON body is used otherwise. An aggregate payload is only valid
// when it carries a nonnegative bw_counter_b (a real zero is valid); quota-
// only, reset-day-only or counter-less JSON fails the fetch so no invented
// zero can ever be persisted. Network errors are sanitized so no endpoint or
// credential can leak into logs, stored errors or API responses.
func FetchVendorInfo(client *http.Client, rawURL string) (*VendorSnapshot, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid subscription URL")
	}
	// Vendors gate configs on a clash UA — some return nothing to others.
	req.Header.Set("User-Agent", "clash.meta")
	resp, err := client.Do(req)
	if err != nil {
		return nil, sanitizeURLError("subscription", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, sanitizeURLError("subscription", err)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("subscription returned HTTP %d", resp.StatusCode)
	}

	if header := resp.Header.Get("subscription-userinfo"); header != "" {
		info, err := ParseUserinfo(header)
		if err != nil {
			return nil, err
		}
		return &VendorSnapshot{Info: info}, nil
	}

	// Aggregate JSON fallback. The counter is the payload's load-bearing
	// field: absent or negative bw_counter_b must FAIL (unknown stays
	// unknown, never a fabricated zero); limit and reset day are optional.
	var jms justMySocksJSON
	if err := json.Unmarshal(body, &jms); err != nil {
		return nil, fmt.Errorf("no subscription-userinfo header")
	}
	counter := nonNegative(jms.BwCounterB)
	if counter == nil {
		return nil, fmt.Errorf("no subscription-userinfo header")
	}
	return &VendorSnapshot{
		AggregateUsed:  counter,
		AggregateLimit: nonNegative(jms.MonthlyBwLimitB),
		VendorResetDay: validResetDay(jms.BwResetDayOfMonth),
	}, nil
}
