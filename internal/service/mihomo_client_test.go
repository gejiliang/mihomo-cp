package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMihomoClientGetProxyProviders(t *testing.T) {
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"providers":{}}`))
	}))
	t.Cleanup(srv.Close)

	raw, err := NewMihomoClient(srv.URL, "").GetProxyProviders()
	if err != nil {
		t.Fatalf("GetProxyProviders: %v", err)
	}
	if method != http.MethodGet || path != "/providers/proxies" {
		t.Errorf("request = %s %s, want GET /providers/proxies", method, path)
	}
	if string(raw) != `{"providers":{}}` {
		t.Errorf("body = %s", raw)
	}
}

func TestMihomoClientProviderNodeHealthcheck(t *testing.T) {
	var path, query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, query = r.URL.EscapedPath(), r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"delay":42}`))
	}))
	t.Cleanup(srv.Close)

	delay, err := NewMihomoClient(srv.URL, "").ProviderNodeHealthcheck(
		"Demo/Provider +1", "Demo HK / 01 #1 节点", "http://example.com/generate_204", 5000)
	if err != nil {
		t.Fatalf("ProviderNodeHealthcheck: %v", err)
	}
	if delay != 42 {
		t.Errorf("delay = %d, want 42", delay)
	}
	wantPath := "/providers/proxies/Demo%2FProvider%20+1/Demo%20HK%20%2F%2001%20%231%20%E8%8A%82%E7%82%B9/healthcheck"
	if path != wantPath {
		t.Errorf("path = %q, want %q", path, wantPath)
	}
	if !strings.Contains(query, "timeout=5000") || !strings.Contains(query, "url=http%3A%2F%2Fexample.com%2Fgenerate_204") {
		t.Errorf("query = %q, want url and timeout forwarded", query)
	}
}

func TestMihomoClientRefreshProviderPaths(t *testing.T) {
	type recorded struct {
		method string
		path   string
		auth   string
	}
	var got []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, recorded{
			method: r.Method,
			path:   r.URL.EscapedPath(),
			auth:   r.Header.Get("Authorization"),
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	client := NewMihomoClient(srv.URL, "test-secret")
	if err := client.RefreshProvider("Rule Provider"); err != nil {
		t.Fatalf("RefreshProvider: %v", err)
	}
	if err := client.RefreshProxyProvider("Proxy Provider"); err != nil {
		t.Fatalf("RefreshProxyProvider: %v", err)
	}

	want := []recorded{
		{method: http.MethodPut, path: "/providers/rules/Rule%20Provider", auth: "Bearer test-secret"},
		{method: http.MethodPut, path: "/providers/proxies/Proxy%20Provider", auth: "Bearer test-secret"},
	}
	if len(got) != len(want) {
		t.Fatalf("requests = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("request %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}
