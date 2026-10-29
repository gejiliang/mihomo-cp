package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gejiliang/mihomo-cp/internal/service"
)

// newProxyProviderUpstream starts a fake mihomo controller and records the
// escaped request paths it receives.
func newProxyProviderUpstream(t *testing.T, status int, body string) (*service.MihomoClient, *[]string) {
	t.Helper()
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return service.NewMihomoClient(srv.URL, ""), &paths
}

func TestRuntimeProxyProvidersServesSafeInventory(t *testing.T) {
	client, paths := newProxyProviderUpstream(t, http.StatusOK, `{
	  "providers": {
	    "Example Provider": {
	      "name": "Example Provider", "type": "Proxy", "vehicleType": "HTTP",
	      "url": "https://subscription.example.com/private-token",
	      "proxies": [{"name":"Demo HK / 01","type":"AnyTLS","alive":true,"server":"203.0.113.10","password":"super-secret"}]
	    }
	  }
	}`)
	h := NewRuntimeHandler(client)

	rec := httptest.NewRecorder()
	h.ProxyProviders(rec, httptest.NewRequest(http.MethodGet, "/api/runtime/proxy-providers", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(*paths) != 1 || (*paths)[0] != "/providers/proxies" {
		t.Fatalf("upstream paths = %#v, want [/providers/proxies]", *paths)
	}

	var resp struct {
		Data service.ProxyProviderInventory `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	if len(resp.Data.Providers) != 1 {
		t.Fatalf("providers = %d, want 1", len(resp.Data.Providers))
	}
	node := resp.Data.Providers[0].Nodes[0]
	if node.Name != "Demo HK / 01" || node.Type != "AnyTLS" || node.Provider != "Example Provider" {
		t.Errorf("node = %#v", node)
	}
	if node.Delay != nil || node.Alive == nil || !*node.Alive {
		t.Errorf("node metadata = %#v", node)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"subscription.example.com", "private-token", "203.0.113.10", "super-secret"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("response leaked %q: %s", forbidden, body)
		}
	}
}

func TestRuntimeProxyProvidersSurfacesUpstreamFailure(t *testing.T) {
	// mihomo error bodies may echo provider URLs/tokens; none of that may pass
	// through. Only a fixed, non-sensitive message is returned.
	const fakeSecret = "fake-secret-token-123456"
	client, _ := newProxyProviderUpstream(t, http.StatusInternalServerError,
		`{"message":"fetch https://subscription.example.com/sub?token=`+fakeSecret+` failed"}`)
	h := NewRuntimeHandler(client)

	rec := httptest.NewRecorder()
	h.ProxyProviders(rec, httptest.NewRequest(http.MethodGet, "/api/runtime/proxy-providers", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if resp.Error.Code != "upstream_error" {
		t.Errorf("error code = %q, want upstream_error", resp.Error.Code)
	}
	if resp.Error.Message != "proxy provider inventory unavailable" {
		t.Errorf("error message = %q, want the fixed message", resp.Error.Message)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{fakeSecret, "subscription.example.com"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("error response leaked %q: %s", forbidden, body)
		}
	}
}

func TestRuntimeProviderNodeDelayUsesProviderScopedUpstreamPath(t *testing.T) {
	type call struct{ path, query string }
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, call{path: r.URL.EscapedPath(), query: r.URL.RawQuery})
		w.Header().Set("Content-Type", "application/json")
		// Extra fields must never be projected to the client.
		_, _ = w.Write([]byte(`{"delay":42,"message":"upstream-secret-body"}`))
	}))
	t.Cleanup(srv.Close)
	h := NewRuntimeHandler(service.NewMihomoClient(srv.URL, ""))

	node := "Demo HK / 01 + #1 节点"
	requests := []struct{ provider, node string }{
		{"Demo/Provider +1", node},
		{"Other Provider", node},
	}
	for _, rc := range requests {
		req := httptest.NewRequest(http.MethodGet,
			"/api/runtime/proxy-providers/x/y/healthcheck?url=http%3A%2F%2Fexample.com%2Fgenerate_204&timeout=5000", nil)
		req.SetPathValue("provider", rc.provider)
		req.SetPathValue("node", rc.node)
		rec := httptest.NewRecorder()
		h.ProxyProviderNodeDelay(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Data struct {
				Delay int `json:"delay"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
		}
		if resp.Data.Delay != 42 {
			t.Errorf("delay = %d, want 42", resp.Data.Delay)
		}
		if strings.Contains(rec.Body.String(), "upstream-secret-body") {
			t.Errorf("response leaked a non-delay upstream field: %s", rec.Body.String())
		}
	}

	if len(calls) != 2 {
		t.Fatalf("upstream calls = %d, want 2", len(calls))
	}
	// Same node name under different providers must resolve to distinct
	// provider-scoped upstream paths (the provider is the first segment).
	wantPaths := []string{
		"/providers/proxies/Demo%2FProvider%20+1/Demo%20HK%20%2F%2001%20+%20%231%20%E8%8A%82%E7%82%B9/healthcheck",
		"/providers/proxies/Other%20Provider/Demo%20HK%20%2F%2001%20+%20%231%20%E8%8A%82%E7%82%B9/healthcheck",
	}
	if calls[0].path == calls[1].path {
		t.Fatalf("same-named nodes under different providers used the same upstream path %q", calls[0].path)
	}
	for i, want := range wantPaths {
		if calls[i].path != want {
			t.Errorf("upstream path %d = %q, want %q", i, calls[i].path, want)
		}
	}
	if !strings.Contains(calls[0].query, "timeout=5000") ||
		!strings.Contains(calls[0].query, "url=http%3A%2F%2Fexample.com%2Fgenerate_204") {
		t.Errorf("upstream query = %q, want url and timeout forwarded", calls[0].query)
	}
}

func TestRuntimeProviderNodeDelayHidesUpstreamErrorBody(t *testing.T) {
	const fakeSecret = "fake-secret-token-123456"
	client, _ := newProxyProviderUpstream(t, http.StatusBadGateway,
		`{"message":"fetch https://subscription.example.com/sub?token=`+fakeSecret+` failed"}`)
	h := NewRuntimeHandler(client)

	req := httptest.NewRequest(http.MethodGet, "/api/runtime/proxy-providers/x/y/healthcheck", nil)
	req.SetPathValue("provider", "Demo Provider")
	req.SetPathValue("node", "Demo Node")
	rec := httptest.NewRecorder()
	h.ProxyProviderNodeDelay(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if resp.Error.Code != "upstream_error" || resp.Error.Message != "proxy provider node healthcheck failed" {
		t.Errorf("error = %+v, want fixed upstream_error message", resp.Error)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{fakeSecret, "subscription.example.com"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("error response leaked %q: %s", forbidden, body)
		}
	}
}

// A provider node name with spaces, a slash and '#' must reach mihomo as a
// single properly escaped path segment — the name is used verbatim for the
// runtime delay test.
func TestRuntimeProxyDelayEncodesActualNodeName(t *testing.T) {
	client, paths := newProxyProviderUpstream(t, http.StatusOK, `{"delay":42}`)
	h := NewRuntimeHandler(client)

	req := httptest.NewRequest(http.MethodGet, "/api/runtime/proxies/x/delay?url=http%3A%2F%2Fexample.com%2Fgenerate_204&timeout=5000", nil)
	req.SetPathValue("name", "Demo HK / 01 #A")
	rec := httptest.NewRecorder()
	h.ProxyDelay(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(*paths) != 1 {
		t.Fatalf("upstream paths = %#v", *paths)
	}
	want := "/proxies/Demo%20HK%20%2F%2001%20%23A/delay"
	if (*paths)[0] != want {
		t.Errorf("upstream path = %q, want %q", (*paths)[0], want)
	}
}
