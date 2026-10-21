package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
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
//
// Two source types are supported:
//   - "native": linked to (or creating) a proxy-providers entry in the draft.
//     Create/update/delete/refresh may mutate the DRAFT provider entry.
//   - "traffic": linked to an existing traffic provider row (static/legacy
//     source). Every mutation is metadata-only: the draft config, native
//     provider refresh and static proxies/groups are never touched, and
//     draft_changed is always false.
type SubscriptionService struct {
	subs     *store.SubscriptionStore
	configs  *store.ConfigStore
	traffic  *store.TrafficStore
	scores   *store.NodeScoreStore
	groups   *store.ProxyGroupStore
	proxies  *store.ProxyStore
	mihomo   *MihomoClient
	settings *store.SettingsStore
	scorer   *ScoringService
	http     *http.Client

	// mutateMu serializes config+metadata mutations (Create/Update/Delete) so
	// a draft read-modify-write can never lose another mutation's entry and a
	// losing create cannot overwrite a winner's provider URL before the DB
	// unique index rejects it. Validations run under the lock before any
	// mutation; a DB failure rolls the draft side effect back.
	mutateMu sync.Mutex

	nowFn func() time.Time
}

// NewSubscriptionService creates the service. scorer may be nil in tests.
func NewSubscriptionService(
	subs *store.SubscriptionStore,
	configs *store.ConfigStore,
	traffic *store.TrafficStore,
	scores *store.NodeScoreStore,
	groups *store.ProxyGroupStore,
	proxies *store.ProxyStore,
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
		proxies:  proxies,
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

// autoRefreshUserinfo refreshes vendor snapshots of enabled subscriptions:
// native subs with a provider entry URL, and traffic-source subs whose linked
// traffic provider has a subscription URL. Disabled subscriptions are paused.
func (s *SubscriptionService) autoRefreshUserinfo() {
	subs, err := s.subs.List()
	if err != nil {
		return
	}
	for i := range subs {
		sub := &subs[i]
		if !sub.Enabled {
			continue
		}
		if sub.SourceType == model.SubscriptionSourceTraffic {
			if tp, err := s.traffic.GetProvider(sub.TrafficProviderID); err == nil && tp.SubscriptionURL != "" {
				s.autoRefreshTrafficSource(sub, tp)
			}
			continue
		}
		if sub.ProviderName == "" {
			continue
		}
		entry, err := s.findProviderEntry(sub.ProviderName)
		if err != nil || entry.URL == "" {
			continue
		}
		fetchURL := entry.URL
		// Network fetch OUTSIDE the mutation lock.
		info, err := s.fetchUserinfo(fetchURL)
		if err != nil {
			slog.Debug("subscription: auto userinfo refresh failed", "subscription", sub.Name, "err", err)
			continue // keep last success; do not mark error for background polls
		}
		// Short critical section: source re-validation (atomic against
		// provider/URL rebind) + REAL field-specific snapshot persist.
		s.mutateMu.Lock()
		fresh, gerr := s.subs.Get(sub.ID)
		if gerr != nil {
			s.mutateMu.Unlock()
			continue
		}
		cur, cerr := s.findProviderEntry(fresh.ProviderName)
		if cerr != nil || cur.URL != fetchURL {
			s.mutateMu.Unlock()
			continue // URL/provider rebound mid-flight: drop the stale-source result
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
		if err := s.subs.UpdateSnapshot(fresh); err != nil {
			slog.Warn("subscription: auto userinfo persist failed", "subscription", sub.Name, "err", err)
		}
		s.mutateMu.Unlock()
	}
}

// autoRefreshTrafficSource fetches the traffic provider's subscription URL
// (shared with the legacy traffic polling) and persists the vendor snapshot
// onto the subscription row only — the traffic provider row, its cache and
// the draft are never touched. Source re-validation guards rebinds.
func (s *SubscriptionService) autoRefreshTrafficSource(sub *model.Subscription, tp *model.TrafficProvider) {
	fetchURL := tp.SubscriptionURL
	fetchTPID := tp.ID // provider identity AND URL must both survive the fetch
	snap, err := FetchVendorInfo(s.http, fetchURL) // outside the mutation lock
	if err != nil {
		slog.Debug("subscription: auto traffic-source refresh failed", "subscription", sub.Name, "err", err)
		return // keep last good; background polls never set the error flag
	}
	s.mutateMu.Lock()
	defer s.mutateMu.Unlock()
	fresh, gerr := s.subs.Get(sub.ID)
	if gerr != nil {
		return
	}
	cur, cerr := s.traffic.GetProvider(fresh.TrafficProviderID)
	if cerr != nil || fresh.TrafficProviderID != fetchTPID || cur.SubscriptionURL != fetchURL {
		return // rebound mid-flight: even an identical URL is a different source — drop
	}
	now := s.nowFn()
	applyVendorSnapshot(fresh, snap, now)
	fresh.LastSuccessAt = model.NewNullTime(now)
	fresh.LastRefreshError = ""
	if err := s.subs.UpdateSnapshot(fresh); err != nil {
		slog.Warn("subscription: auto traffic-source persist failed", "subscription", sub.Name, "err", err)
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
	ProviderSource string `json:"provider_source"` // created | linked (native only; empty for traffic sources)
	ProviderExists bool   `json:"provider_exists"`
	URLMasked      string `json:"url_masked"`
	NodeCount      *int   `json:"node_count"`

	// SourceType: "native" (proxy-providers entry) or "traffic" (existing
	// traffic provider; no native provider exists and that is NOT an error).
	SourceType string `json:"source_type"`
	// TrafficProviderName is the linked traffic provider's display name
	// (traffic sources only; empty otherwise).
	TrafficProviderName string `json:"traffic_provider_name"`
	// SupplierResetDay is the vendor-OBSERVED reset day-of-month from an
	// aggregate JSON payload. Informational only — the configured schedule
	// below stays authoritative and no hour/timezone is implied.
	SupplierResetDay *int `json:"supplier_reset_day"`

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
		SourceType:        sub.SourceType,
		TrafficProviderID: sub.TrafficProviderID,
		CreatedAt:         sub.CreatedAt,
		UpdatedAt:         sub.UpdatedAt,
		UsageSource:       "none",
		ExpiresSource:     "unknown",
	}
	if v.SourceType == "" {
		v.SourceType = model.SubscriptionSourceNative
	}

	// Traffic-source link. A missing native provider entry is normal for
	// this type; a missing traffic provider row means the source itself is
	// gone (surfaced as an error below).
	var tp *model.TrafficProvider
	if sub.SourceType == model.SubscriptionSourceTraffic && sub.TrafficProviderID != "" {
		if got, err := s.traffic.GetProvider(sub.TrafficProviderID); err == nil {
			tp = got
		}
		v.TrafficProviderName = tpName(tp)
		if sub.VendorResetDay >= 1 && sub.VendorResetDay <= 31 {
			day := sub.VendorResetDay
			v.SupplierResetDay = &day
		}
		v.URLMasked = MaskURL(tpSubscriptionURL(tp))
	}

	// Status. disabled means "automatic metadata refresh & auto scoring
	// paused" — the provider stays in the config and can still carry traffic.
	// For traffic sources: a vanished traffic provider row is an error; a
	// source without a subscription URL is healthy local-only data (never
	// "stale" — there is nothing vendor-side to fetch).
	if !sub.Enabled {
		v.Status = "disabled"
	} else if sub.SourceType == model.SubscriptionSourceTraffic && sub.TrafficProviderID != "" && tp == nil {
		v.Status = "error"
		v.LastError = "linked traffic provider no longer exists"
	} else if sub.SourceType == model.SubscriptionSourceTraffic && tpSubscriptionURL(tp) == "" && sub.LastRefreshError == "" {
		v.Status = "local"
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
		if !sub.UserinfoIsAggregate {
			// Aggregate snapshots carry no per-direction counters; projecting
			// them as 0/0 would fabricate directions that the vendor never
			// reported. The aggregate used value is exposed via used_bytes.
			v.Userinfo = &model.TrafficUserinfo{
				ProviderID: sub.ID,
				Upload:     sub.UserinfoUpload,
				Download:   sub.UserinfoDownload,
				Total:      sub.UserinfoTotal,
				Expire:     sub.UserinfoExpire,
				FetchedAt:  sub.UserinfoFetchedAt.Time,
			}
		}
	}

	// Quota & usage. Vendor usage is "known" only when the header carried
	// both counters or the payload was an aggregate counter; a total/expire-
	// only header must not become used=0. Explicit upload=0;download=0 and
	// an explicit aggregate 0 stay genuine zeros.
	vendorKnown := !sub.UserinfoFetchedAt.IsZero() && sub.UserinfoCountersSeen
	if vendorKnown {
		used := sub.UserinfoUpload + sub.UserinfoDownload
		v.UsedBytes = &used
		v.UsageSource = "vendor"
	} else if !sub.UserinfoFetchedAt.IsZero() && sub.UserinfoIsAggregate {
		used := sub.UserinfoAggregateUsed
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

	// Traffic-source supplements: when the subscription has no vendor
	// snapshot of its own yet, fall back to the traffic provider's cached
	// vendor data (fetched by legacy traffic polling) for quota/expiry —
	// never for counters, which the cache cannot distinguish from absent.
	// Recompute remaining if the fallback added a quota side.
	if sub.SourceType == model.SubscriptionSourceTraffic && tp != nil && sub.UserinfoFetchedAt.IsZero() {
		if cached, err := s.traffic.GetUserinfo(tp.ID); err == nil {
			if quota == nil && cached.Total > 0 {
				t := cached.Total
				quota = &t
				v.QuotaBytes = quota
			}
			if v.ExpiresAt == nil && cached.Expire > 0 {
				v.ExpiresAt = rfc3339Time(time.Unix(cached.Expire, 0).UTC())
				v.ExpiresSource = "vendor"
			}
			if quota != nil && v.UsedBytes != nil && v.RemainingBytes == nil {
				rem := *quota - *v.UsedBytes
				if rem < 0 {
					rem = 0
				}
				v.RemainingBytes = &rem
			}
		}
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
	if sub.SourceType == model.SubscriptionSourceTraffic {
		// Traffic source: no native provider exists (that is normal, not an
		// error) and the URL comes from the linked traffic provider (set
		// above). Node count = actual static proxy names matched by the
		// provider's node patterns — flat matching, never group/fallback
		// traversal.
		v.ProviderExists = false
		v.ProviderSource = ""
		v.ProviderName = ""
		if tp != nil {
			v.NodeCount = s.staticMatchedProxyCount(tp)
		}
		return v, nil
	}
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

// staticMatchedProxyCount counts distinct static proxy names in the draft
// that match any of the traffic provider's node patterns. Flat name matching
// only — group members and nested fallbacks are never counted.
func (s *SubscriptionService) staticMatchedProxyCount(tp *model.TrafficProvider) *int {
	if tp == nil || len(tp.NodePatternList) == 0 {
		n := 0
		return &n
	}
	proxies, err := s.proxies.List("", "")
	if err != nil {
		return nil // unknown
	}
	n := 0
	for _, p := range proxies {
		for _, pat := range tp.NodePatternList {
			if ok, err := path.Match(pat, p.Name); err == nil && ok {
				n++
				break
			}
		}
	}
	return &n
}

// tpName safely resolves a traffic provider display name (nil-safe).
func tpName(tp *model.TrafficProvider) string {
	if tp == nil {
		return ""
	}
	return tp.Name
}

// tpSubscriptionURL safely resolves a traffic provider's subscription URL
// (nil-safe; the caller treats an empty URL as "no vendor link").
func tpSubscriptionURL(tp *model.TrafficProvider) string {
	if tp == nil {
		return ""
	}
	return tp.SubscriptionURL
}

// applyVendorSnapshot writes a fetched VendorSnapshot onto a subscription
// row. Header-format snapshots clear the aggregate fields and vice versa —
// the latest fetch decides the snapshot mode. Aggregate payloads always
// carry a valid counter (FetchVendorInfo guarantees it); the nil-counter
// guard only prevents an invented zero if that invariant were ever broken.
func applyVendorSnapshot(sub *model.Subscription, snap *VendorSnapshot, now time.Time) {
	sub.UserinfoFetchedAt = model.NewNullTime(now)
	if snap.Info == nil {
		if snap.AggregateUsed == nil {
			// Defensive: never claim a counter that was not reported.
			sub.UserinfoIsAggregate = false
			sub.UserinfoAggregateUsed = 0
			sub.UserinfoUpload, sub.UserinfoDownload = 0, 0
			sub.UserinfoCountersSeen = false
			sub.UserinfoExpire = 0
			if snap.AggregateLimit != nil {
				sub.UserinfoTotal = *snap.AggregateLimit
			} else {
				sub.UserinfoTotal = 0
			}
			sub.VendorResetDay = 0
			return
		}
		sub.UserinfoUpload, sub.UserinfoDownload = 0, 0
		sub.UserinfoCountersSeen = false
		sub.UserinfoExpire = 0
		sub.UserinfoIsAggregate = true
		sub.UserinfoAggregateUsed = *snap.AggregateUsed // genuine zero preserved
		if snap.AggregateLimit != nil {
			sub.UserinfoTotal = *snap.AggregateLimit
		} else {
			sub.UserinfoTotal = 0 // unknown, never a zero quota
		}
		if snap.VendorResetDay != nil {
			sub.VendorResetDay = *snap.VendorResetDay
		} else {
			sub.VendorResetDay = 0
		}
		return
	}
	info := snap.Info
	sub.UserinfoUpload = info.Upload
	sub.UserinfoDownload = info.Download
	sub.UserinfoTotal = info.Total
	sub.UserinfoExpire = info.Expire
	sub.UserinfoCountersSeen = info.HasCounters
	sub.UserinfoIsAggregate = false
	sub.UserinfoAggregateUsed = 0
	sub.VendorResetDay = 0
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

// draftEntrySnapshot captures one provider entry so a failed DB write can
// restore the draft exactly (entry present with its original JSON, or absent).
type draftEntrySnapshot struct {
	existed bool
	raw     map[string]any
}

func (s *SubscriptionService) snapshotDraftProvider(name string) draftEntrySnapshot {
	cfg, err := s.draftConfig()
	if err != nil {
		return draftEntrySnapshot{}
	}
	raw, ok := draftProviders(cfg)[name].(map[string]any)
	if !ok {
		return draftEntrySnapshot{}
	}
	return draftEntrySnapshot{existed: true, raw: raw}
}

// restoreDraftProvider returns the entry to its pre-mutation state. Unrelated
// settings and entries are untouched.
func (s *SubscriptionService) restoreDraftProvider(name string, snap draftEntrySnapshot) error {
	if !snap.existed {
		_, err := s.removeDraftProvider(name)
		return err
	}
	cfg, err := s.draftConfig()
	if err != nil {
		return err
	}
	draftProviders(cfg)[name] = snap.raw
	return s.saveDraft(cfg)
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
// url keeps the stored secret. source_type selects "native" (default) or
// "traffic" (existing traffic provider; create-only — a stored source type
// never changes via update).
type SubscriptionRequest struct {
	Name              *string `json:"name"`
	URL               *string `json:"url"`
	ProviderName      *string `json:"provider_name"`
	SourceType        *string `json:"source_type"`
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

// validateSourceType trims and validates a requested source type. An absent
// value defaults to "native" (existing behavior).

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
	// Serialize config+metadata mutations: one read-modify-write of the draft
	// and one metadata insert, atomically enough that a losing create can
	// neither clobber a winner's URL nor leave draft side effects behind.
	s.mutateMu.Lock()
	defer s.mutateMu.Unlock()

	// ── validation, all before any mutation ──
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		return nil, false, fmt.Errorf("name is required")
	}
	sub := &model.Subscription{
		ID:             uuid.New().String(),
		Enabled:        true,
		ProviderSource: "created",
		SourceType:     model.SubscriptionSourceNative,
	}
	if err := req.applyTo(sub); err != nil {
		return nil, false, err
	}
	if req.SourceType != nil {
		switch st := strings.TrimSpace(*req.SourceType); st {
		case "":
			sub.SourceType = model.SubscriptionSourceNative
		case model.SubscriptionSourceNative, model.SubscriptionSourceTraffic:
			sub.SourceType = st
		default:
			return nil, false, fmt.Errorf("source_type must be %q or %q", model.SubscriptionSourceNative, model.SubscriptionSourceTraffic)
		}
	}
	if sub.SourceType == model.SubscriptionSourceTraffic {
		return s.createTrafficSource(sub, req)
	}
	if sub.ProviderName == "" {
		sub.ProviderName = sub.Name
	}
	if err := validateReset(sub); err != nil {
		return nil, false, err
	}
	if sub.TrafficProviderID != "" {
		if _, err := s.traffic.GetProvider(sub.TrafficProviderID); err != nil {
			return nil, false, fmt.Errorf("traffic provider %q not found", sub.TrafficProviderID)
		}
	}
	if _, err := s.subs.GetByName(sub.Name); err == nil {
		return nil, false, &ErrConflict{fmt.Sprintf("subscription %q already exists", sub.Name)}
	}
	if other, err := s.subs.GetByProvider(sub.ProviderName); err == nil && other.ID != sub.ID {
		return nil, false, &ErrConflict{fmt.Sprintf("provider %q is already managed by subscription %q; a provider cannot be shared", sub.ProviderName, other.Name)}
	}

	var entryURL string
	if entry, err := s.findProviderEntry(sub.ProviderName); err == nil {
		entryURL = entry.URL
		if req.URL != nil && strings.TrimSpace(*req.URL) != "" {
			return nil, false, &ErrConflict{fmt.Sprintf("provider %q is already managed or configured; omit url to link it, or pick another name", sub.ProviderName)}
		}
		sub.ProviderSource = "linked"
		if sub.TrafficProviderID == "" && entryURL != "" {
			// Auto-link a traffic provider that tracks the same subscription URL.
			if tp, err := s.traffic.GetProvider(s.trafficProviderIDByURL(entryURL)); err == nil {
				sub.TrafficProviderID = tp.ID
			}
		}
	} else {
		if req.URL == nil || strings.TrimSpace(*req.URL) == "" {
			return nil, false, fmt.Errorf("url is required when creating a new subscription provider")
		}
	}

	// ── mutation: draft first (rollback-able), then metadata ──
	draftChanged := false
	var snap draftEntrySnapshot
	if sub.ProviderSource == "created" {
		snap = s.snapshotDraftProvider(sub.ProviderName)
		changed, err := s.upsertDraftProvider(sub.ProviderName, strings.TrimSpace(*req.URL))
		if err != nil {
			return nil, false, err
		}
		draftChanged = changed
	}

	if err := s.subs.Create(sub); err != nil {
		if draftChanged {
			if rerr := s.restoreDraftProvider(sub.ProviderName, snap); rerr != nil {
				slog.Error("subscription: draft rollback after failed create failed", "provider", sub.ProviderName, "err", rerr)
			}
		}
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

// createTrafficSource creates a subscription that tracks an EXISTING traffic
// provider (static/legacy source). It never creates a native proxy-provider
// entry: the draft config is untouched (draft_changed is always false), the
// native provider name stays blank, and the traffic provider's own
// subscription URL remains the only URL source — no duplicate secret storage
// here. Quota and the reset schedule are inherited from the selected traffic
// provider unless the request explicitly overrides them; an unknown/empty
// provider timezone inherits as empty (server-local), never fabricated.
// Caller holds s.mutateMu.
func (s *SubscriptionService) createTrafficSource(sub *model.Subscription, req *SubscriptionRequest) (*SubscriptionView, bool, error) {
	if sub.ProviderName != "" {
		return nil, false, fmt.Errorf("provider_name must be empty for a traffic-source subscription; the node source is owned by the traffic provider")
	}
	if req.URL != nil && strings.TrimSpace(*req.URL) != "" {
		return nil, false, fmt.Errorf("url is owned by the linked traffic provider; edit it there instead of storing a second copy")
	}
	if sub.TrafficProviderID == "" {
		return nil, false, fmt.Errorf("traffic_provider_id is required for a traffic-source subscription")
	}
	tp, err := s.traffic.GetProvider(sub.TrafficProviderID)
	if err != nil {
		return nil, false, fmt.Errorf("traffic provider %q not found", sub.TrafficProviderID)
	}

	// Inheritance happens per omitted field so a partial override survives:
	// an explicit timezone-only / hour-only / minute-only / day-only / quota-
	// only request keeps its value and inherits the rest. Explicit zero/empty
	// values are preserved (quota 0 = unknown, empty timezone = server-local,
	// hour 0 stays 0). The final combined policy is validated once; explicit
	// invalid values error out instead of being replaced by inherited ones.
	if req.QuotaBytes != nil {
		sub.QuotaBytes = *req.QuotaBytes
	} else {
		sub.QuotaBytes = tp.QuotaBytes
	}
	if req.ResetDay != nil {
		sub.ResetDay = *req.ResetDay
	} else {
		sub.ResetDay = tp.ResetDay
	}
	if req.ResetHour != nil {
		sub.ResetHour = *req.ResetHour
	} else {
		sub.ResetHour = tp.ResetHour
	}
	if req.ResetMinute != nil {
		sub.ResetMinute = *req.ResetMinute
	} else {
		sub.ResetMinute = tp.ResetMinute
	}
	if req.Timezone != nil {
		sub.ResetTimezone = strings.TrimSpace(*req.Timezone)
	} else {
		sub.ResetTimezone = tp.ResetTimezone
	}
	if err := validateReset(sub); err != nil {
		return nil, false, err
	}
	if _, err := s.subs.GetByName(sub.Name); err == nil {
		return nil, false, &ErrConflict{fmt.Sprintf("subscription %q already exists", sub.Name)}
	}
	if other, err := s.subs.GetByTrafficProvider(sub.TrafficProviderID); err == nil {
		return nil, false, &ErrConflict{fmt.Sprintf("traffic provider %q is already managed by subscription %q", tp.Name, other.Name)}
	}

	sub.ProviderName = ""
	sub.ProviderSource = ""

	// Metadata-only mutation: the draft is not touched, so there is nothing
	// to roll back. The partial unique index still rejects a concurrent
	// duplicate adoption as a conflict.
	if err := s.subs.Create(sub); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), "traffic_provider") {
			return nil, false, &ErrConflict{fmt.Sprintf("traffic provider %q is already managed by another subscription", tp.Name)}
		}
		return nil, false, err
	}
	v, err := s.View(sub)
	return v, false, err
}

// Update mutates metadata; nil fields keep stored values, an empty url keeps
// the stored secret. Provider rebinding requires the new provider to exist.
// Traffic sources update metadata-only: no draft mutation, no URL edit (the
// URL belongs to the traffic provider), source type immutable.
func (s *SubscriptionService) Update(id string, req *SubscriptionRequest) (*SubscriptionView, bool, error) {
	s.mutateMu.Lock()
	defer s.mutateMu.Unlock()

	// ── validation, all before any mutation ──
	sub, err := s.subs.Get(id)
	if err != nil {
		return nil, false, err
	}
	if sub.SourceType == model.SubscriptionSourceTraffic {
		return s.updateTrafficSource(sub, req)
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
	if sub.TrafficProviderID != "" {
		if _, err := s.traffic.GetProvider(sub.TrafficProviderID); err != nil {
			return nil, false, fmt.Errorf("traffic provider %q not found", sub.TrafficProviderID)
		}
	}

	// ── mutation: draft first (rollback-able), then metadata ──
	draftChanged := false
	var snap draftEntrySnapshot
	if req.URL != nil && strings.TrimSpace(*req.URL) != "" && sub.ProviderName != "" {
		snap = s.snapshotDraftProvider(sub.ProviderName)
		changed, err := s.upsertDraftProvider(sub.ProviderName, strings.TrimSpace(*req.URL))
		if err != nil {
			return nil, false, err
		}
		draftChanged = changed
	}

	if err := s.subs.UpdateMetadata(sub); err != nil {
		if draftChanged {
			if rerr := s.restoreDraftProvider(sub.ProviderName, snap); rerr != nil {
				slog.Error("subscription: draft rollback after failed update failed", "provider", sub.ProviderName, "err", rerr)
			}
		}
		return nil, draftChanged, err
	}
	v, err := s.View(sub)
	return v, draftChanged, err
}

// updateTrafficSource mutates a traffic-source subscription's metadata only.
// The linked traffic provider (and thus the URL authority) may be rebound;
// inherited quota/reset values are NOT re-derived on rebind — they stay as
// stored unless explicitly overridden in the request. draft_changed is
// always false. Caller holds s.mutateMu.
func (s *SubscriptionService) updateTrafficSource(sub *model.Subscription, req *SubscriptionRequest) (*SubscriptionView, bool, error) {
	oldName := sub.Name
	oldTPID := sub.TrafficProviderID
	if req.SourceType != nil && strings.TrimSpace(*req.SourceType) != sub.SourceType {
		return nil, false, fmt.Errorf("source_type cannot be changed after creation")
	}
	if err := req.applyTo(sub); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(sub.Name) == "" {
		return nil, false, fmt.Errorf("name is required")
	}
	if sub.ProviderName != "" {
		return nil, false, fmt.Errorf("provider_name must be empty for a traffic-source subscription")
	}
	if req.URL != nil && strings.TrimSpace(*req.URL) != "" {
		return nil, false, fmt.Errorf("url is owned by the linked traffic provider; edit it there instead of storing a second copy")
	}
	if err := validateReset(sub); err != nil {
		return nil, false, err
	}
	if sub.Name != oldName {
		if _, err := s.subs.GetByName(sub.Name); err == nil {
			return nil, false, &ErrConflict{fmt.Sprintf("subscription %q already exists", sub.Name)}
		}
	}
	if sub.TrafficProviderID == "" {
		return nil, false, fmt.Errorf("traffic_provider_id is required for a traffic-source subscription")
	}
	tp, err := s.traffic.GetProvider(sub.TrafficProviderID)
	if err != nil {
		return nil, false, fmt.Errorf("traffic provider %q not found", sub.TrafficProviderID)
	}
	if other, err := s.subs.GetByTrafficProvider(sub.TrafficProviderID); err == nil && other.ID != sub.ID {
		return nil, false, &ErrConflict{fmt.Sprintf("traffic provider %q is already managed by subscription %q", tp.Name, other.Name)}
	}

	if sub.TrafficProviderID != oldTPID {
		// Provider rebind: atomically save metadata AND clear every vendor
		// snapshot/status column — old-source usage/quota/expiry/reset-day and
		// refresh state must never be attributed to the new source. The row is
		// read back so the returned view reflects the cleared state.
		if err := s.subs.UpdateMetadataRebind(sub); err != nil {
			return nil, false, err
		}
		fresh, gerr := s.subs.Get(sub.ID)
		if gerr != nil {
			return nil, false, gerr
		}
		v, verr := s.View(fresh)
		return v, false, verr
	}

	// Same-source metadata save: field-specific snapshot safety — vendor data
	// and refresh state are untouched.
	if err := s.subs.UpdateMetadata(sub); err != nil {
		return nil, false, err
	}
	v, err := s.View(sub)
	return v, false, err
}

// Delete implements the real delete semantics: referenced providers are
// refused with their references; otherwise the native provider entry is
// removed from the draft, the metadata row and its score rows are deleted.
// Shared traffic-provider accounting is not touched. Business groups are
// never rewritten here. Traffic sources delete metadata-only: the traffic
// provider, its historical accounting and all static nodes/groups survive.
func (s *SubscriptionService) Delete(id string) (map[string]any, error) {
	s.mutateMu.Lock()
	defer s.mutateMu.Unlock()

	sub, err := s.subs.Get(id)
	if err != nil {
		return nil, err
	}
	if sub.SourceType == model.SubscriptionSourceTraffic {
		if err := s.subs.Delete(sub.ID); err != nil {
			return nil, err
		}
		return map[string]any{
			"status":                      "deleted",
			"source_type":                 model.SubscriptionSourceTraffic,
			"provider_name":               "",
			"provider_removed_from_draft": false,
			"draft_changed":               false,
		}, nil
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

	// Metadata first; if anything later fails the provider stays in the draft
	// (never silently removed while its manager row is gone).
	if sub.ProviderName != "" {
		if err := s.scores.DeleteByProvider(sub.ProviderName); err != nil {
			return nil, err
		}
	}
	if err := s.subs.Delete(sub.ID); err != nil {
		return nil, err
	}
	removedFromDraft := false
	if sub.ProviderName != "" {
		removedFromDraft, err = s.removeDraftProvider(sub.ProviderName)
		if err != nil {
			// The row is gone but the draft entry removal failed: surface it
			// instead of silently widening the change.
			return map[string]any{
				"status":                      "deleted",
				"provider_name":               sub.ProviderName,
				"provider_removed_from_draft": false,
				"draft_changed":               false,
				"warning":                     "subscription deleted but provider entry removal from draft failed: " + err.Error(),
			}, nil
		}
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
	if sub.SourceType == model.SubscriptionSourceTraffic {
		return s.refreshTrafficSource(sub)
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

	// Best-effort runtime provider refresh (downloads new node list). Native
	// proxy providers live under /providers/proxies/{name} — the rules
	// endpoint would 404.
	if entryErr == nil && s.mihomo != nil {
		if perr := s.mihomo.RefreshProxyProvider(sub.ProviderName); perr != nil {
			msg := "provider refresh failed: " + sanitizeMessage(perr)
			if refErr == nil {
				refErr = &RefreshError{Code: "provider_refresh_failed", Message: msg}
			} else {
				refErr.Message += "; " + msg
			}
		}
	}

	// Short critical section AFTER the network calls: re-validate the source
	// atomically against provider/URL rebind (metadata mutations hold the same
	// lock), then persist ONLY snapshot/status columns via SQL field-specific
	// updates — concurrent metadata edits are structurally unaffected.
	s.mutateMu.Lock()
	fresh, rerr := s.subs.Get(id)
	if rerr != nil {
		s.mutateMu.Unlock()
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
	if err := s.subs.UpdateSnapshot(fresh); err != nil {
		s.mutateMu.Unlock()
		return nil, &RefreshError{Code: "internal", Message: err.Error()}
	}
	s.mutateMu.Unlock()

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
// fetchUserinfo fetches a subscription URL and returns ONLY the header-format
// payload. Native subscriptions keep their existing behavior: an aggregate
// JSON payload (or any other non-header body) stays an error, exactly as
// before the shared vendor fetch existed.
func (s *SubscriptionService) fetchUserinfo(rawURL string) (*model.TrafficUserinfo, error) {
	snap, err := FetchVendorInfo(s.http, rawURL)
	if err != nil {
		return nil, err
	}
	if snap.Info == nil {
		return nil, fmt.Errorf("no subscription-userinfo header")
	}
	return snap.Info, nil
}

// refreshTrafficSource refreshes a traffic-source subscription's vendor
// metadata. The URL comes from the linked traffic provider and is never
// stored, logged or returned; errors are sanitized; a failed fetch keeps the
// last good snapshot (stale-on-error). A source without a subscription URL
// refreshes local info successfully with an explicit local-only status and
// unknown supplier expiry. No native /providers/proxies request is made, no
// draft mutation happens, and snapshots are never copied across sources.
func (s *SubscriptionService) refreshTrafficSource(sub *model.Subscription) (*SubscriptionView, *RefreshError) {
	now := s.nowFn()
	tp, err := s.traffic.GetProvider(sub.TrafficProviderID)
	if err != nil {
		msg := "linked traffic provider no longer exists"
		s.persistTrafficRefreshFailure(sub.ID, sub.TrafficProviderID, now, msg)
		return nil, &RefreshError{Code: "source_missing", Message: msg}
	}
	fetchURL := tp.SubscriptionURL
	fetchTPID := tp.ID // provider identity AND URL must both survive the fetch
	if fetchURL == "" {
		// Local-only refresh: succeed with explicit local-only semantics. The
		// whole branch runs under the mutation lock and re-validates provider
		// identity: a row rebound before we took the lock is a different
		// source and must not be marked refreshed from this attempt.
		s.mutateMu.Lock()
		fresh, gerr := s.subs.Get(sub.ID)
		if gerr != nil {
			s.mutateMu.Unlock()
			return nil, &RefreshError{Code: "internal", Message: gerr.Error()}
		}
		if fresh.TrafficProviderID != fetchTPID {
			s.mutateMu.Unlock()
			return nil, &RefreshError{Code: "userinfo_failed", Message: "traffic source changed during refresh; retry"}
		}
		fresh.LastRefreshAt = model.NewNullTime(now)
		fresh.LastSuccessAt = model.NewNullTime(now)
		fresh.LastRefreshError = ""
		if uerr := s.subs.UpdateSnapshot(fresh); uerr != nil {
			s.mutateMu.Unlock()
			return nil, &RefreshError{Code: "internal", Message: uerr.Error()}
		}
		s.mutateMu.Unlock()
		v, verr := s.View(fresh)
		if verr != nil {
			return nil, &RefreshError{Code: "internal", Message: verr.Error()}
		}
		return v, nil
	}

	// Network fetch OUTSIDE the mutation lock.
	snap, ferr := FetchVendorInfo(s.http, fetchURL)
	refErr := (*RefreshError)(nil)
	if ferr != nil {
		var rl *RateLimitError
		if errors.As(ferr, &rl) {
			secs := int(respectRetryAfter(rl.RetryAfter).Seconds())
			refErr = &RefreshError{Code: "rate_limited", Message: "subscription server rate limited; vendor snapshot kept", RetryAfterSecond: secs}
		} else {
			refErr = &RefreshError{Code: "userinfo_failed", Message: "vendor fetch failed: " + sanitizeMessage(ferr)}
		}
	}

	// Short critical section: re-validate the source atomically against
	// rebinds (metadata mutations hold the same lock) — BOTH provider id and
	// URL, since different providers sharing a URL are still different
	// sources. A stale attempt touches neither snapshot nor refresh state;
	// nothing is persisted for a row that rebinds mid-flight.
	s.mutateMu.Lock()
	fresh, rerr := s.subs.Get(sub.ID)
	if rerr != nil {
		s.mutateMu.Unlock()
		return nil, &RefreshError{Code: "internal", Message: rerr.Error()}
	}
	sourceChanged := false
	if cur, cerr := s.traffic.GetProvider(fresh.TrafficProviderID); cerr != nil || fresh.TrafficProviderID != fetchTPID || cur.SubscriptionURL != fetchURL {
		sourceChanged = true
	}
	if sourceChanged {
		// The result (success or failure) belongs to a source this row no
		// longer manages: drop it entirely instead of writing old-source data
		// or old-source failure state onto the new source.
		s.mutateMu.Unlock()
		if refErr == nil {
			refErr = &RefreshError{Code: "userinfo_failed", Message: "traffic source changed during refresh; fetched result discarded"}
		}
		return nil, refErr
	}
	fresh.LastRefreshAt = model.NewNullTime(now)
	if refErr == nil {
		applyVendorSnapshot(fresh, snap, now)
		fresh.LastSuccessAt = model.NewNullTime(now)
		fresh.LastRefreshError = ""
	} else {
		fresh.LastRefreshError = refErr.Message
	}
	if uerr := s.subs.UpdateSnapshot(fresh); uerr != nil {
		s.mutateMu.Unlock()
		return nil, &RefreshError{Code: "internal", Message: uerr.Error()}
	}
	s.mutateMu.Unlock()

	v, verr := s.View(fresh)
	if verr != nil {
		return nil, &RefreshError{Code: "internal", Message: verr.Error()}
	}
	return v, refErr
}

// persistTrafficRefreshFailure records a sanitized refresh failure without
// touching any snapshot field (the last good vendor data survives). The
// write is guarded by the traffic-provider identity the failure belonged
// to: a row rebound meanwhile must not inherit the old source's failure.
func (s *SubscriptionService) persistTrafficRefreshFailure(id, trafficProviderID string, now time.Time, msg string) {
	s.mutateMu.Lock()
	defer s.mutateMu.Unlock()
	fresh, err := s.subs.Get(id)
	if err != nil || fresh.TrafficProviderID != trafficProviderID {
		return
	}
	fresh.LastRefreshAt = model.NewNullTime(now)
	fresh.LastRefreshError = msg
	if err := s.subs.UpdateSnapshot(fresh); err != nil {
		slog.Warn("subscription: traffic-source failure persist failed", "err", err)
	}
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
