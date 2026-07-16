package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/store"
	"github.com/google/uuid"
)

// ProxyGroupHandler handles CRUD operations for proxy groups.
type ProxyGroupHandler struct {
	proxyGroups *store.ProxyGroupStore
	rules       *store.RuleStore
}

// NewProxyGroupHandler creates a new ProxyGroupHandler.
func NewProxyGroupHandler(pgs *store.ProxyGroupStore, rs *store.RuleStore) *ProxyGroupHandler {
	return &ProxyGroupHandler{proxyGroups: pgs, rules: rs}
}

// List handles GET /api/proxy-groups
func (h *ProxyGroupHandler) List(w http.ResponseWriter, r *http.Request) {
	groups, err := h.proxyGroups.List()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, groups)
}

// Get handles GET /api/proxy-groups/{id}
func (h *ProxyGroupHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	g, err := h.proxyGroups.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "proxy group not found")
		return
	}
	JSON(w, 200, g)
}

// Create handles POST /api/proxy-groups
func (h *ProxyGroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	var g model.ProxyGroup
	if err := DecodeJSON(r, &g); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	g.ID = uuid.New().String()
	if err := h.proxyGroups.Create(&g); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 201, g)
}

// Update handles PUT /api/proxy-groups/{id}
func (h *ProxyGroupHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var g model.ProxyGroup
	if err := DecodeJSON(r, &g); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	g.ID = id
	if err := h.proxyGroups.Update(&g); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, g)
}

// Delete handles DELETE /api/proxy-groups/{id}
func (h *ProxyGroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	refs, _ := h.findRefs(id)
	if len(refs) > 0 {
		Error(w, 409, "has_references", "proxy group is referenced by: "+strings.Join(refs, ", "))
		return
	}
	if err := h.proxyGroups.Delete(id); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "deleted"})
}

// Copy handles POST /api/proxy-groups/{id}/copy
func (h *ProxyGroupHandler) Copy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	g, err := h.proxyGroups.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "proxy group not found")
		return
	}
	g.ID = uuid.New().String()
	g.Name = g.Name + " (copy)"
	if err := h.proxyGroups.Create(g); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 201, g)
}

// Reorder handles POST /api/proxy-groups/reorder
func (h *ProxyGroupHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	if err := h.proxyGroups.Reorder(req.IDs); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "ok"})
}

// Refs handles GET /api/proxy-groups/{id}/refs
func (h *ProxyGroupHandler) Refs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	refs, err := h.findRefs(id)
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, refs)
}

func (h *ProxyGroupHandler) findRefs(id string) ([]string, error) {
	group, err := h.proxyGroups.GetByID(id)
	if err != nil {
		return nil, err
	}

	var refs []string

	// Check other groups' members
	allGroups, _ := h.proxyGroups.List()
	for _, g := range allGroups {
		if g.ID == id {
			continue
		}
		var members []string
		_ = json.Unmarshal(g.Members, &members)
		for _, m := range members {
			if m == group.Name {
				refs = append(refs, "group:"+g.Name)
			}
		}
	}

	// Check rules' targets
	allRules, _ := h.rules.List("", "")
	for _, r := range allRules {
		if r.Target == group.Name {
			refs = append(refs, "rule:"+r.Type+","+r.Payload)
		}
	}

	return refs, nil
}
