package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
	"github.com/google/uuid"
)

// RuleProviderHandler handles CRUD operations for rule providers.
type RuleProviderHandler struct {
	providers *store.RuleProviderStore
	settings  *store.SettingsStore
	mihomo    *service.MihomoClient
}

// NewRuleProviderHandler creates a new RuleProviderHandler.
func NewRuleProviderHandler(rps *store.RuleProviderStore, ss *store.SettingsStore, mc *service.MihomoClient) *RuleProviderHandler {
	return &RuleProviderHandler{providers: rps, settings: ss, mihomo: mc}
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

// resolveProviderPath returns the absolute path for a provider's list file.
func (h *RuleProviderHandler) resolveProviderPath(p *model.RuleProvider) (string, error) {
	var cfg map[string]any
	if err := json.Unmarshal(p.Config, &cfg); err != nil {
		return "", err
	}
	relPath, _ := cfg["path"].(string)
	if relPath == "" {
		return "", nil
	}
	st, err := h.settings.Get()
	if err != nil {
		return "", err
	}
	base := st.MihomoDir
	if base == "" {
		base = "/etc/mihomo"
	}
	// Resolve ./providers/foo.list relative to mihomo dir
	abs := filepath.Join(base, strings.TrimPrefix(relPath, "./"))
	// Security: ensure resolved path stays under mihomo dir
	abs = filepath.Clean(abs)
	if !strings.HasPrefix(abs, filepath.Clean(base)) {
		return "", nil
	}
	return abs, nil
}

// GetContent handles GET /api/rule-providers/{id}/content
func (h *RuleProviderHandler) GetContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.providers.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "rule provider not found")
		return
	}
	path, err := h.resolveProviderPath(p)
	if err != nil || path == "" {
		Error(w, 400, "bad_request", "provider has no file path")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			JSON(w, 200, map[string]string{"content": ""})
			return
		}
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"content": string(data)})
}

// UpdateContent handles PUT /api/rule-providers/{id}/content
func (h *RuleProviderHandler) UpdateContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.providers.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "rule provider not found")
		return
	}
	path, err := h.resolveProviderPath(p)
	if err != nil || path == "" {
		Error(w, 400, "bad_request", "provider has no file path")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		Error(w, 400, "bad_request", "failed to read body")
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		Error(w, 400, "bad_request", "invalid JSON")
		return
	}
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	if err := os.WriteFile(path, []byte(req.Content), 0o644); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "ok"})
}
