package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gejiliang/mihomo-cp/internal/service"
)

// contextKey is a custom type to avoid context key collisions.
type contextKey string

const (
	contextKeyUserID contextKey = "user_id"
	contextKeyRole   contextKey = "role"
)

// authError writes a JSON error response without importing the handler package
// (to avoid import cycles).
func authError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

// Auth returns a middleware that validates Bearer JWT tokens and injects
// user_id and role into the request context.
func Auth(authSvc *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				authError(w, http.StatusUnauthorized, "unauthorized", "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				authError(w, http.StatusUnauthorized, "unauthorized", "invalid authorization header format")
				return
			}

			claims, err := authSvc.ValidateToken(parts[1])
			if err != nil {
				authError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
				return
			}

			if claims.Type != "access" {
				authError(w, http.StatusUnauthorized, "unauthorized", "token is not an access token")
				return
			}

			ctx := context.WithValue(r.Context(), contextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, contextKeyRole, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin is a middleware that rejects requests where the role is not "admin".
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, ok := r.Context().Value(contextKeyRole).(string)
		if !ok || role != "admin" {
			authError(w, http.StatusForbidden, "forbidden", "admin role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetUserID retrieves the authenticated user's ID from the context.
func GetUserID(ctx context.Context) string {
	v, _ := ctx.Value(contextKeyUserID).(string)
	return v
}

// GetRole retrieves the authenticated user's role from the context.
func GetRole(ctx context.Context) string {
	v, _ := ctx.Value(contextKeyRole).(string)
	return v
}
