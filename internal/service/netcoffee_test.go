package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

func TestIPMatchScope(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		response   string
		want       string
	}{
		{name: "exact", query: "203.0.113.10", response: "203.0.113.10", want: model.IPMatchExact},
		{name: "same /24", query: "203.0.113.10", response: "203.0.113.11", want: model.IPMatchCIDR},
		{name: "different /24", query: "203.0.113.10", response: "198.51.100.7", want: model.IPMatchMismatch},
		{name: "missing response", query: "203.0.113.10", response: "", want: model.IPMatchUnknown},
		{name: "invalid response", query: "203.0.113.10", response: "not-an-ip", want: model.IPMatchMismatch},
		{name: "IPv6 same /64", query: "2001:db8::1", response: "2001:db8::2", want: model.IPMatchCIDR},
		{name: "IPv6 different /64", query: "2001:db8::1", response: "2001:db8:1::1", want: model.IPMatchMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IPMatchScope(tc.query, tc.response); got != tc.want {
				t.Errorf("IPMatchScope(%q, %q) = %q, want %q", tc.query, tc.response, got, tc.want)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter("120"); got != 120*time.Second {
		t.Errorf("seconds Retry-After = %s, want 2m", got)
	}
	if got := parseRetryAfter(""); got != time.Minute {
		t.Errorf("missing Retry-After = %s, want 1m", got)
	}
	if got := parseRetryAfter("-5"); got != time.Minute {
		t.Errorf("negative Retry-After = %s, want 1m", got)
	}
	if got := parseRetryAfter("garbage"); got != time.Minute {
		t.Errorf("invalid Retry-After = %s, want 1m", got)
	}
	date := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(date); got <= time.Minute || got > 100*time.Second {
		t.Errorf("HTTP-date Retry-After = %s, want around 90s", got)
	}
}

func newNetCoffeeTestClient(t *testing.T, handler http.HandlerFunc) (*NetCoffeeClient, *httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return &NetCoffeeClient{BaseURL: srv.URL, HTTP: srv.Client()}, srv, &calls
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func TestFetchClaudeTrustIPValidationAndMissingScore(t *testing.T) {
	responses := map[string]map[string]any{
		"/api/iprisk/203.0.113.10": {
			"ip": "203.0.113.10", "cidr": "203.0.113.0/24", "trust_score": 87,
			"asn": 64500, "asOrganization": "Example ISP", "country": "United States",
			"countryCode": "us", "is_datacenter": false, "isResidential": true,
			"is_vpn": false, "is_proxy": false, "is_tor": false,
		},
		"/api/iprisk/203.0.113.20": {
			"ip": "203.0.113.21", "cidr": "203.0.113.0/24", "trust_score": 55,
		},
		"/api/iprisk/203.0.113.30": {
			"ip": "198.51.100.7", "cidr": "198.51.100.0/24", "trust_score": 100,
		},
		"/api/iprisk/203.0.113.40": {
			"ip": "203.0.113.40", "cidr": "203.0.113.0/24",
		},
		"/api/iprisk/203.0.113.50": {
			"ip": "203.0.113.50", "trust_score": 0,
		},
	}
	client, _, calls := newNetCoffeeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		resp, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})

	ctx := context.Background()

	exact, err := client.FetchClaudeTrust(ctx, "203.0.113.10")
	if err != nil {
		t.Fatalf("FetchClaudeTrust exact: %v", err)
	}
	if exact.IPMatch != model.IPMatchExact {
		t.Errorf("exact match = %q, want exact", exact.IPMatch)
	}
	if exact.TrustScore == nil || *exact.TrustScore != 87 {
		t.Errorf("trust score = %v, want 87", exact.TrustScore)
	}
	if exact.ASN != 64500 || exact.ASNOrg != "Example ISP" || exact.CountryCode != "us" {
		t.Errorf("unexpected parsed fields: %+v", exact)
	}
	if exact.IsResidential == nil || !*exact.IsResidential || exact.IsDatacenter == nil || *exact.IsDatacenter {
		t.Errorf("residential/datacenter flags not preserved: %+v", exact)
	}

	cidr, err := client.FetchClaudeTrust(ctx, "203.0.113.20")
	if err != nil {
		t.Fatalf("FetchClaudeTrust cidr: %v", err)
	}
	if cidr.IPMatch != model.IPMatchCIDR {
		t.Errorf("same /24 response match = %q, want cidr", cidr.IPMatch)
	}
	if cidr.ResponseIP != "203.0.113.21" || cidr.QueryIP != "203.0.113.20" {
		t.Errorf("query/response IP not preserved: %+v", cidr)
	}

	mismatch, err := client.FetchClaudeTrust(ctx, "203.0.113.30")
	if err != nil {
		t.Fatalf("FetchClaudeTrust mismatch: %v", err)
	}
	if mismatch.IPMatch != model.IPMatchMismatch {
		t.Errorf("unrelated response match = %q, want mismatch", mismatch.IPMatch)
	}

	missing, err := client.FetchClaudeTrust(ctx, "203.0.113.40")
	if err != nil {
		t.Fatalf("FetchClaudeTrust missing score: %v", err)
	}
	if missing.TrustScore != nil {
		t.Errorf("missing trust_score must stay nil, got %d", *missing.TrustScore)
	}

	zero, err := client.FetchClaudeTrust(ctx, "203.0.113.50")
	if err != nil {
		t.Fatalf("FetchClaudeTrust zero score: %v", err)
	}
	if zero.TrustScore == nil || *zero.TrustScore != 0 {
		t.Errorf("true zero trust_score must stay 0, got %v", zero.TrustScore)
	}

	if got := atomic.LoadInt32(calls); got != 5 {
		t.Errorf("upstream calls = %d, want 5", got)
	}
}

func TestNetCoffeeRateLimitRetryAfter(t *testing.T) {
	t.Run("claude source", func(t *testing.T) {
		client, _, _ := newNetCoffeeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "120")
			http.Error(w, "slow down", http.StatusTooManyRequests)
		})
		_, err := client.FetchClaudeTrust(context.Background(), "203.0.113.10")
		var rl *RateLimitError
		if !errors.As(err, &rl) {
			t.Fatalf("error = %v, want RateLimitError", err)
		}
		if rl.RetryAfter != 120*time.Second {
			t.Errorf("RetryAfter = %s, want 2m", rl.RetryAfter)
		}
	})

	t.Run("lookup source", func(t *testing.T) {
		client, _, _ := newNetCoffeeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "45")
			http.Error(w, "slow down", http.StatusTooManyRequests)
		})
		_, match, err := client.FetchIPLookup(context.Background(), "203.0.113.10")
		var rl *RateLimitError
		if !errors.As(err, &rl) {
			t.Fatalf("error = %v, want RateLimitError", err)
		}
		if rl.RetryAfter != 45*time.Second {
			t.Errorf("RetryAfter = %s, want 45s", rl.RetryAfter)
		}
		if match != model.IPMatchUnknown {
			t.Errorf("match on rate limit = %q, want unknown", match)
		}
	})

	t.Run("default retry after", func(t *testing.T) {
		client, _, _ := newNetCoffeeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "slow down", http.StatusTooManyRequests)
		})
		_, err := client.FetchClaudeTrust(context.Background(), "203.0.113.10")
		var rl *RateLimitError
		if !errors.As(err, &rl) {
			t.Fatalf("error = %v, want RateLimitError", err)
		}
		if rl.RetryAfter != time.Minute {
			t.Errorf("default RetryAfter = %s, want 1m", rl.RetryAfter)
		}
	})
}

func TestFetchIPLookupSubsetAndValidation(t *testing.T) {
	responses := map[string]map[string]any{
		"/api/ip/lookup/203.0.113.10": {
			"ip": "203.0.113.10", "cidr": "203.0.113.0/24", "trust_score": 64,
			"asn": 64500, "asOrganization": "Example ISP", "country": "United States",
			"countryCode": "us", "company_type": "isp", "isResidential": true,
			"is_datacenter": false, "is_vpn": false, "is_proxy": false, "is_tor": false,
			"abuser_score": "0.001 (Low)",
			"ai_verdict": map[string]any{"label": "Clean residential", "confidence": 90},
			"related_domains": []string{"should-not-be-stored.example"},
			"geo_sources":     []map[string]any{{"src": "g1", "country": "United States"}},
		},
		"/api/ip/lookup/203.0.113.20": {
			"ip": "203.0.113.21", "cidr": "203.0.113.0/24", "trust_score": 20,
		},
		"/api/ip/lookup/203.0.113.30": {
			"cidr": "203.0.113.0/24", "trust_score": 99,
		},
	}
	client, _, _ := newNetCoffeeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		resp, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})

	body, match, err := client.FetchIPLookup(context.Background(), "203.0.113.10")
	if err != nil {
		t.Fatalf("FetchIPLookup: %v", err)
	}
	if match != model.IPMatchExact {
		t.Errorf("lookup match = %q, want exact", match)
	}
	var subset map[string]any
	if err := json.Unmarshal(body, &subset); err != nil {
		t.Fatalf("unmarshal lookup subset: %v", err)
	}
	if subset["ip_match"] != model.IPMatchExact {
		t.Errorf("subset ip_match = %v, want exact", subset["ip_match"])
	}
	if subset["query_ip"] != "203.0.113.10" {
		t.Errorf("subset query_ip = %v, want queried address", subset["query_ip"])
	}
	if _, ok := subset["related_domains"]; ok {
		t.Error("bulky related_domains must be dropped from the stored subset")
	}
	if _, ok := subset["ai_verdict"]; !ok {
		t.Error("documented ai_verdict field must be kept")
	}
	if _, ok := subset["geo_sources"]; !ok {
		t.Error("documented geo_sources field must be kept")
	}

	if _, match, err := client.FetchIPLookup(context.Background(), "203.0.113.20"); err != nil {
		t.Fatalf("FetchIPLookup cidr: %v", err)
	} else if match != model.IPMatchCIDR {
		t.Errorf("lookup same /24 match = %q, want cidr", match)
	}

	if _, match, err := client.FetchIPLookup(context.Background(), "203.0.113.30"); err != nil {
		t.Fatalf("FetchIPLookup missing ip: %v", err)
	} else if match != model.IPMatchUnknown {
		t.Errorf("lookup missing response IP match = %q, want unknown", match)
	}
}

func TestNetCoffeeHTTPErrorSanitized(t *testing.T) {
	client, _, _ := newNetCoffeeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failure", http.StatusInternalServerError)
	})
	_, err := client.FetchClaudeTrust(context.Background(), "203.0.113.10")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	if strings.Contains(err.Error(), "token") || strings.Contains(err.Error(), "203.0.113.10") {
		t.Errorf("error leaked request details: %v", err)
	}
}

func TestNetCoffeeMalformedJSON(t *testing.T) {
	client, _, _ := newNetCoffeeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{not-json")
	})
	if _, err := client.FetchClaudeTrust(context.Background(), "203.0.113.10"); err == nil {
		t.Fatal("expected parse error for malformed Claude response")
	}
	if _, _, err := client.FetchIPLookup(context.Background(), "203.0.113.10"); err == nil {
		t.Fatal("expected parse error for malformed lookup response")
	}
}
