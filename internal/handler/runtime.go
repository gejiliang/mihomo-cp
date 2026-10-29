package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gejiliang/mihomo-cp/internal/service"
)

// RuntimeHandler proxies requests to mihomo's external-controller.
type RuntimeHandler struct {
	mihomo *service.MihomoClient
}

// NewRuntimeHandler creates a new RuntimeHandler.
func NewRuntimeHandler(mc *service.MihomoClient) *RuntimeHandler {
	return &RuntimeHandler{mihomo: mc}
}

// Connections handles GET /api/runtime/connections
func (h *RuntimeHandler) Connections(w http.ResponseWriter, r *http.Request) {
	data, err := h.mihomo.GetConnections()
	if err != nil {
		Error(w, 502, "upstream_error", err.Error())
		return
	}
	JSON(w, 200, json.RawMessage(data))
}

// CloseConnection handles DELETE /api/runtime/connections/{id}
func (h *RuntimeHandler) CloseConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.mihomo.CloseConnection(id); err != nil {
		Error(w, 502, "upstream_error", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "closed"})
}

// Proxies handles GET /api/runtime/proxies
func (h *RuntimeHandler) Proxies(w http.ResponseWriter, r *http.Request) {
	data, err := h.mihomo.GetProxies()
	if err != nil {
		Error(w, 502, "upstream_error", err.Error())
		return
	}
	JSON(w, 200, json.RawMessage(data))
}

// ProxyDelay handles GET /api/runtime/proxies/{name}/delay
func (h *RuntimeHandler) ProxyDelay(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	testURL := r.URL.Query().Get("url")
	timeoutStr := r.URL.Query().Get("timeout")
	timeout := 5000
	if timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil {
			timeout = t
		}
	}

	data, err := h.mihomo.GetProxyDelay(name, testURL, timeout)
	if err != nil {
		Error(w, 502, "upstream_error", err.Error())
		return
	}
	JSON(w, 200, json.RawMessage(data))
}

// SwitchProxy handles PUT /api/runtime/proxies/{group}/selected
func (h *RuntimeHandler) SwitchProxy(w http.ResponseWriter, r *http.Request) {
	group := r.PathValue("group")
	var req struct {
		Name string `json:"name"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	if err := h.mihomo.SwitchProxy(group, req.Name); err != nil {
		Error(w, 502, "upstream_error", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "ok"})
}

// Rules handles GET /api/runtime/rules
func (h *RuntimeHandler) Rules(w http.ResponseWriter, r *http.Request) {
	data, err := h.mihomo.GetRules()
	if err != nil {
		Error(w, 502, "upstream_error", err.Error())
		return
	}
	JSON(w, 200, json.RawMessage(data))
}

// Providers handles GET /api/runtime/providers
func (h *RuntimeHandler) Providers(w http.ResponseWriter, r *http.Request) {
	data, err := h.mihomo.GetProviders()
	if err != nil {
		Error(w, 502, "upstream_error", err.Error())
		return
	}
	JSON(w, 200, json.RawMessage(data))
}

// ProxyProviders handles GET /api/runtime/proxy-providers: the read-only
// inventory of native proxy-provider (subscription) nodes, projected to safe
// metadata only (no provider URL, server address or node credential).
// Upstream errors are replaced by a fixed message: mihomo error bodies can
// echo the provider URL or other sensitive data.
func (h *RuntimeHandler) ProxyProviders(w http.ResponseWriter, r *http.Request) {
	raw, err := h.mihomo.GetProxyProviders()
	if err != nil {
		Error(w, 502, "upstream_error", "proxy provider inventory unavailable")
		return
	}
	inventory, err := service.BuildProxyProviderInventory(raw)
	if err != nil {
		Error(w, 502, "upstream_error", "proxy provider inventory unavailable")
		return
	}
	JSON(w, 200, inventory)
}

// ProxyProviderNodeDelay handles
// GET /api/runtime/proxy-providers/{provider}/{node}/healthcheck.
//
// Provider nodes must be tested through mihomo's provider-scoped healthcheck:
// the global /proxies/{name}/delay endpoint cannot address the same node name
// under two different providers. Provider and node arrive as separate
// path segments (decoded by the router) and are re-escaped for upstream.
// Only the numeric delay is projected; upstream bodies and errors never pass
// through.
func (h *RuntimeHandler) ProxyProviderNodeDelay(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	node := r.PathValue("node")
	testURL := r.URL.Query().Get("url")
	timeout := 5000
	if timeoutStr := r.URL.Query().Get("timeout"); timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil {
			timeout = t
		}
	}
	delay, err := h.mihomo.ProviderNodeHealthcheck(provider, node, testURL, timeout)
	if err != nil {
		Error(w, 502, "upstream_error", "proxy provider node healthcheck failed")
		return
	}
	JSON(w, 200, map[string]int{"delay": delay})
}

// RefreshProvider handles PUT /api/runtime/providers/rules/{name}
func (h *RuntimeHandler) RefreshProvider(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.mihomo.RefreshProvider(name); err != nil {
		Error(w, 502, "upstream_error", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "ok"})
}

// Version handles GET /api/runtime/version
func (h *RuntimeHandler) Version(w http.ResponseWriter, r *http.Request) {
	data, err := h.mihomo.GetVersion()
	if err != nil {
		Error(w, 502, "upstream_error", err.Error())
		return
	}
	JSON(w, 200, json.RawMessage(data))
}
