package service

import (
	"net/http"
	"net/http/httptest"
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
