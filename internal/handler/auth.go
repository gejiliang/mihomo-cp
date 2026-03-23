package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gejiliang/mihomo-cp/internal/middleware"
	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
	"github.com/google/uuid"
)

// AuthHandler handles authentication-related HTTP requests.
type AuthHandler struct {
	users   *store.UserStore
	authSvc *service.AuthService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(users *store.UserStore, authSvc *service.AuthService) *AuthHandler {
	return &AuthHandler{users: users, authSvc: authSvc}
}

// Login handles POST /api/auth/login.
// On first run (no users exist), it auto-creates a default admin/admin user.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req model.LoginRequest
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.Username == "" || req.Password == "" {
		Error(w, http.StatusBadRequest, "bad_request", "username and password are required")
		return
	}

	// Auto-create default admin on first run.
	count, err := h.users.Count()
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal_error", "failed to check users")
		return
	}
	if count == 0 {
		hashed, err := h.authSvc.HashPassword("admin")
		if err != nil {
			Error(w, http.StatusInternalServerError, "internal_error", "failed to hash password")
			return
		}
		defaultAdmin := &model.User{
			ID:       uuid.New().String(),
			Username: "admin",
			Password: hashed,
			Role:     "admin",
		}
		if err := h.users.Create(defaultAdmin); err != nil {
			Error(w, http.StatusInternalServerError, "internal_error", "failed to create default admin")
			return
		}
	}

	user, err := h.users.GetByUsername(req.Username)
	if err != nil {
		// Distinguish between not found and other errors.
		Error(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}

	if !h.authSvc.VerifyPassword(user.Password, req.Password) {
		Error(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}

	accessToken, err := h.authSvc.GenerateAccessToken(user.ID, user.Role)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal_error", "failed to generate access token")
		return
	}
	refreshToken, err := h.authSvc.GenerateRefreshToken(user.ID, user.Role)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal_error", "failed to generate refresh token")
		return
	}

	JSON(w, http.StatusOK, model.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}

// Refresh handles POST /api/auth/refresh.
// Accepts a refresh token and returns a new access token.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req model.RefreshRequest
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.RefreshToken == "" {
		Error(w, http.StatusBadRequest, "bad_request", "refresh_token is required")
		return
	}

	claims, err := h.authSvc.ValidateToken(req.RefreshToken)
	if err != nil {
		Error(w, http.StatusUnauthorized, "unauthorized", "invalid or expired refresh token")
		return
	}
	if claims.Type != "refresh" {
		Error(w, http.StatusUnauthorized, "unauthorized", "token is not a refresh token")
		return
	}

	accessToken, err := h.authSvc.GenerateAccessToken(claims.UserID, claims.Role)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal_error", "failed to generate access token")
		return
	}

	JSON(w, http.StatusOK, model.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: req.RefreshToken,
	})
}

// Me handles GET /api/auth/me.
// Returns the currently authenticated user's info.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		Error(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	user, err := h.users.GetByID(userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			Error(w, http.StatusNotFound, "not_found", "user not found")
		} else {
			Error(w, http.StatusInternalServerError, "internal_error", "failed to fetch user")
		}
		return
	}

	JSON(w, http.StatusOK, user)
}
