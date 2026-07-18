package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/store"
)

const (
	pollInterval     = 2 * time.Second
	flushInterval    = 60 * time.Second
	userinfoInterval = 30 * time.Minute
	alertInterval    = 5 * time.Minute
	retentionDays    = 90
	// Vendor-reported usage older than this is ignored for alerting.
	userinfoMaxAge = 3 * time.Hour
)

// connSample remembers the last-seen cumulative counters of one connection.
type connSample struct {
	node     string
	upload   int64
	download int64
}

// bucketKey identifies one in-memory accumulation bucket.
type bucketKey struct {
	bucket int64
	node   string
}

type delta struct {
	upload   int64
	download int64
}

// TrafficService polls mihomo's /connections API, attributes per-connection
// traffic deltas to the actual outbound node (chains[0]), accumulates them
// into hourly buckets, fetches vendor-reported usage from subscription URLs,
// and fires quota alerts.
type TrafficService struct {
	store    *store.TrafficStore
	mihomo   *MihomoClient
	notifier *NotifyService
	http     *http.Client

	mu       sync.Mutex // guards lastSeen, pending, baseline
	lastSeen map[string]connSample
	pending  map[bucketKey]delta
	baseline bool // true once the first poll established baselines
}

// NewTrafficService creates a new TrafficService.
func NewTrafficService(ts *store.TrafficStore, mihomo *MihomoClient, notifier *NotifyService) *TrafficService {
	return &TrafficService{
		store:    ts,
		mihomo:   mihomo,
		notifier: notifier,
		http:     &http.Client{Timeout: 30 * time.Second},
		lastSeen: make(map[string]connSample),
		pending:  make(map[bucketKey]delta),
	}
}

// Run starts the collection loops and blocks until ctx is cancelled.
func (s *TrafficService) Run(ctx context.Context) {
	poll := time.NewTicker(pollInterval)
	flush := time.NewTicker(flushInterval)
	userinfo := time.NewTicker(userinfoInterval)
	alerts := time.NewTicker(alertInterval)
	defer poll.Stop()
	defer flush.Stop()
	defer userinfo.Stop()
	defer alerts.Stop()

	// Initial userinfo fetch shortly after boot.
	go func() {
		select {
		case <-time.After(10 * time.Second):
			s.fetchAllUserinfo()
		case <-ctx.Done():
		}
	}()

	for {
		select {
		case <-ctx.Done():
			s.flush()
			return
		case <-poll.C:
			s.poll()
		case <-flush.C:
			s.flush()
		case <-userinfo.C:
			s.fetchAllUserinfo()
		case <-alerts.C:
			s.checkAlerts()
		}
	}
}

// ── Collection ───────────────────────────────────────────────────────────────

type mihomoConn struct {
	ID       string   `json:"id"`
	Upload   int64    `json:"upload"`
	Download int64    `json:"download"`
	Chains   []string `json:"chains"`
}

type mihomoConnections struct {
	Connections []mihomoConn `json:"connections"`
}

// poll reads /connections once and accumulates per-connection deltas.
func (s *TrafficService) poll() {
	raw, err := s.mihomo.GetConnections()
	if err != nil {
		// mihomo down or restarting — drop baselines so we don't misattribute
		// counter resets as negative deltas when it comes back.
		s.mu.Lock()
		s.lastSeen = make(map[string]connSample)
		s.baseline = false
		s.mu.Unlock()
		return
	}
	var resp mihomoConnections
	if err := json.Unmarshal(raw, &resp); err != nil {
		slog.Warn("traffic: parse connections failed", "err", err)
		return
	}
	s.ingest(resp.Connections, time.Now())
}

// ingest accumulates per-connection deltas from one /connections snapshot.
func (s *TrafficService) ingest(conns []mihomoConn, now time.Time) {
	bucket := now.Truncate(time.Hour).Unix()
	seen := make(map[string]connSample, len(conns))

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range conns {
		if len(c.Chains) == 0 {
			continue
		}
		node := c.Chains[0]
		seen[c.ID] = connSample{node: node, upload: c.Upload, download: c.Download}

		if !s.baseline {
			continue // first poll after start: baselines only
		}
		var up, down int64
		if last, ok := s.lastSeen[c.ID]; ok {
			up, down = c.Upload-last.upload, c.Download-last.download
			if up < 0 || down < 0 {
				// Counter reset (id reuse) — treat current values as fresh traffic.
				up, down = c.Upload, c.Download
			}
		} else {
			// Connection born since the last poll — all its traffic is new.
			up, down = c.Upload, c.Download
		}
		if up == 0 && down == 0 {
			continue
		}
		k := bucketKey{bucket: bucket, node: node}
		d := s.pending[k]
		d.upload += up
		d.download += down
		s.pending[k] = d
	}

	s.lastSeen = seen
	s.baseline = true
}

// flush persists pending in-memory deltas into hourly buckets.
func (s *TrafficService) flush() {
	s.mu.Lock()
	if len(s.pending) == 0 {
		s.mu.Unlock()
		return
	}
	taken := s.pending
	s.pending = make(map[bucketKey]delta)
	s.mu.Unlock()

	rows := make([]model.TrafficHourly, 0, len(taken))
	for k, d := range taken {
		rows = append(rows, model.TrafficHourly{Bucket: k.bucket, Node: k.node, Upload: d.upload, Download: d.download})
	}
	if err := s.store.AddHourly(rows); err != nil {
		slog.Error("traffic: flush failed", "err", err)
		// Put the deltas back so they retry on the next flush.
		s.mu.Lock()
		for k, d := range taken {
			cur := s.pending[k]
			cur.upload += d.upload
			cur.download += d.download
			s.pending[k] = cur
		}
		s.mu.Unlock()
		return
	}

	// Opportunistic retention pruning.
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()
	if err := s.store.PruneHourlyBefore(cutoff); err != nil {
		slog.Warn("traffic: prune failed", "err", err)
	}
}

// ── Vendor userinfo ──────────────────────────────────────────────────────────

// fetchAllUserinfo refreshes vendor-reported usage for all providers that
// have a subscription URL.
func (s *TrafficService) fetchAllUserinfo() {
	providers, err := s.store.ListProviders()
	if err != nil {
		slog.Error("traffic: list providers failed", "err", err)
		return
	}
	for _, p := range providers {
		if !p.Enabled || p.SubscriptionURL == "" {
			continue
		}
		info, err := s.fetchUserinfo(p.SubscriptionURL)
		if err != nil {
			slog.Warn("traffic: fetch userinfo failed", "provider", p.Name, "err", err)
			continue
		}
		info.ProviderID = p.ID
		if err := s.store.UpsertUserinfo(info); err != nil {
			slog.Error("traffic: store userinfo failed", "provider", p.Name, "err", err)
		}
	}
}

// fetchUserinfo GETs the subscription URL and parses the
// subscription-userinfo header. The UA must be a clash.meta variant —
// some vendors return empty configs (and no header) to unknown clients.
func (s *TrafficService) fetchUserinfo(url string) (*model.TrafficUserinfo, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "clash.meta")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("subscription returned %d", resp.StatusCode)
	}
	header := resp.Header.Get("subscription-userinfo")
	if header == "" {
		return nil, fmt.Errorf("no subscription-userinfo header")
	}
	return ParseUserinfo(header)
}

// ParseUserinfo parses "upload=1; download=2; total=3; expire=4".
func ParseUserinfo(header string) (*model.TrafficUserinfo, error) {
	info := &model.TrafficUserinfo{}
	found := false
	for _, part := range strings.Split(header, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		v, err := strconv.ParseInt(strings.TrimSpace(kv[1]), 10, 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(kv[0])) {
		case "upload":
			info.Upload, found = v, true
		case "download":
			info.Download, found = v, true
		case "total":
			info.Total, found = v, true
		case "expire":
			info.Expire = v
		}
	}
	if !found {
		return nil, fmt.Errorf("unparsable subscription-userinfo: %q", header)
	}
	return info, nil
}

// ── Cycle math & attribution ─────────────────────────────────────────────────

// CycleStart returns the start of the current billing cycle for a monthly
// quota that resets on resetDay (1-31), in now's location. Reset days beyond
// the end of a month are clamped to that month's last day.
func CycleStart(now time.Time, resetDay int) time.Time {
	if resetDay < 1 {
		resetDay = 1
	}
	year, month := now.Year(), now.Month()
	day := clampDay(year, month, resetDay)
	if now.Day() < day {
		// Cycle started in the previous month.
		prev := now.AddDate(0, -1, -now.Day()+1) // some day in the previous month
		year, month = prev.Year(), prev.Month()
		day = clampDay(year, month, resetDay)
	}
	return time.Date(year, month, day, 0, 0, 0, 0, now.Location())
}

func clampDay(year int, month time.Month, day int) int {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day > last {
		return last
	}
	return day
}

// MatchProvider returns the id of the first enabled provider whose pattern
// matches the node name, or "".
func MatchProvider(node string, providers []model.TrafficProvider) string {
	for _, p := range providers {
		if !p.Enabled {
			continue
		}
		for _, pat := range p.NodePatternList {
			if ok, err := path.Match(pat, node); err == nil && ok {
				return p.ID
			}
		}
	}
	return ""
}

// ── Summary ──────────────────────────────────────────────────────────────────

// NodeUsage is one node's aggregated usage inside a window.
type NodeUsage struct {
	Node     string `json:"node"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
	Total    int64  `json:"total"`
}

// ProviderSummary is one provider's current-cycle status.
type ProviderSummary struct {
	Provider    model.TrafficProvider  `json:"provider"`
	CycleStart  string                 `json:"cycle_start"`
	CycleEnd    string                 `json:"cycle_end"`
	Local       delta3                 `json:"local"`
	Reported    *model.TrafficUserinfo `json:"reported,omitempty"`
	UsageBytes  int64                  `json:"usage_bytes"`
	Percent     float64                `json:"percent"`
	TodayBytes  int64                  `json:"today_bytes"`
	EstDaysLeft float64                `json:"est_days_left"` // -1 = unknown
	TopNodes    []NodeUsage            `json:"top_nodes"`
}

type delta3 struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
}

// TrafficSummary is the full /api/traffic/summary payload.
type TrafficSummary struct {
	Providers  []ProviderSummary `json:"providers"`
	Unassigned []NodeUsage       `json:"unassigned"`
}

// Summary computes the current-cycle usage for every provider plus the
// usage of nodes not matched by any provider (over the last 30 days).
func (s *TrafficService) Summary() (*TrafficSummary, error) {
	// Flush pending deltas so the summary is current.
	s.flush()

	providers, err := s.store.ListProviders()
	if err != nil {
		return nil, err
	}
	now := time.Now()

	// Query enough history to cover every provider's cycle and 30 days.
	earliest := now.AddDate(0, 0, -31)
	for _, p := range providers {
		if cs := CycleStart(now, p.ResetDay); cs.Before(earliest) {
			earliest = cs
		}
	}
	rows, err := s.store.HourlySince(earliest.Unix())
	if err != nil {
		return nil, err
	}

	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	out := &TrafficSummary{Providers: []ProviderSummary{}, Unassigned: []NodeUsage{}}
	unassigned := map[string]*NodeUsage{}

	for _, p := range providers {
		cycleStart := CycleStart(now, p.ResetDay)
		cycleEnd := CycleStart(cycleStart.AddDate(0, 1, 0), p.ResetDay)
		ps := ProviderSummary{
			Provider:    p,
			CycleStart:  cycleStart.Format("2006-01-02"),
			CycleEnd:    cycleEnd.Format("2006-01-02"),
			EstDaysLeft: -1,
			TopNodes:    []NodeUsage{},
		}

		nodeTotals := map[string]*NodeUsage{}
		for _, r := range rows {
			if MatchProvider(r.Node, []model.TrafficProvider{p}) == "" {
				continue
			}
			if r.Bucket >= cycleStart.Unix() {
				ps.Local.Upload += r.Upload
				ps.Local.Download += r.Download
				nu := nodeTotals[r.Node]
				if nu == nil {
					nu = &NodeUsage{Node: r.Node}
					nodeTotals[r.Node] = nu
				}
				nu.Upload += r.Upload
				nu.Download += r.Download
				nu.Total += r.Upload + r.Download
			}
			if r.Bucket >= todayStart {
				ps.TodayBytes += r.Upload + r.Download
			}
		}
		ps.Local.Total = ps.Local.Upload + ps.Local.Download

		if info, err := s.store.GetUserinfo(p.ID); err == nil {
			ps.Reported = info
		}

		ps.UsageBytes = effectiveUsage(ps.Local.Total, ps.Reported, now)
		if p.QuotaBytes > 0 {
			ps.Percent = float64(ps.UsageBytes) * 100 / float64(p.QuotaBytes)
			elapsed := now.Sub(cycleStart).Hours() / 24
			if elapsed >= 0.5 && ps.UsageBytes > 0 {
				burnPerDay := float64(ps.UsageBytes) / elapsed
				ps.EstDaysLeft = float64(p.QuotaBytes-ps.UsageBytes) / burnPerDay
			}
		}

		for _, nu := range nodeTotals {
			ps.TopNodes = append(ps.TopNodes, *nu)
		}
		sort.Slice(ps.TopNodes, func(i, j int) bool { return ps.TopNodes[i].Total > ps.TopNodes[j].Total })
		if len(ps.TopNodes) > 10 {
			ps.TopNodes = ps.TopNodes[:10]
		}
		out.Providers = append(out.Providers, ps)
	}

	// Unassigned nodes over the last 30 days.
	monthAgo := now.AddDate(0, 0, -30).Unix()
	for _, r := range rows {
		if r.Bucket < monthAgo {
			continue
		}
		if MatchProvider(r.Node, providers) != "" {
			continue
		}
		nu := unassigned[r.Node]
		if nu == nil {
			nu = &NodeUsage{Node: r.Node}
			unassigned[r.Node] = nu
		}
		nu.Upload += r.Upload
		nu.Download += r.Download
		nu.Total += r.Upload + r.Download
	}
	for _, nu := range unassigned {
		out.Unassigned = append(out.Unassigned, *nu)
	}
	sort.Slice(out.Unassigned, func(i, j int) bool { return out.Unassigned[i].Total > out.Unassigned[j].Total })

	return out, nil
}

// effectiveUsage picks the usage figure used for percent/alerting: the max of
// locally-counted traffic and fresh vendor-reported traffic. Vendor numbers
// capture consumers outside this gateway (other devices using the same
// subscription); local numbers cover vendors that report nothing.
func effectiveUsage(local int64, reported *model.TrafficUserinfo, now time.Time) int64 {
	usage := local
	if reported != nil && now.Sub(reported.FetchedAt) < userinfoMaxAge {
		if vendor := reported.Upload + reported.Download; vendor > usage {
			usage = vendor
		}
	}
	return usage
}

// DailyHistory returns per-day usage grouped by provider for the last N days.
type DailyUsage struct {
	Date       string           `json:"date"`
	Providers  map[string]int64 `json:"providers"` // provider id -> bytes
	Unassigned int64            `json:"unassigned"`
}

// History aggregates hourly buckets into local-time days.
func (s *TrafficService) History(days int) ([]DailyUsage, error) {
	if days <= 0 || days > retentionDays {
		days = 30
	}
	now := time.Now()
	since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	rows, err := s.store.HourlySince(since.Unix())
	if err != nil {
		return nil, err
	}
	providers, err := s.store.ListProviders()
	if err != nil {
		return nil, err
	}

	byDate := map[string]*DailyUsage{}
	out := make([]DailyUsage, 0, days)
	for i := 0; i < days; i++ {
		d := since.AddDate(0, 0, i).Format("2006-01-02")
		out = append(out, DailyUsage{Date: d, Providers: map[string]int64{}})
		byDate[d] = &out[len(out)-1]
	}
	for _, r := range rows {
		d := time.Unix(r.Bucket, 0).In(now.Location()).Format("2006-01-02")
		du := byDate[d]
		if du == nil {
			continue
		}
		if pid := MatchProvider(r.Node, providers); pid != "" {
			du.Providers[pid] += r.Upload + r.Download
		} else {
			du.Unassigned += r.Upload + r.Download
		}
	}
	return out, nil
}

// ── Alerting ─────────────────────────────────────────────────────────────────

// checkAlerts evaluates thresholds for every provider and fires
// notifications for newly-crossed ones.
func (s *TrafficService) checkAlerts() {
	summary, err := s.Summary()
	if err != nil {
		slog.Error("traffic: alert check summary failed", "err", err)
		return
	}
	settings, err := s.store.GetSettings()
	if err != nil {
		slog.Error("traffic: alert settings failed", "err", err)
		return
	}
	var thresholds []int
	if err := json.Unmarshal([]byte(settings.AlertThresholds), &thresholds); err != nil || len(thresholds) == 0 {
		thresholds = []int{80, 95}
	}
	sort.Ints(thresholds)

	for _, ps := range summary.Providers {
		p := ps.Provider
		if !p.Enabled || p.QuotaBytes <= 0 {
			continue
		}
		for _, th := range thresholds {
			if ps.Percent < float64(th) {
				continue
			}
			msg := fmt.Sprintf(
				"%s 流量已用 %.1f%%（%s / %s），周期 %s ~ %s",
				p.Name, ps.Percent, FormatBytes(ps.UsageBytes), FormatBytes(p.QuotaBytes),
				ps.CycleStart, ps.CycleEnd,
			)
			if ps.EstDaysLeft >= 0 {
				msg += fmt.Sprintf("，按当前速率约 %.1f 天后耗尽", ps.EstDaysLeft)
			}
			alert := &model.TrafficAlert{
				ID:         uuid.New().String(),
				ProviderID: p.ID,
				CycleStart: ps.CycleStart,
				Threshold:  th,
				UsageBytes: ps.UsageBytes,
				QuotaBytes: p.QuotaBytes,
				Message:    msg,
			}
			inserted, err := s.store.InsertAlert(alert)
			if err != nil {
				slog.Error("traffic: insert alert failed", "err", err)
				continue
			}
			if !inserted {
				continue // already fired this cycle
			}
			slog.Warn("traffic: quota alert", "provider", p.Name, "threshold", th, "usage", ps.UsageBytes)
			title := fmt.Sprintf("代理流量告警：%s %d%%", p.Name, th)
			if err := s.notifier.Send(settings.AlertChannel, settings.AlertTarget, title, msg); err != nil {
				slog.Error("traffic: notify failed", "err", err)
				continue
			}
			if settings.AlertChannel != "" {
				_ = s.store.MarkAlertNotified(alert.ID)
			}
		}
	}
}

// FormatBytes renders a byte count in human-friendly binary units.
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
