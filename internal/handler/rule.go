package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/store"
	"github.com/google/uuid"
)

// RuleHandler handles CRUD operations for rules.
type RuleHandler struct {
	rules *store.RuleStore
}

// NewRuleHandler creates a new RuleHandler.
func NewRuleHandler(rs *store.RuleStore) *RuleHandler {
	return &RuleHandler{rules: rs}
}

// List handles GET /api/rules
func (h *RuleHandler) List(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	ruleType := r.URL.Query().Get("type")
	rules, err := h.rules.List(search, ruleType)
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, rules)
}

// Get handles GET /api/rules/{id}
func (h *RuleHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rule, err := h.rules.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "rule not found")
		return
	}
	JSON(w, 200, rule)
}

// Create handles POST /api/rules
func (h *RuleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var rule model.Rule
	if err := DecodeJSON(r, &rule); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	rule.ID = uuid.New().String()
	if len(rule.Params) == 0 {
		rule.Params = json.RawMessage(`{}`)
	}
	if err := h.rules.Create(&rule); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 201, rule)
}

// Update handles PUT /api/rules/{id}
func (h *RuleHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var rule model.Rule
	if err := DecodeJSON(r, &rule); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	rule.ID = id
	if err := h.rules.Update(&rule); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, rule)
}

// Delete handles DELETE /api/rules/{id}
func (h *RuleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.rules.Delete(id); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "deleted"})
}

// Reorder handles POST /api/rules/reorder
func (h *RuleHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	if err := h.rules.Reorder(req.IDs); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "ok"})
}
