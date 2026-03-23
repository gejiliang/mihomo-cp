package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gejiliang/mihomo-cp/internal/store"
)

// SystemConfigHandler handles get/put for the single-row system config.
type SystemConfigHandler struct {
	configs *store.ConfigStore
}

// NewSystemConfigHandler creates a new SystemConfigHandler.
func NewSystemConfigHandler(cs *store.ConfigStore) *SystemConfigHandler {
	return &SystemConfigHandler{configs: cs}
}

// Get handles GET /api/system-config
func (h *SystemConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.configs.Get()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, cfg)
}

// Update handles PUT /api/system-config
func (h *SystemConfigHandler) Update(w http.ResponseWriter, r *http.Request) {
	var body json.RawMessage
	if err := DecodeJSON(r, &body); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	if err := h.configs.Update(body); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, body)
}
