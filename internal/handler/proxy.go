package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gejiliang/mihomo-cp/internal/middleware"
	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
	"github.com/google/uuid"
)

// ProxyHandler handles CRUD operations for proxies.
type ProxyHandler struct {
	proxies     *store.ProxyStore
	proxyGroups *store.ProxyGroupStore
	scoring     *service.ScoringService
	settings    *store.SettingsStore
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(ps *store.ProxyStore, pgs *store.ProxyGroupStore, scoring *service.ScoringService, ss *store.SettingsStore) *ProxyHandler {
	return &ProxyHandler{proxies: ps, proxyGroups: pgs, scoring: scoring, settings: ss}
}

// sensitiveProxyKeys are node-config fields that carry credentials. Reads by
// non-admin users get these redacted (top level and one nesting level down,
// e.g. reality-opts.private-key).
var sensitiveProxyKeys = map[string]bool{
	"password": true, "uuid": true, "token": true, "psk": true,
	"private-key": true, "private_key": true, "secret": true, "auth": true,
}

// sanitizeProxyConfig redacts credential fields from a proxy config JSON.
func sanitizeProxyConfig(raw json.RawMessage) json.RawMessage {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw // unparsable: return as stored
	}
	redactMap(m)
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}

func redactMap(m map[string]any) {
	for k, v := range m {
		if nested, ok := v.(map[string]any); ok {
			redactMap(nested)
			continue
		}
		if sensitiveProxyKeys[k] {
			m[k] = "[redacted]"
		}
	}
}

// List handles GET /api/proxies
func (h *ProxyHandler) List(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	proxyType := r.URL.Query().Get("type")
	proxies, err := h.proxies.List(search, proxyType)
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	if middleware.GetRole(r.Context()) != "admin" {
		for _, p := range proxies {
			p.Config = sanitizeProxyConfig(p.Config)
		}
	}
	JSON(w, 200, proxies)
}

// Get handles GET /api/proxies/{id}
func (h *ProxyHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.proxies.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "proxy not found")
		return
	}
	if middleware.GetRole(r.Context()) != "admin" {
		p.Config = sanitizeProxyConfig(p.Config)
	}
	JSON(w, 200, p)
}

// enqueueScoreAsync queues a real-egress score for the proxy after
// create/update. The isolated egress test replaces the old global-mode
// country detection; results (exit IP, trust score, country) land on the
// node-scores endpoints.
func (h *ProxyHandler) enqueueScoreAsync(proxyName string) {
	if h.scoring == nil {
		return
	}
	go h.scoring.EnqueueStaticProxy(proxyName)
}

// Create handles POST /api/proxies
func (h *ProxyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var p model.Proxy
	if err := DecodeJSON(r, &p); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	p.ID = uuid.New().String()
	if err := h.proxies.Create(&p); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	h.enqueueScoreAsync(p.Name)
	JSON(w, 201, p)
}

// Update handles PUT /api/proxies/{id}
func (h *ProxyHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var p model.Proxy
	if err := DecodeJSON(r, &p); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	p.ID = id
	if err := h.proxies.Update(&p); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	h.enqueueScoreAsync(p.Name)
	JSON(w, 200, p)
}

// Delete handles DELETE /api/proxies/{id}
// If the proxy is referenced by groups, it removes the reference from those groups first.
func (h *ProxyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	proxy, err := h.proxies.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "proxy not found")
		return
	}

	// Remove this proxy from any groups that reference it
	groups, _ := h.proxyGroups.List()
	for _, g := range groups {
		var members []string
		_ = json.Unmarshal(g.Members, &members)
		filtered := make([]string, 0, len(members))
		changed := false
		for _, m := range members {
			if m == proxy.Name {
				changed = true
			} else {
				filtered = append(filtered, m)
			}
		}
		if changed {
			newMembers, _ := json.Marshal(filtered)
			g.Members = newMembers
			_ = h.proxyGroups.Update(g)
		}
	}

	if err := h.proxies.Delete(id); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	if h.scoring != nil {
		h.scoring.RemoveStaticProxy(proxy.Name)
	}
	JSON(w, 200, map[string]string{"status": "deleted"})
}

// Copy handles POST /api/proxies/{id}/copy
func (h *ProxyHandler) Copy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.proxies.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "proxy not found")
		return
	}
	p.ID = uuid.New().String()
	p.Name = p.Name + " (copy)"
	if err := h.proxies.Create(p); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 201, p)
}

// Reorder handles POST /api/proxies/reorder
func (h *ProxyHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	if err := h.proxies.Reorder(req.IDs); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "ok"})
}

// Refs handles GET /api/proxies/{id}/refs
func (h *ProxyHandler) Refs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	refs, err := h.findRefs(id)
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, refs)
}

func (h *ProxyHandler) findRefs(id string) ([]string, error) {
	proxy, err := h.proxies.GetByID(id)
	if err != nil {
		return nil, err
	}
	groups, _ := h.proxyGroups.List()
	var refs []string
	for _, g := range groups {
		var members []string
		_ = json.Unmarshal(g.Members, &members)
		for _, m := range members {
			if m == proxy.Name {
				refs = append(refs, g.Name)
			}
		}
	}
	return refs, nil
}
