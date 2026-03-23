package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/store"
	"github.com/google/uuid"
)

// ProxyHandler handles CRUD operations for proxies.
type ProxyHandler struct {
	proxies     *store.ProxyStore
	proxyGroups *store.ProxyGroupStore
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(ps *store.ProxyStore, pgs *store.ProxyGroupStore) *ProxyHandler {
	return &ProxyHandler{proxies: ps, proxyGroups: pgs}
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
	JSON(w, 200, p)
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
