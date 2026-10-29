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
	client, _ := newProxyProviderUpstream(t, http.StatusInternalServerError, `{"message":"boom"}`)
	h := NewRuntimeHandler(client)

	rec := httptest.NewRecorder()
	h.ProxyProviders(rec, httptest.NewRequest(http.MethodGet, "/api/runtime/proxy-providers", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if resp.Error.Code != "upstream_error" {
		t.Errorf("error code = %q, want upstream_error", resp.Error.Code)
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
