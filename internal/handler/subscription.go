package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
)

// SubscriptionHandler exposes subscription management (draft-only mutations,
// admin enforced) per docs/subscription-score-api.md.
type SubscriptionHandler struct {
	svc *service.SubscriptionService
}

// NewSubscriptionHandler creates the handler.
func NewSubscriptionHandler(svc *service.SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{svc: svc}
}

// List handles GET /api/subscriptions
func (h *SubscriptionHandler) List(w http.ResponseWriter, r *http.Request) {
	views, err := h.svc.ListViews()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, views)
}

// Get handles GET /api/subscriptions/{id}
func (h *SubscriptionHandler) Get(w http.ResponseWriter, r *http.Request) {
	view, err := h.svc.GetView(r.PathValue("id"))
	if err != nil {
		Error(w, 404, "not_found", "subscription not found")
		return
	}
	JSON(w, 200, view)
}

// Discover handles GET /api/subscriptions/providers
func (h *SubscriptionHandler) Discover(w http.ResponseWriter, r *http.Request) {
	providers, err := h.svc.DiscoverProviders()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, providers)
}

// Create handles POST /api/subscriptions
func (h *SubscriptionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req service.SubscriptionRequest
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	view, draftChanged, err := h.svc.Create(&req)
	if err != nil {
		writeSubError(w, err)
		return
	}
	if draftChanged {
		view.DraftChanged = true
	}
	JSON(w, 201, view)
}

// Update handles PUT /api/subscriptions/{id}
func (h *SubscriptionHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req service.SubscriptionRequest
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	view, draftChanged, err := h.svc.Update(r.PathValue("id"), &req)
	if err != nil {
		writeSubError(w, err)
		return
	}
	if draftChanged {
		view.DraftChanged = true
	}
	JSON(w, 200, view)
}

// Delete handles DELETE /api/subscriptions/{id}
func (h *SubscriptionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.Delete(r.PathValue("id"))
	if err != nil {
		writeSubError(w, err)
		return
	}
	JSON(w, 200, result)
}

// Refresh handles POST /api/subscriptions/{id}/refresh
func (h *SubscriptionHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	view, refErr := h.svc.Refresh(r.PathValue("id"))
	if refErr != nil && view == nil {
		Error(w, statusForRefreshCode(refErr.Code), refErr.Code, refErr.Message)
		return
	}
	JSON(w, 200, map[string]any{
		"subscription": view,
		"error":        refErr,
	})
}

func statusForRefreshCode(code string) int {
	switch code {
	case "not_found":
		return 404
	case "rate_limited":
		return 429
	default:
		return 502
	}
}

// writeSubError maps service errors onto API errors. Referenced-provider
// deletions surface their references for the GUI.
func writeSubError(w http.ResponseWriter, err error) {
	var conflict *service.ErrConflict
	if errors.As(err, &conflict) {
		Error(w, 409, "conflict", conflict.Message)
		return
	}
	var ref *service.ErrReferenced
	if errors.As(err, &ref) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(409)
		writeEnvelope(w, APIResponse{Error: &APIError{
			Code:       "referenced",
			Message:    "subscription provider is still referenced by proxy-groups; remove the references first",
			References: ref.References,
		}})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		Error(w, 404, "not_found", "subscription not found")
		return
	}
	msg := err.Error()
	if strings.Contains(msg, "UNIQUE constraint failed") {
		// Concurrent duplicate create raced past the pre-check and hit the DB
		// unique index — surface it as a conflict, not a bad request.
		Error(w, 409, "conflict", "subscription or provider already managed")
		return
	}
	Error(w, 400, "bad_request", msg)
}
