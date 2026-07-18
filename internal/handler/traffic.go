package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
)

// TrafficHandler exposes traffic accounting, provider management and alerting.
type TrafficHandler struct {
	traffic  *service.TrafficService
	store    *store.TrafficStore
	notifier *service.NotifyService
}

// NewTrafficHandler creates a new TrafficHandler.
func NewTrafficHandler(traffic *service.TrafficService, ts *store.TrafficStore, notifier *service.NotifyService) *TrafficHandler {
	return &TrafficHandler{traffic: traffic, store: ts, notifier: notifier}
}

// Summary handles GET /api/traffic/summary
func (h *TrafficHandler) Summary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.traffic.Summary()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, summary)
}

// History handles GET /api/traffic/history?days=30
func (h *TrafficHandler) History(w http.ResponseWriter, r *http.Request) {
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil {
			days = v
		}
	}
	history, err := h.traffic.History(days)
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, history)
}

// ListProviders handles GET /api/traffic/providers
func (h *TrafficHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.store.ListProviders()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, providers)
}

type providerRequest struct {
	Name            string   `json:"name"`
	QuotaBytes      int64    `json:"quota_bytes"`
	ResetDay        int      `json:"reset_day"`
	NodePatterns    []string `json:"node_patterns"`
	SubscriptionURL string   `json:"subscription_url"`
	Enabled         *bool    `json:"enabled"`
}

func (req *providerRequest) validate() string {
	if strings.TrimSpace(req.Name) == "" {
		return "name is required"
	}
	if req.QuotaBytes < 0 {
		return "quota_bytes must be >= 0"
	}
	if req.ResetDay < 1 || req.ResetDay > 31 {
		return "reset_day must be between 1 and 31"
	}
	return ""
}

// CreateProvider handles POST /api/traffic/providers
func (h *TrafficHandler) CreateProvider(w http.ResponseWriter, r *http.Request) {
	var req providerRequest
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	if msg := req.validate(); msg != "" {
		Error(w, 400, "bad_request", msg)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	p := &model.TrafficProvider{
		ID:              uuid.New().String(),
		Name:            strings.TrimSpace(req.Name),
		QuotaBytes:      req.QuotaBytes,
		ResetDay:        req.ResetDay,
		NodePatternList: req.NodePatterns,
		SubscriptionURL: strings.TrimSpace(req.SubscriptionURL),
		Enabled:         enabled,
	}
	if err := h.store.CreateProvider(p); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 201, p)
}

// UpdateProvider handles PUT /api/traffic/providers/{id}
func (h *TrafficHandler) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	p, err := h.store.GetProvider(r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			Error(w, 404, "not_found", "provider not found")
			return
		}
		Error(w, 500, "internal", err.Error())
		return
	}
	var req providerRequest
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	if msg := req.validate(); msg != "" {
		Error(w, 400, "bad_request", msg)
		return
	}
	p.Name = strings.TrimSpace(req.Name)
	p.QuotaBytes = req.QuotaBytes
	p.ResetDay = req.ResetDay
	p.NodePatternList = req.NodePatterns
	p.SubscriptionURL = strings.TrimSpace(req.SubscriptionURL)
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	if err := h.store.UpdateProvider(p); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, p)
}

// DeleteProvider handles DELETE /api/traffic/providers/{id}
func (h *TrafficHandler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteProvider(r.PathValue("id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			Error(w, 404, "not_found", "provider not found")
			return
		}
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "deleted"})
}

// Alerts handles GET /api/traffic/alerts
func (h *TrafficHandler) Alerts(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = v
		}
	}
	alerts, err := h.store.ListAlerts(limit)
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, alerts)
}

// GetSettings handles GET /api/traffic/settings
func (h *TrafficHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.store.GetSettings()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, st)
}

// UpdateSettings handles PUT /api/traffic/settings
func (h *TrafficHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var st model.TrafficSettings
	if err := DecodeJSON(r, &st); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	switch st.AlertChannel {
	case "", "bark", "ntfy", "webhook":
	default:
		Error(w, 400, "bad_request", "alert_channel must be one of: bark, ntfy, webhook, or empty")
		return
	}
	if st.AlertThresholds == "" {
		st.AlertThresholds = "[80,95]"
	}
	if err := h.store.UpdateSettings(&st); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, st)
}

// TestNotify handles POST /api/traffic/test-notify — sends a test message
// through the configured channel.
func (h *TrafficHandler) TestNotify(w http.ResponseWriter, r *http.Request) {
	st, err := h.store.GetSettings()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	if st.AlertChannel == "" {
		Error(w, 400, "bad_request", "no alert channel configured")
		return
	}
	if err := h.notifier.Send(st.AlertChannel, st.AlertTarget, "mihomo-cp 测试通知", "流量告警通知渠道工作正常。"); err != nil {
		Error(w, 502, "notify_failed", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "sent"})
}
