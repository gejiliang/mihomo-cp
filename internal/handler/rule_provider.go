package handler

import (
	"net/http"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
	"github.com/google/uuid"
)

// RuleProviderHandler handles CRUD operations for rule providers.
type RuleProviderHandler struct {
	providers *store.RuleProviderStore
	mihomo    *service.MihomoClient
}

// NewRuleProviderHandler creates a new RuleProviderHandler.
func NewRuleProviderHandler(rps *store.RuleProviderStore, mc *service.MihomoClient) *RuleProviderHandler {
	return &RuleProviderHandler{providers: rps, mihomo: mc}
}

// List handles GET /api/rule-providers
func (h *RuleProviderHandler) List(w http.ResponseWriter, r *http.Request) {
	providers, err := h.providers.List()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, providers)
}

// Get handles GET /api/rule-providers/{id}
func (h *RuleProviderHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.providers.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "rule provider not found")
		return
	}
	JSON(w, 200, p)
}

// Create handles POST /api/rule-providers
func (h *RuleProviderHandler) Create(w http.ResponseWriter, r *http.Request) {
	var p model.RuleProvider
	if err := DecodeJSON(r, &p); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	p.ID = uuid.New().String()
	if err := h.providers.Create(&p); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 201, p)
}

// Update handles PUT /api/rule-providers/{id}
func (h *RuleProviderHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var p model.RuleProvider
	if err := DecodeJSON(r, &p); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	p.ID = id
	if err := h.providers.Update(&p); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, p)
}

// Delete handles DELETE /api/rule-providers/{id}
func (h *RuleProviderHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.providers.Delete(id); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "deleted"})
}

// Refresh handles POST /api/rule-providers/{id}/refresh
func (h *RuleProviderHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.providers.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "rule provider not found")
		return
	}
	if err := h.mihomo.RefreshProvider(p.Name); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "ok"})
}
