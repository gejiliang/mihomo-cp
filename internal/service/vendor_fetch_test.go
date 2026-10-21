package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fetchFrom(t *testing.T, srv *httptest.Server) (*VendorSnapshot, error) {
	t.Helper()
	return FetchVendorInfo(http.DefaultClient, srv.URL)
}

func TestFetchVendorInfoHeaderFormatWins(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("subscription-userinfo", "upload=10; download=20; total=100; expire=1794000000")
		_, _ = w.Write([]byte(`{"monthly_bw_limit_b":1,"bw_counter_b":2}`))
	}))
	t.Cleanup(srv.Close)

	snap, err := fetchFrom(t, srv)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if snap.Info == nil || !snap.Info.HasCounters {
		t.Fatalf("header payload = %+v, want directional counters", snap)
	}
	if snap.Info.Upload != 10 || snap.Info.Download != 20 || snap.Info.Total != 100 || snap.Info.Expire != 1794000000 {
		t.Errorf("header payload fields = %+v", snap.Info)
	}
	if snap.IsAggregate() {
		t.Error("header format must not be reported as aggregate")
	}
}

func TestFetchVendorInfoAggregateJSON(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantUsed   int64
		wantLimit  int64
		wantHasLim bool
		wantDay    int
		wantHasDay bool
	}{
		{
			name:       "full payload",
			body:       `{"monthly_bw_limit_b":1000000000000,"bw_counter_b":123456789,"bw_reset_day_of_month":20}`,
			wantUsed:   123456789, wantLimit: 1000000000000, wantHasLim: true, wantDay: 20, wantHasDay: true,
		},
		{
			name:       "real zero counter is a genuine zero",
			body:       `{"monthly_bw_limit_b":1000,"bw_counter_b":0}`,
			wantUsed:   0, wantLimit: 1000, wantHasLim: true,
		},
		{
			name:       "limit optional",
			body:       `{"bw_counter_b":5,"bw_reset_day_of_month":20}`,
			wantUsed:   5, wantDay: 20, wantHasDay: true,
		},
		{
			name:     "reset day optional",
			body:     `{"bw_counter_b":5}`,
			wantUsed: 5,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)

			snap, err := fetchFrom(t, srv)
			if err != nil {
				t.Fatalf("fetch: %v", err)
			}
			if !snap.IsAggregate() || snap.Info != nil {
				t.Fatalf("snapshot = %+v, want aggregate format", snap)
			}
			if snap.AggregateUsed == nil || *snap.AggregateUsed != tc.wantUsed {
				t.Errorf("used = %v, want %d", snap.AggregateUsed, tc.wantUsed)
			}
			if tc.wantHasLim {
				if snap.AggregateLimit == nil || *snap.AggregateLimit != tc.wantLimit {
					t.Errorf("limit = %v, want %d", snap.AggregateLimit, tc.wantLimit)
				}
			} else if snap.AggregateLimit != nil {
				t.Errorf("limit = %v, want absent", *snap.AggregateLimit)
			}
			if tc.wantHasDay {
				if snap.VendorResetDay == nil || *snap.VendorResetDay != tc.wantDay {
					t.Errorf("reset day = %v, want %d", snap.VendorResetDay, tc.wantDay)
				}
			} else if snap.VendorResetDay != nil {
				t.Errorf("reset day = %v, want absent", *snap.VendorResetDay)
			}
		})
	}
}

func TestFetchVendorInfoInvalidPayloadsFail(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"missing counter (quota only)", `{"monthly_bw_limit_b":1000}`},
		{"missing counter (reset day only)", `{"bw_reset_day_of_month":20}`},
		{"negative counter", `{"bw_counter_b":-1,"monthly_bw_limit_b":1000}`},
		{"wrong types mean no counter", `{"bw_counter_b":"12","monthly_bw_limit_b":[1],"bw_reset_day_of_month":"20"}`},
		{"empty object", `{}`},
		{"garbage", `not json`},
		{"html", `<html>blocked</html>`},
		{"json array", `[1,2,3]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)

			snap, err := fetchFrom(t, srv)
			if err == nil {
				t.Fatalf("snapshot = %+v, want explicit failure", snap)
			}
			if snap != nil {
				t.Errorf("a failed fetch must not return a snapshot: %+v", snap)
			}
		})
	}
}

func TestFetchVendorInfoInvalidResetDayAbsent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"bw_counter_b":7,"bw_reset_day_of_month":32}`))
	}))
	t.Cleanup(srv.Close)
	snap, err := fetchFrom(t, srv)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if snap.VendorResetDay != nil {
		t.Errorf("reset day 32 must be treated as absent, got %d", *snap.VendorResetDay)
	}
}

func TestFetchVendorInfoHTTPStatuses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	if _, err := fetchFrom(t, srv); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("error = %v, want HTTP 500", err)
	}

	rl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "90")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(rl.Close)
	_, err := fetchFrom(t, rl)
	var rate *RateLimitError
	if !errors.As(err, &rate) {
		t.Fatalf("error = %v, want RateLimitError", err)
	}
	if rate.RetryAfter != 90*1e9 {
		t.Errorf("retry after = %v, want 90s", rate.RetryAfter)
	}
}

// Network failures are sanitized: the endpoint (and any token in it) never
// appears in the error text that ends up in logs or stored error strings.
func TestFetchVendorInfoNetworkErrorSanitized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL + "/path?token=secret-token-value"
	srv.Close() // unreachable now; the URL contains the token

	_, err := FetchVendorInfo(&http.Client{Timeout: 2e9}, url)
	if err == nil {
		t.Fatal("expected a network error")
	}
	if strings.Contains(err.Error(), url) || strings.Contains(err.Error(), "secret-token-value") || strings.Contains(err.Error(), "http://") {
		t.Errorf("network error leaked the endpoint: %q", err.Error())
	}
}
