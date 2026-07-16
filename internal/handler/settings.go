package handler

import (
	"net/http"
	"os"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
	"github.com/google/uuid"
)

// SettingsHandler handles app settings and user management.
type SettingsHandler struct {
	settings *store.SettingsStore
	users    *store.UserStore
	authSvc  *service.AuthService
}

// NewSettingsHandler creates a new SettingsHandler.
func NewSettingsHandler(settings *store.SettingsStore, users *store.UserStore, authSvc *service.AuthService) *SettingsHandler {
	return &SettingsHandler{settings: settings, users: users, authSvc: authSvc}
}

// GetSettings handles GET /api/settings
func (h *SettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.settings.Get()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, st)
}

// UpdateSettings handles PUT /api/settings
func (h *SettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var st model.AppSettings
	if err := DecodeJSON(r, &st); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	if err := h.settings.Update(&st); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, st)
}

// ListUsers handles GET /api/settings/users
func (h *SettingsHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.List()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, users)
}

// CreateUser handles POST /api/settings/users
func (h *SettingsHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		Error(w, 400, "bad_request", "username and password are required")
		return
	}
	if req.Role == "" {
		req.Role = "readonly"
	}

	hashed, err := h.authSvc.HashPassword(req.Password)
	if err != nil {
		Error(w, 500, "internal", "failed to hash password")
		return
	}

	user := &model.User{
		ID:       uuid.New().String(),
		Username: req.Username,
		Password: hashed,
		Role:     req.Role,
	}
	if err := h.users.Create(user); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 201, user)
}

// UpdateUser handles PUT /api/settings/users/{id}
func (h *SettingsHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}

	user, err := h.users.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "user not found")
		return
	}

	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Role != "" {
		user.Role = req.Role
	}
	if req.Password != "" {
		hashed, err := h.authSvc.HashPassword(req.Password)
		if err != nil {
			Error(w, 500, "internal", "failed to hash password")
			return
		}
		user.Password = hashed
	}

	if err := h.users.Update(user); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, user)
}

// DeleteUser handles DELETE /api/settings/users/{id}
func (h *SettingsHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.users.Delete(id); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "deleted"})
}

// GetConfigYAML handles GET /api/settings/config-yaml
// Returns the raw config draft if it exists, otherwise reads the current config file from disk.
func (h *SettingsHandler) GetConfigYAML(w http.ResponseWriter, r *http.Request) {
	st, err := h.settings.Get()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}

	if st.RawConfigDraft != "" {
		JSON(w, 200, map[string]any{"content": st.RawConfigDraft, "source": "draft"})
		return
	}

	content, err := os.ReadFile(st.MihomoConfig)
	if err != nil {
		Error(w, 500, "internal", "failed to read config file: "+err.Error())
		return
	}
	JSON(w, 200, map[string]any{"content": string(content), "source": "file"})
}

// UpdateConfigYAML handles PUT /api/settings/config-yaml
// Saves raw YAML as a draft for later validation and publishing.
func (h *SettingsHandler) UpdateConfigYAML(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	if req.Content == "" {
		Error(w, 400, "bad_request", "content is required")
		return
	}
	if err := h.settings.SetRawConfigDraft(req.Content); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "saved"})
}

// DeleteConfigYAML handles DELETE /api/settings/config-yaml
// Clears the raw config draft.
func (h *SettingsHandler) DeleteConfigYAML(w http.ResponseWriter, r *http.Request) {
	if err := h.settings.ClearRawConfigDraft(); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "cleared"})
}
