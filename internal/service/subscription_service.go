package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/store"
)

// StaleAfter marks vendor snapshots older than this as stale in the GUI.
const StaleAfter = 24 * time.Hour

// AutoUserinfoInterval is how often enabled subscriptions get their vendor
// snapshot refreshed automatically. Disabled subscriptions are skipped:
// enabled=false pauses automatic metadata refresh and automatic scoring, it
// does NOT detach the native provider from the config or stop traffic — the
// provider entry stays in the managed config and can still carry quota usage.
const AutoUserinfoInterval = 30 * time.Minute

// SubscriptionService maintains normalized subscription metadata against the
// system-config draft (the single config source of truth) and the vendor
// userinfo snapshots. It never publishes and never touches vendor rules, DNS
// or global settings.
type SubscriptionService struct {
	subs     *store.SubscriptionStore
	configs  *store.ConfigStore
	traffic  *store.TrafficStore
	scores   *store.NodeScoreStore
	groups   *store.ProxyGroupStore
	mihomo   *MihomoClient
	settings *store.SettingsStore
	scorer   *ScoringService
	http     *http.Client

	nowFn func() time.Time
}

// NewSubscriptionService creates the service. scorer may be nil in tests.
func NewSubscriptionService(
	subs *store.SubscriptionStore,
	configs *store.ConfigStore,
	traffic *store.TrafficStore,
	scores *store.NodeScoreStore,
	groups *store.ProxyGroupStore,
	mihomo *MihomoClient,
	settings *store.SettingsStore,
	scorer *ScoringService,
) *SubscriptionService {
	return &SubscriptionService{
		subs:     subs,
		configs:  configs,
		traffic:  traffic,
		scores:   scores,
		groups:   groups,
		mihomo:   mihomo,
		settings: settings,
		scorer:   scorer,
		http:     &http.Client{Timeout: 30 * time.Second},
		nowFn:    time.Now,
	}
}

// Run blocks until ctx is cancelled, refreshing vendor snapshots for enabled
// subscriptions on a fixed interval (manual refresh works regardless).
func (s *SubscriptionService) Run(ctx context.Context) {
	ticker := time.NewTicker(AutoUserinfoInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.autoRefreshUserinfo()
		}
	}
}

// autoRefreshUserinfo refreshes vendor snapshots of enabled subscriptions
// that have a provider entry with a URL. Disabled subscriptions are paused.
func (s *SubscriptionService) autoRefreshUserinfo() {
	subs, err := s.subs.List()
	if err != nil {
		return
	}
	for i := range subs {
		sub := &subs[i]
		if !sub.Enabled || sub.ProviderName == "" {
			continue
		}
		entry, err := s.findProviderEntry(sub.ProviderName)
		if err != nil || entry.URL == "" {
			continue
		}
		fetchURL := entry.URL
		info, err := s.fetchUserinfo(fetchURL)
		if err != nil {
			slog.Debug("subscription: auto userinfo refresh failed", "subscription", sub.Name, "err", err)
			continue // keep last success; do not mark error for background polls
		}
		// Re-read and persist snapshot fields only; drop results fetched from
		// a URL that is no longer current for the row's provider.
		fresh, gerr := s.subs.Get(sub.ID)
		if gerr != nil {
			continue
		}
		cur, cerr := s.findProviderEntry(fresh.ProviderName)
		if cerr != nil || cur.URL != fetchURL {
			continue
		}
		now := s.nowFn()
		fresh.UserinfoUpload = info.Upload
		fresh.UserinfoDownload = info.Download
		fresh.UserinfoTotal = info.Total
		fresh.UserinfoExpire = info.Expire
		fresh.UserinfoCountersSeen = info.HasCounters
		fresh.UserinfoFetchedAt = model.NewNullTime(now)
		fresh.LastSuccessAt = model.NewNullTime(now)
		fresh.LastRefreshError = ""
		if err := s.subs.Update(fresh); err != nil {
			slog.Warn("subscription: auto userinfo persist failed", "subscription", sub.Name, "err", err)
		}
	}
}

// ErrConflict marks uniqueness violations (duplicate subscription name or a
// provider already managed by another subscription) — mapped to HTTP 409.
type ErrConflict struct{ Message string }

func (e *ErrConflict) Error() string { return e.Message }

// ErrReferenced is returned when deletion is blocked by config references.
type ErrReferenced struct {
	References []string
}

func (e *ErrReferenced) Error() string {
	return "still referenced by proxy-groups: " + strings.Join(e.References, ", ")
}

// ── views ────────────────────────────────────────────────────────────────────

// SubscriptionView is the API projection of a subscription (contract:
// docs/subscription-score-api.md). Unknown values are null, never 0.
type SubscriptionView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Status    string `json:"status"` // ok | stale | error | disabled
	LastError string `json:"last_error"`

	ProviderName   string `json:"provider_name"`
	ProviderSource string `json:"provider_source"` // created | linked
	ProviderExists bool   `json:"provider_exists"`
	URLMasked      string `json:"url_masked"`
	NodeCount      *int   `json:"node_count"`

	QuotaBytes     *int64 `json:"quota_bytes"`
	UsedBytes      *int64 `json:"used_bytes"`
	RemainingBytes *int64 `json:"remaining_bytes"`
	UsageSource    string `json:"usage_source"` // vendor | local | manual | none
	LocalUsedBytes *int64 `json:"local_used_bytes"`

	ExpiresAt     *string `json:"expires_at"`
	ExpiresSource string  `json:"expires_source"` // vendor | manual | unknown

	ResetDay    int     `json:"reset_day"`
	ResetHour   int     `json:"reset_hour"`
	ResetMinute int     `json:"reset_minute"`
	Timezone    *string `json:"timezone"`
	CycleStart  *string `json:"cycle_start"`
	CycleEnd    *string `json:"cycle_end"`
	NextReset   *string `json:"next_reset"`

	UserinfoFetchedAt *string                `json:"userinfo_fetched_at"`
	LastRefreshAt     *string                `json:"last_refresh_at"`
	LastSuccessAt     *string                `json:"last_success_at"`
	Stale             bool                   `json:"stale"`
	Userinfo          *model.TrafficUserinfo `json:"userinfo"`
	TrafficProviderID string                 `json:"traffic_provider_id"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	DraftChanged bool `json:"draft_changed,omitempty"`
}

func rfc3339Time(t time.Time) *string {
	s := t.Format(time.RFC3339)
	return &s
}

// MaskURL reduces a subscription URL to scheme://host/… — the path, query and
// any embedded credentials never leave the process.
func MaskURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "[masked]"
	}
	return u.Scheme + "://" + u.Host + "/…"
}

// View projects a subscription row into its API shape.
func (s *SubscriptionService) View(sub *model.Subscription) (*SubscriptionView, error) {
	now := s.nowFn()
	v := &SubscriptionView{
		ID:                sub.ID,
		Name:              sub.Name,
		Enabled:           sub.Enabled,
		ProviderName:      sub.ProviderName,
		ProviderSource:    sub.ProviderSource,
		TrafficProviderID: sub.TrafficProviderID,
		CreatedAt:         sub.CreatedAt,
		UpdatedAt:         sub.UpdatedAt,
		UsageSource:       "none",
		ExpiresSource:     "unknown",
	}

	// Status. disabled means "automatic metadata refresh & auto scoring
	// paused" — the provider stays in the config and can still carry traffic.
	if !sub.Enabled {
		v.Status = "disabled"
	} else if sub.LastRefreshError != "" && (sub.LastSuccessAt.IsZero() || sub.LastRefreshAt.Time.After(sub.LastSuccessAt.Time)) {
		v.Status = "error"
	} else if sub.UserinfoFetchedAt.IsZero() || now.Sub(sub.UserinfoFetchedAt.Time) > StaleAfter {
		v.Status = "stale"
	} else {
		v.Status = "ok"
	}
	v.LastError = sub.LastRefreshError
	v.Stale = sub.UserinfoFetchedAt.IsZero() || now.Sub(sub.UserinfoFetchedAt.Time) > StaleAfter

	if !sub.LastRefreshAt.IsZero() {
		v.LastRefreshAt = rfc3339Time(sub.LastRefreshAt.Time)
	}
	if !sub.LastSuccessAt.IsZero() {
		v.LastSuccessAt = rfc3339Time(sub.LastSuccessAt.Time)
	}
	if !sub.UserinfoFetchedAt.IsZero() {
		v.UserinfoFetchedAt = rfc3339Time(sub.UserinfoFetchedAt.Time)
		v.Userinfo = &model.TrafficUserinfo{
			ProviderID: sub.ID,
			Upload:     sub.UserinfoUpload,
			Download:   sub.UserinfoDownload,
			Total:      sub.UserinfoTotal,
			Expire:     sub.UserinfoExpire,
			FetchedAt:  sub.UserinfoFetchedAt.Time,
		}
	}

	// Quota & usage. Vendor usage is "known" only when the header carried
	// both counters; a total/expire-only header must not become used=0.
	// Explicit upload=0;download=0 stays a genuine zero.
	vendorKnown := !sub.UserinfoFetchedAt.IsZero() && sub.UserinfoCountersSeen
	if vendorKnown {
		used := sub.UserinfoUpload + sub.UserinfoDownload
		v.UsedBytes = &used
		v.UsageSource = "vendor"
	}
	var quota *int64
	if sub.UserinfoTotal > 0 {
		t := sub.UserinfoTotal
		quota = &t
	} else if sub.QuotaBytes > 0 {
		t := sub.QuotaBytes
		quota = &t
	}
	v.QuotaBytes = quota
	if sub.TrafficProviderID != "" {
		if local, ok := s.localUsed(sub, now); ok {
			l := local
			v.LocalUsedBytes = &l
			if v.UsedBytes == nil {
				v.UsedBytes = &l
				v.UsageSource = "local"
			}
		}
	}
	if quota != nil && v.UsedBytes != nil {
		rem := *quota - *v.UsedBytes
		if rem < 0 {
			rem = 0 // clamp: never show negative remaining
		}
		v.RemainingBytes = &rem
	}

	// Expiry — vendor value first, manual supplement second. Never derived
	// from the reset schedule.
	if sub.UserinfoExpire > 0 {
		v.ExpiresAt = rfc3339Time(time.Unix(sub.UserinfoExpire, 0).UTC())
		v.ExpiresSource = "vendor"
	} else if sub.ManualExpire > 0 {
		v.ExpiresAt = rfc3339Time(time.Unix(sub.ManualExpire, 0).UTC())
		v.ExpiresSource = "manual"
	}

	// Billing reset schedule.
	if sub.ResetDay >= 1 && sub.ResetDay <= 31 {
		policy := ResetPolicy{Day: sub.ResetDay, Hour: sub.ResetHour, Minute: sub.ResetMinute, Timezone: sub.ResetTimezone}
		if _, err := policy.Normalize(); err == nil {
			v.ResetDay, v.ResetHour, v.ResetMinute = sub.ResetDay, sub.ResetHour, sub.ResetMinute
			if sub.ResetTimezone != "" {
				tz := sub.ResetTimezone
				v.Timezone = &tz
			}
			start := CycleStartAt(now, policy)
			next := NextResetAt(now, policy)
			v.CycleStart = rfc3339Time(start)
			v.CycleEnd = rfc3339Time(next)
			v.NextReset = rfc3339Time(next)
		}
	}

	// Provider presence, masked URL, node count.
	if sub.ProviderName != "" {
		entry, err := s.findProviderEntry(sub.ProviderName)
		v.ProviderExists = err == nil
		if err == nil {
			v.URLMasked = MaskURL(entry.URL)
			if n, err := s.providerNodeCount(entry); err == nil {
				v.NodeCount = &n
			}
		}
	}
	return v, nil
}

// localUsed sums locally-sampled hourly usage of the linked traffic provider
// within the SUBSCRIPTION's current billing cycle (the View displays the
// subscription's reset schedule — using the shared traffic provider's policy
// here would compute a window that disagrees with the displayed one). The
// traffic provider contributes only node-pattern attribution; its own policy
// is untouched. Unknown subscription reset ⇒ local usage unknown.
func (s *SubscriptionService) localUsed(sub *model.Subscription, now time.Time) (int64, bool) {
	if sub.TrafficProviderID == "" || sub.ResetDay < 1 {
		return 0, false
	}
	tp, err := s.traffic.GetProvider(sub.TrafficProviderID)
	if err != nil {
		return 0, false
	}
	policy := ResetPolicy{Day: sub.ResetDay, Hour: sub.ResetHour, Minute: sub.ResetMinute, Timezone: sub.ResetTimezone}
	if _, err := policy.Normalize(); err != nil {
		return 0, false
	}
	start := CycleStartAt(now, policy)
	rows, err := s.traffic.HourlySince(start.Unix())
	if err != nil {
		return 0, false
	}
	var total int64
	for _, r := range rows {
		if MatchProvider(r.Node, []model.TrafficProvider{*tp}) == tp.ID {
			total += r.Upload + r.Download
		}
	}
	return total, true
}

// providerNodeCount reads the provider cache once and counts nodes.
func (s *SubscriptionService) providerNodeCount(e providerEntry) (int, error) {
	st, err := s.settings.Get()
	if err != nil {
		return 0, err
	}
	nodes, err := (&MihomoIsolation{}).LoadProviderNodes(e, st.MihomoDir)
	if err != nil {
		return 0, err
	}
	return len(nodes), nil
}

// ListViews returns all subscriptions projected.
func (s *SubscriptionService) ListViews() ([]*SubscriptionView, error) {
	subs, err := s.subs.List()
	if err != nil {
		return nil, err
	}
	out := make([]*SubscriptionView, 0, len(subs))
	for i := range subs {
		v, err := s.View(&subs[i])
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// GetView returns one subscription projected.
func (s *SubscriptionService) GetView(id string) (*SubscriptionView, error) {
	sub, err := s.subs.Get(id)
	if err != nil {
		return nil, err
	}
	return s.View(sub)
}

// ── draft provider manipulation ──────────────────────────────────────────────

// draftConfig decodes the system config draft into a mutable map.
func (s *SubscriptionService) draftConfig() (map[string]any, error) {
	raw, err := s.configs.Get()
	if err != nil {
		return nil, err
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("system config is not a JSON object: %w", err)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return cfg, nil
}

// saveDraft marshals and persists the draft map.
func (s *SubscriptionService) saveDraft(cfg map[string]any) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.configs.Update(data)
}

// draftProviders returns the mutable proxy-providers map (creating it if absent).
func draftProviders(cfg map[string]any) map[string]any {
	provs, _ := cfg["proxy-providers"].(map[string]any)
	if provs == nil {
		provs = map[string]any{}
		cfg["proxy-providers"] = provs
	}
	return provs
}

// findProviderEntry locates a provider entry in the current draft.
func (s *SubscriptionService) findProviderEntry(name string) (providerEntry, error) {
	cfg, err := s.draftConfig()
	if err != nil {
		return providerEntry{}, err
	}
	provs := draftProviders(cfg)
	raw, ok := provs[name].(map[string]any)
	if !ok {
		return providerEntry{}, fmt.Errorf("provider %q not found in draft", name)
	}
	return parseProviderEntry(name, raw), nil
}

// upsertDraftProvider writes/updates the native provider entry for a
// subscription. Only the provider entry is touched — never vendor rules,
// DNS, or global settings.
func (s *SubscriptionService) upsertDraftProvider(name, rawURL string) (bool, error) {
	cfg, err := s.draftConfig()
	if err != nil {
		return false, err
	}
	provs := draftProviders(cfg)
	changed := false
	existing, ok := provs[name].(map[string]any)
	if !ok {
		provs[name] = map[string]any{
			"type":     "http",
			"url":      rawURL,
			"path":     fmt.Sprintf("providers/proxies/%s.yaml", name),
			"interval": 86400,
		}
		changed = true
	} else if rawURL != "" && existing["url"] != rawURL {
		existing["url"] = rawURL
		changed = true
	}
	if !changed {
		return false, nil
	}
	if err := s.saveDraft(cfg); err != nil {
		return false, err
	}
	return true, nil
}

// removeDraftProvider deletes the provider entry from the draft.
func (s *SubscriptionService) removeDraftProvider(name string) (bool, error) {
	cfg, err := s.draftConfig()
	if err != nil {
		return false, err
	}
	provs := draftProviders(cfg)
	if _, ok := provs[name]; !ok {
		return false, nil
	}
	delete(provs, name)
	if err := s.saveDraft(cfg); err != nil {
		return false, err
	}
	return true, nil
}

// groupsUsingProvider lists proxy-groups whose `use` references the provider.
func (s *SubscriptionService) groupsUsingProvider(name string) ([]string, error) {
	groups, err := s.groups.List()
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, g := range groups {
		var cfg map[string]any
		_ = json.Unmarshal(g.Config, &cfg)
		useList, _ := cfg["use"].([]any)
		for _, u := range useList {
			if us, ok := u.(string); ok && us == name {
				refs = append(refs, g.Name)
				break
			}
		}
	}
	sort.Strings(refs)
	return refs, nil
}

// ── CRUD ─────────────────────────────────────────────────────────────────────

// SubscriptionRequest is the create/update payload. All fields are pointers:
// nil means "not provided" on update and keeps the stored value; an empty
// url keeps the stored secret.
type SubscriptionRequest struct {
	Name              *string `json:"name"`
	URL               *string `json:"url"`
	ProviderName      *string `json:"provider_name"`
	QuotaBytes        *int64  `json:"quota_bytes"`
	ManualExpire      *string `json:"manual_expire"` // RFC3339
	ResetDay          *int    `json:"reset_day"`
	ResetHour         *int    `json:"reset_hour"`
	ResetMinute       *int    `json:"reset_minute"`
	Timezone          *string `json:"timezone"`
	TrafficProviderID *string `json:"traffic_provider_id"`
	Enabled           *bool   `json:"enabled"`
}

func (r *SubscriptionRequest) applyTo(sub *model.Subscription) error {
	if r.Name != nil {
		sub.Name = strings.TrimSpace(*r.Name)
	}
	if r.QuotaBytes != nil {
		if *r.QuotaBytes < 0 {
			return fmt.Errorf("quota_bytes must be >= 0")
		}
		sub.QuotaBytes = *r.QuotaBytes
	}
	if r.ManualExpire != nil {
		if strings.TrimSpace(*r.ManualExpire) == "" {
			sub.ManualExpire = 0
		} else {
			t, err := time.Parse(time.RFC3339, strings.TrimSpace(*r.ManualExpire))
			if err != nil {
				return fmt.Errorf("manual_expire must be RFC3339: %w", err)
			}
			sub.ManualExpire = t.Unix()
		}
	}
	if r.ResetDay != nil {
		sub.ResetDay = *r.ResetDay
	}
	if r.ResetHour != nil {
		sub.ResetHour = *r.ResetHour
	}
	if r.ResetMinute != nil {
		sub.ResetMinute = *r.ResetMinute
	}
	if r.Timezone != nil {
		sub.ResetTimezone = strings.TrimSpace(*r.Timezone)
	}
	if r.TrafficProviderID != nil {
		sub.TrafficProviderID = strings.TrimSpace(*r.TrafficProviderID)
	}
	if r.Enabled != nil {
		sub.Enabled = *r.Enabled
	}
	if r.ProviderName != nil {
		sub.ProviderName = strings.TrimSpace(*r.ProviderName)
	}
	return nil
}

func validateReset(sub *model.Subscription) error {
	if sub.ResetDay == 0 {
		if sub.ResetHour != 0 || sub.ResetMinute != 0 {
			return fmt.Errorf("reset_hour/reset_minute require reset_day")
		}
		return nil
	}
	_, err := (ResetPolicy{Day: sub.ResetDay, Hour: sub.ResetHour, Minute: sub.ResetMinute, Timezone: sub.ResetTimezone}).Normalize()
	return err
}

// Create creates the metadata row and, when needed, the native provider
// entry in the draft. Existing providers are linked, never duplicated. A
// custom provider_name different from the subscription name is honored.
func (s *SubscriptionService) Create(req *SubscriptionRequest) (*SubscriptionView, bool, error) {
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		return nil, false, fmt.Errorf("name is required")
	}
	sub := &model.Subscription{
		ID:             uuid.New().String(),
		Enabled:        true,
		ProviderSource: "created",
	}
	if err := req.applyTo(sub); err != nil {
		return nil, false, err
	}
	if sub.ProviderName == "" {
		sub.ProviderName = sub.Name
	}
	if err := validateReset(sub); err != nil {
		return nil, false, err
	}
	if _, err := s.subs.GetByName(sub.Name); err == nil {
		return nil, false, &ErrConflict{fmt.Sprintf("subscription %q already exists", sub.Name)}
	}
	if other, err := s.subs.GetByProvider(sub.ProviderName); err == nil && other.ID != sub.ID {
		return nil, false, &ErrConflict{fmt.Sprintf("provider %q is already managed by subscription %q; a provider cannot be shared", sub.ProviderName, other.Name)}
	}

	draftChanged := false
	if _, exists := s.findProviderEntry(sub.ProviderName); exists == nil {
		// Linking an existing native provider — do not touch its config.
		sub.ProviderSource = "linked"
		if req.URL != nil && strings.TrimSpace(*req.URL) != "" {
			return nil, false, fmt.Errorf("provider %q already exists; omit url to link it", sub.ProviderName)
		}
	} else {
		if req.URL == nil || strings.TrimSpace(*req.URL) == "" {
			return nil, false, fmt.Errorf("url is required when creating a new subscription provider")
		}
		changed, err := s.upsertDraftProvider(sub.ProviderName, strings.TrimSpace(*req.URL))
		if err != nil {
			return nil, false, err
		}
		draftChanged = draftChanged || changed
	}

	if sub.TrafficProviderID != "" {
		if _, err := s.traffic.GetProvider(sub.TrafficProviderID); err != nil {
			return nil, false, fmt.Errorf("traffic provider %q not found", sub.TrafficProviderID)
		}
	} else if entry, err := s.findProviderEntry(sub.ProviderName); err == nil && entry.URL != "" {
		// Auto-link a traffic provider that tracks the same subscription URL.
		if tp, err := s.traffic.GetProvider(s.trafficProviderIDByURL(entry.URL)); err == nil {
			sub.TrafficProviderID = tp.ID
		}
	}

	if err := s.subs.Create(sub); err != nil {
		return nil, draftChanged, err
	}
	if s.scorer != nil {
		s.scorer.InvalidateProviderCache()
		s.scorer.ScanAndEnqueue()
		s.scorer.Wake()
	}
	v, err := s.View(sub)
	return v, draftChanged, err
}

// trafficProviderIDByURL finds the traffic provider tracking a subscription URL.
func (s *SubscriptionService) trafficProviderIDByURL(rawURL string) string {
	providers, err := s.traffic.ListProviders()
	if err != nil {
		return ""
	}
	for _, p := range providers {
		if p.SubscriptionURL == rawURL {
			return p.ID
		}
	}
	return ""
}

// Update mutates metadata; nil fields keep stored values, an empty url keeps
// the stored secret. Provider rebinding requires the new provider to exist.
func (s *SubscriptionService) Update(id string, req *SubscriptionRequest) (*SubscriptionView, bool, error) {
	sub, err := s.subs.Get(id)
	if err != nil {
		return nil, false, err
	}
	oldName := sub.Name
	oldProvider := sub.ProviderName
	if err := req.applyTo(sub); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(sub.Name) == "" {
		return nil, false, fmt.Errorf("name is required")
	}
	if err := validateReset(sub); err != nil {
		return nil, false, err
	}
	if sub.Name != oldName {
		if _, err := s.subs.GetByName(sub.Name); err == nil {
			return nil, false, &ErrConflict{fmt.Sprintf("subscription %q already exists", sub.Name)}
		}
	}
	if other, err := s.subs.GetByProvider(sub.ProviderName); err == nil && other.ID != sub.ID {
		return nil, false, &ErrConflict{fmt.Sprintf("provider %q is already managed by subscription %q; a provider cannot be shared", sub.ProviderName, other.Name)}
	}
	if sub.ProviderName != oldProvider {
		if _, err := s.findProviderEntry(sub.ProviderName); err != nil {
			return nil, false, fmt.Errorf("provider %q not found in draft", sub.ProviderName)
		}
		sub.ProviderSource = "linked"
	}

	draftChanged := false
	if req.URL != nil && strings.TrimSpace(*req.URL) != "" && sub.ProviderName != "" {
		changed, err := s.upsertDraftProvider(sub.ProviderName, strings.TrimSpace(*req.URL))
		if err != nil {
			return nil, false, err
		}
		draftChanged = draftChanged || changed
	}

	if sub.TrafficProviderID != "" {
		if _, err := s.traffic.GetProvider(sub.TrafficProviderID); err != nil {
			return nil, false, fmt.Errorf("traffic provider %q not found", sub.TrafficProviderID)
		}
	}

	if err := s.subs.Update(sub); err != nil {
		return nil, draftChanged, err
	}
	v, err := s.View(sub)
	return v, draftChanged, err
}

// Delete implements the real delete semantics: referenced providers are
// refused with their references; otherwise the native provider entry is
// removed from the draft, the metadata row and its score rows are deleted.
// Shared traffic-provider accounting is not touched. Business groups are
// never rewritten here.
func (s *SubscriptionService) Delete(id string) (map[string]any, error) {
	sub, err := s.subs.Get(id)
	if err != nil {
		return nil, err
	}
	if sub.ProviderName != "" {
		refs, err := s.groupsUsingProvider(sub.ProviderName)
		if err != nil {
			return nil, err
		}
		if len(refs) > 0 {
			return nil, &ErrReferenced{References: refs}
		}
	}
	removedFromDraft := false
	if sub.ProviderName != "" {
		removedFromDraft, err = s.removeDraftProvider(sub.ProviderName)
		if err != nil {
			return nil, err
		}
	}
	if sub.ProviderName != "" {
		if err := s.scores.DeleteByProvider(sub.ProviderName); err != nil {
			return nil, err
		}
	}
	if err := s.subs.Delete(sub.ID); err != nil {
		return nil, err
	}
	if s.scorer != nil {
		s.scorer.InvalidateProviderCache()
	}
	return map[string]any{
		"status":                      "deleted",
		"provider_name":               sub.ProviderName,
		"provider_removed_from_draft": removedFromDraft,
		"draft_changed":               removedFromDraft,
	}, nil
}

// ── refresh ──────────────────────────────────────────────────────────────────

// RefreshError carries a machine-readable code and sanitized message.
type RefreshError struct {
	Code             string `json:"code"`
	Message          string `json:"message"`
	RetryAfterSecond int    `json:"retry_after_seconds,omitempty"`
}

func (e *RefreshError) Error() string { return e.Message }

// Refresh re-fetches vendor userinfo, asks the running mihomo to refresh the
// provider, and re-enqueues discovered nodes. It never publishes and never
// touches mode/selector state.
// Refresh re-fetches vendor userinfo, asks the running mihomo to refresh the
// provider, and re-enqueues discovered nodes. It never publishes and never
// touches mode/selector state.
//
// Concurrency: the network fetch happens against a snapshot; afterwards the
// row is RE-READ and only snapshot/status fields are persisted, so concurrent
// name/enabled/reset edits are never clobbered. If the provider or its URL
// changed while fetching, the stale-source result is discarded.
func (s *SubscriptionService) Refresh(id string) (*SubscriptionView, *RefreshError) {
	sub, err := s.subs.Get(id)
	if err != nil {
		return nil, &RefreshError{Code: "not_found", Message: "subscription not found"}
	}
	now := s.nowFn()

	entry, entryErr := s.findProviderEntry(sub.ProviderName)
	fetchURL := entry.URL
	var info *model.TrafficUserinfo
	var refErr *RefreshError

	if entryErr == nil && entry.URL != "" {
		fetched, ferr := s.fetchUserinfo(entry.URL)
		switch {
		case ferr == nil:
			info = fetched
		default:
			var rl *RateLimitError
			if errors.As(ferr, &rl) {
				secs := int(respectRetryAfter(rl.RetryAfter).Seconds())
				refErr = &RefreshError{Code: "rate_limited", Message: "subscription server rate limited; vendor snapshot kept", RetryAfterSecond: secs}
			} else {
				refErr = &RefreshError{Code: "userinfo_failed", Message: "userinfo fetch failed: " + sanitizeMessage(ferr)}
			}
		}
	} else {
		refErr = &RefreshError{Code: "userinfo_failed", Message: "provider entry has no subscription URL to refresh"}
	}

	// Best-effort runtime provider refresh (downloads new node list).
	if entryErr == nil && s.mihomo != nil {
		if perr := s.mihomo.RefreshProvider(sub.ProviderName); perr != nil {
			msg := "provider refresh failed: " + sanitizeMessage(perr)
			if refErr == nil {
				refErr = &RefreshError{Code: "provider_refresh_failed", Message: msg}
			} else {
				refErr.Message += "; " + msg
			}
		}
	}

	// Re-read the row: concurrent metadata edits must survive this refresh.
	fresh, rerr := s.subs.Get(id)
	if rerr != nil {
		return nil, &RefreshError{Code: "internal", Message: rerr.Error()}
	}
	fresh.LastRefreshAt = model.NewNullTime(now)

	if info != nil {
		// Stale-source guard: only apply results fetched from the URL that is
		// still current for the row's (possibly renamed) provider.
		stale := true
		if cur, cerr := s.findProviderEntry(fresh.ProviderName); cerr == nil && cur.URL == fetchURL {
			stale = false
		}
		if stale {
			if refErr == nil {
				refErr = &RefreshError{Code: "userinfo_failed", Message: "provider or URL changed during refresh; fetched snapshot discarded"}
			}
			fresh.LastRefreshError = refErr.Message
		} else {
			fresh.UserinfoUpload = info.Upload
			fresh.UserinfoDownload = info.Download
			fresh.UserinfoTotal = info.Total
			fresh.UserinfoExpire = info.Expire
			fresh.UserinfoCountersSeen = info.HasCounters
			fresh.UserinfoFetchedAt = model.NewNullTime(now)
		}
	}

	if refErr == nil {
		if info != nil {
			fresh.LastSuccessAt = model.NewNullTime(now)
		}
		fresh.LastRefreshError = ""
	} else {
		fresh.LastRefreshError = refErr.Message
	}
	if err := s.subs.Update(fresh); err != nil {
		return nil, &RefreshError{Code: "internal", Message: err.Error()}
	}

	if s.scorer != nil {
		s.scorer.InvalidateProviderCache()
		s.scorer.ScanAndEnqueue()
		s.scorer.Wake()
	}

	v, verr := s.View(fresh)
	if verr != nil {
		return nil, &RefreshError{Code: "internal", Message: verr.Error()}
	}
	return v, refErr
}
func (s *SubscriptionService) fetchUserinfo(rawURL string) (*model.TrafficUserinfo, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid subscription URL")
	}
	// Vendors gate configs on a clash UA — some return nothing to others.
	req.Header.Set("User-Agent", "clash.meta")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, sanitizeURLError("subscription", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("subscription returned HTTP %d", resp.StatusCode)
	}
	header := resp.Header.Get("subscription-userinfo")
	if header == "" {
		return nil, fmt.Errorf("no subscription-userinfo header")
	}
	return ParseUserinfo(header)
}

// ── discovery ────────────────────────────────────────────────────────────────

// DiscoveredProviderView matches the contract's /api/subscriptions/providers row.
type DiscoveredProviderView struct {
	Name                 string `json:"name"`
	Type                 string `json:"type"`
	NodeCount            *int   `json:"node_count"`
	LinkedSubscriptionID string `json:"linked_subscription_id"` // "" when unlinked
	Linked               bool   `json:"linked"`
}

// DiscoverProviders lists native proxy-providers in the draft with their link
// state, so the GUI can adopt existing entries instead of duplicating them.
func (s *SubscriptionService) DiscoverProviders() ([]DiscoveredProviderView, error) {
	cfg, err := s.draftConfig()
	if err != nil {
		return nil, err
	}
	provs := draftProviders(cfg)
	names := make([]string, 0, len(provs))
	for name := range provs {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]DiscoveredProviderView, 0, len(names))
	for _, name := range names {
		raw, _ := provs[name].(map[string]any)
		e := parseProviderEntry(name, raw)
		v := DiscoveredProviderView{Name: name, Type: e.Type}
		if n, err := s.providerNodeCount(e); err == nil {
			v.NodeCount = &n
		}
		if sub, err := s.subs.GetByProvider(name); err == nil {
			v.Linked = true
			v.LinkedSubscriptionID = sub.ID
		}
		out = append(out, v)
	}
	return out, nil
}
