package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/store"
)

// Scoring cadence and safety limits.
const (
	// RefreshInterval is the weekly automatic re-score cadence.
	RefreshInterval = 7 * 24 * time.Hour
	// DispatchInterval is how often the scheduler looks for due work.
	DispatchInterval = 30 * time.Second
	// ReconcileInterval is the periodic inventory+fingerprint sweep that
	// discovers new/changed provider-cache nodes and prunes removed ones.
	ReconcileInterval = 10 * time.Minute
	// DefaultConcurrency caps parallel isolated test instances.
	DefaultConcurrency = 2
	// StartupTimeout bounds one isolated instance's startup.
	StartupTimeout = 20 * time.Second
	// ProbeTimeout bounds one full node probe (traces + scoring requests).
	ProbeTimeout = 60 * time.Second
	// EgressCacheTTL reuses scores for an already-scored egress IP.
	EgressCacheTTL = 6 * 24 * time.Hour
	// MaxBackoff caps exponential retry spacing.
	MaxBackoff = 24 * time.Hour
)

// Error codes surfaced to the GUI (machine-readable, sanitized).
const (
	ErrCodeRateLimited   = "rate_limited"
	ErrCodeNoEgress      = "no_egress"
	ErrCodeConfigMissing = "config_missing"
	ErrCodeUpstream      = "upstream_error"
	ErrCodeTestFailed    = "test_failed"
)

// default trace endpoints (overridable in tests).
var (
	defaultClaudeTraceURL = "https://claude.ai/cdn-cgi/trace"
	defaultCFTraceURL     = "https://1.1.1.1/cdn-cgi/trace"
	defaultNetCoffeeBase  = NetCoffeeBaseURL
)

// EgressProber creates an isolated transport bound to one node topology.
// Implemented by MihomoIsolation in production; faked in tests.
type EgressProber interface {
	Probe(ctx context.Context, topo *IsolationTopology, iso IsolationConfig) (*EgressSession, error)
}

// ScoringService owns the durable node-score queue: inventory reconciliation
// (new/changed nodes enqueue promptly, removed ones prune), weekly refresh,
// per-egress-IP dedup, bounded concurrency, backoff and 429 handling.
// Persistence in node_scores makes restarts resume cleanly.
type ScoringService struct {
	scores   *store.NodeScoreStore
	proxies  *store.ProxyStore
	groups   *store.ProxyGroupStore
	subs     *store.SubscriptionStore
	configs  *store.ConfigStore
	settings *store.SettingsStore
	prober   EgressProber

	concurrency int
	wake        chan struct{}

	// proberMu guards the lazily-built default runner so a settings change
	// (new binary path) swaps it safely; an injected prober (tests) always
	// wins and is never rebuilt.
	proberMu      sync.Mutex
	defaultRunner *MihomoIsolation
	cachedBinary  string

	// overridable endpoints (tests)
	claudeTraceURL string
	cfTraceURL     string
	netcoffeeBase  string

	mu        sync.Mutex
	coolUntil time.Time            // global 429 cooldown
	provNodes map[string]provNodes // provider name → effective nodes (override applied)
	provGen   int                  // cache generation; bumped on invalidate

	nowFn func() time.Time
}

type provNodes struct {
	gen   int
	nodes []map[string]any // effective nodes: cache + override, raw names
}

// NewScoringService wires the scheduler. prober may be nil; the default
// MihomoIsolation runner is built lazily from settings.
func NewScoringService(
	scores *store.NodeScoreStore,
	proxies *store.ProxyStore,
	groups *store.ProxyGroupStore,
	subs *store.SubscriptionStore,
	configs *store.ConfigStore,
	settings *store.SettingsStore,
	prober EgressProber,
) *ScoringService {
	return &ScoringService{
		scores:         scores,
		proxies:        proxies,
		groups:         groups,
		subs:           subs,
		configs:        configs,
		settings:       settings,
		prober:         prober,
		concurrency:    DefaultConcurrency,
		wake:           make(chan struct{}, 1),
		claudeTraceURL: defaultClaudeTraceURL,
		cfTraceURL:     defaultCFTraceURL,
		netcoffeeBase:  defaultNetCoffeeBase,
		provNodes:      map[string]provNodes{},
		nowFn:          time.Now,
	}
}

// currentProber resolves the prober for one job: an injected prober (tests)
// always wins; otherwise the default isolated runner is built from the
// CURRENT settings, swapping safely when the binary path changes and failing
// with a resolvable error (never a panic) when no binary is configured.
func (s *ScoringService) currentProber() (EgressProber, error) {
	if s.prober != nil {
		return s.prober, nil
	}
	st, err := s.settings.Get()
	if err != nil {
		return nil, fmt.Errorf("read settings for egress prober: %w", err)
	}
	bin := strings.TrimSpace(st.MihomoBinary)
	if bin == "" {
		return nil, errors.New("mihomo binary is not configured in Settings")
	}
	s.proberMu.Lock()
	defer s.proberMu.Unlock()
	if s.defaultRunner == nil || s.cachedBinary != bin {
		s.defaultRunner = NewMihomoIsolation(bin)
		s.cachedBinary = bin
	}
	return s.defaultRunner, nil
}

// Wake nudges the scheduler loop (new work enqueued).
func (s *ScoringService) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run blocks until ctx is cancelled: recover stuck rows, reconcile inventory,
// dispatch due work, and re-reconcile periodically so provider refreshes and
// config changes are picked up without a restart.
func (s *ScoringService) Run(ctx context.Context) {
	if _, err := s.scores.ResetTesting(s.nowFn()); err != nil {
		slog.Warn("scoring: reset stuck rows failed", "err", err)
	}
	s.ScanAndEnqueue()

	ticker := time.NewTicker(DispatchInterval)
	scan := time.NewTicker(ReconcileInterval)
	defer ticker.Stop()
	defer scan.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.dispatch(ctx)
		case <-scan.C:
			s.ScanAndEnqueue()
			s.Wake()
		case <-s.wake:
			s.dispatch(ctx)
		}
	}
}

// disabledProviders returns provider names whose subscriptions are disabled
// (auto metadata refresh and auto scoring paused; the provider entry itself
// stays in the config and can still carry traffic).
func (s *ScoringService) disabledProviders() map[string]bool {
	out := map[string]bool{}
	if s.subs == nil {
		return out
	}
	subs, err := s.subs.List()
	if err != nil {
		return out
	}
	for _, sub := range subs {
		if !sub.Enabled && sub.ProviderName != "" {
			out[sub.ProviderName] = true
		}
	}
	return out
}

// ScanAndEnqueue reconciles the queue with current inventory: static proxies
// from the DB and effective provider nodes (cache + override) from the draft
// config. It always loads a FRESH provider snapshot (generation bumped at the
// start), so cache files auto-updated by mihomo on disk are picked up without
// manual invalidation or restart; within one round the snapshot is shared
// across all nodes of a provider (one read per provider per round).
// Changed fingerprints re-queue promptly while preserving the row's success
// history; removed nodes are pruned.
func (s *ScoringService) ScanAndEnqueue() {
	s.mu.Lock()
	s.provGen++
	s.mu.Unlock()
	disabled := s.disabledProviders()

	// ── static proxies ──────────────────────────────────────────────────
	proxies, err := s.proxies.List("", "")
	if err != nil {
		slog.Warn("scoring: list proxies failed", "err", err)
	} else {
		present := map[string]bool{}
		for _, p := range proxies {
			fp := staticFingerprint(p)
			present["static/"+p.Name] = true
			s.EnsureNodeWithFP("static", "", p.Name, fp)
		}
		rows, err := s.scores.List()
		if err == nil {
			for _, r := range rows {
				if r.Source == "static" && !present[r.NodeKey] {
					_ = s.scores.Delete(r.NodeKey)
				}
			}
		}
	}

	// ── provider nodes (effective = cache + override) ───────────────────
	entries, err := s.providerEntries()
	if err != nil {
		slog.Warn("scoring: read provider entries failed", "err", err)
		return
	}
	mihomoDir := s.mihomoDir()
	liveProviders := map[string]bool{}
	for _, e := range entries {
		liveProviders[e.Name] = true
		if disabled[e.Name] {
			continue // auto scoring paused for this subscription
		}
		nodes, err := s.loadEffectiveNodes(e, mihomoDir)
		if err != nil {
			// Cache not written yet (never published/fetched) — skip quietly.
			slog.Debug("scoring: provider nodes unavailable", "provider", e.Name, "err", err)
			continue
		}
		present := map[string]bool{}
		for _, n := range nodes {
			name, _ := n["name"].(string)
			if name == "" {
				continue
			}
			runtime := e.RuntimeName(name)
			present[store.NewNodeKey("provider", e.Name, runtime)] = true
			s.EnsureNodeWithFP("provider", e.Name, runtime, NodeFingerprint(n))
		}
		// Prune score rows of nodes that disappeared from the provider.
		rows, err := s.scores.List()
		if err == nil {
			for _, r := range rows {
				if r.Source == "provider" && r.ProviderName == e.Name && !present[r.NodeKey] {
					_ = s.scores.Delete(r.NodeKey)
				}
			}
		}
	}
	// Prune rows of providers that no longer exist in the draft at all.
	rows, err := s.scores.List()
	if err == nil {
		for _, r := range rows {
			if r.Source == "provider" && !liveProviders[r.ProviderName] {
				_ = s.scores.Delete(r.NodeKey)
			}
		}
	}
}

// loadEffectiveNodes reads the provider cache and applies the provider
// override, yielding the effective node maps (raw cache names). Result is
// cached per generation so one read/refresh is shared by every node.
func (s *ScoringService) loadEffectiveNodes(e providerEntry, mihomoDir string) ([]map[string]any, error) {
	s.mu.Lock()
	gen := s.provGen
	if cached, ok := s.provNodes[e.Name]; ok && cached.gen == gen {
		s.mu.Unlock()
		return cached.nodes, nil
	}
	s.mu.Unlock()

	iso := s.isolationRunner()
	raw, err := iso.LoadProviderNodes(e, mihomoDir)
	if err != nil {
		return nil, err
	}
	nodes := make([]map[string]any, 0, len(raw))
	for _, n := range raw {
		nodes = append(nodes, e.ApplyOverride(n))
	}
	s.mu.Lock()
	s.provNodes[e.Name] = provNodes{gen: gen, nodes: nodes}
	s.mu.Unlock()
	return nodes, nil
}

// InvalidateProviderCache drops cached provider node lists (e.g. after a
// subscription refresh re-downloaded the cache file).
func (s *ScoringService) InvalidateProviderCache() {
	s.mu.Lock()
	s.provGen++
	s.mu.Unlock()
}

// staticFingerprint hashes the proxy's effective node configuration.
func staticFingerprint(p *model.Proxy) string {
	var m map[string]any
	_ = json.Unmarshal(p.Config, &m)
	if m == nil {
		m = map[string]any{}
	}
	m["name"] = p.Name
	m["type"] = p.Type
	return NodeFingerprint(m)
}

// EnqueueStaticProxy ensures a queue row for a static proxy and wakes the
// scheduler (called after proxy create/update; changed configs re-queue).
func (s *ScoringService) EnqueueStaticProxy(name string) {
	if p, err := s.proxies.GetByName(name); err == nil {
		s.EnsureNodeWithFP("static", "", name, staticFingerprint(p))
	} else {
		s.EnsureNodeWithFP("static", "", name, "")
	}
	s.Wake()
}

// RemoveStaticProxy drops the queue row when a static proxy is deleted.
func (s *ScoringService) RemoveStaticProxy(name string) {
	_ = s.scores.Delete(store.NewNodeKey("static", "", name))
}

// EnsureNodeWithFP inserts a pending row if absent; when the fingerprint
// changed it re-queues the node promptly while preserving its history
// (checked_at / last_success_at) so restarts and edits do not lose data.
func (s *ScoringService) EnsureNodeWithFP(source, providerName, nodeName, fp string) {
	key := store.NewNodeKey(source, providerName, nodeName)
	existing, err := s.scores.Get(key)
	if errors.Is(err, store.ErrNotFound) {
		now := model.NewNullTime(s.nowFn())
		row := &model.NodeScore{
			NodeKey:      key,
			Source:       source,
			NodeName:     nodeName,
			ProviderName: providerName,
			Status:       model.ScoreStatusPending,
			Fingerprint:  fp,
			NextDue:      now,
		}
		if err := s.scores.Upsert(row); err != nil {
			slog.Warn("scoring: enqueue node failed", "node", key, "err", err)
		}
		return
	}
	if err != nil {
		slog.Warn("scoring: get node row failed", "node", key, "err", err)
		return
	}
	if existing.Fingerprint == fp && existing.NodeName == nodeName {
		return // unchanged — keep schedule and history
	}
	now := model.NewNullTime(s.nowFn())
	existing.Fingerprint = fp
	existing.NodeName = nodeName
	existing.Status = model.ScoreStatusPending
	existing.NextDue = now
	existing.Attempts = 0
	if err := s.scores.Upsert(existing); err != nil {
		slog.Warn("scoring: requeue changed node failed", "node", key, "err", err)
	}
}

// RefreshNodes re-queues the given node keys (or everything when all=true).
// Returns queued/running/skipped counts for the GUI.
func (s *ScoringService) RefreshNodes(ids []string, all bool) (queued, running, skipped int, err error) {
	now := s.nowFn()
	var keys []string
	if all {
		rows, listErr := s.scores.List()
		if listErr != nil {
			return 0, 0, 0, listErr
		}
		for _, r := range rows {
			keys = append(keys, r.NodeKey)
		}
	} else {
		keys = ids
	}
	for _, key := range keys {
		row, getErr := s.scores.Get(key)
		if errors.Is(getErr, store.ErrNotFound) {
			skipped++
			continue
		} else if getErr != nil {
			return queued, running, skipped, getErr
		}
		if row.Status == model.ScoreStatusTesting {
			running++
			continue
		}
		row.Status = model.ScoreStatusPending
		row.NextDue = model.NewNullTime(now)
		row.ManualRequest = true // survives restart; runs even when paused
		if upErr := s.scores.Upsert(row); upErr != nil {
			return queued, running, skipped, upErr
		}
		queued++
	}
	if queued > 0 {
		s.Wake()
	}
	return queued, running, skipped, nil
}

// ── dispatch & processing ────────────────────────────────────────────────────

func (s *ScoringService) dispatch(ctx context.Context) {
	s.mu.Lock()
	cool := s.coolUntil
	s.mu.Unlock()
	now := s.nowFn()
	if now.Before(cool) {
		return
	}

	// Automatic dispatch skips paused subscriptions entirely (their backlog
	// cannot starve other providers behind the batch limit); rows carrying a
	// persisted manual request still run.
	paused := s.disabledProviders()
	pausedNames := make([]string, 0, len(paused))
	for name := range paused {
		pausedNames = append(pausedNames, name)
	}

	sem := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup
	for {
		rows, err := s.scores.ListDue(now, 32, pausedNames)
		if err != nil {
			slog.Warn("scoring: list due failed", "err", err)
			return
		}
		progressed := false
		for i := range rows {
			row := rows[i]
			select {
			case sem <- struct{}{}:
			default:
				wg.Wait()
				return // concurrency cap reached
			}
			claimed, err := s.scores.Claim(row.NodeKey, s.nowFn())
			if err != nil {
				slog.Warn("scoring: claim failed", "node", row.NodeKey, "err", err)
				<-sem
				continue
			}
			if !claimed {
				<-sem
				continue
			}
			progressed = true
			wg.Add(1)
			go func(r model.NodeScore) {
				defer wg.Done()
				defer func() { <-sem }()
				s.processNode(ctx, r)
			}(row)
		}
		if !progressed {
			wg.Wait()
			return
		}
		wg.Wait()
	}
}

// materialize builds the isolated topology for one queue row: provider rows
// use the effective node set (cache + override) and resolve cross-static
// dialer-proxy dependencies; static rows resolve their dialer-proxy chain.
func (s *ScoringService) materialize(row model.NodeScore) (*IsolationTopology, error) {
	switch row.Source {
	case "provider":
		e, err := s.findProviderEntry(row.ProviderName)
		if err != nil {
			return nil, err
		}
		nodes, err := s.loadEffectiveNodes(e, s.mihomoDir())
		if err != nil {
			return nil, err
		}
		cacheName, ok := e.CacheName(row.NodeName)
		if !ok {
			return nil, fmt.Errorf("node %q does not map into provider %q cache", row.NodeName, row.ProviderName)
		}
		found := false
		for _, n := range nodes {
			if name, _ := n["name"].(string); name == cacheName {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("node %q not present in provider %q cache", cacheName, row.ProviderName)
		}
		topo := &IsolationTopology{Target: cacheName, Nodes: nodes}

		// Resolve dialer-proxy references that fall outside the provider
		// (cross-static chains): pull the referenced proxies/groups in.
		staticProxies, err := s.proxies.List("", "")
		if err != nil {
			return nil, err
		}
		staticGroups, err := s.groups.List()
		if err != nil {
			return nil, err
		}
		providerNames := map[string]bool{}
		for _, n := range nodes {
			if name, _ := n["name"].(string); name != "" {
				providerNames[name] = true
			}
		}
		known := map[string]bool{}
		for _, n := range topo.Nodes {
			if name, _ := n["name"].(string); name != "" {
				known[name] = true
			}
		}
		for _, g := range topo.Groups {
			known[g["name"].(string)] = true
		}
		for _, n := range nodes {
			dp, _ := n["dialer-proxy"].(string)
			if dp == "" || providerNames[dp] || known[dp] {
				continue
			}
			sub, err := ResolveDialerDeps(dp, staticProxies, staticGroups)
			if err != nil {
				return nil, fmt.Errorf("resolve dialer-proxy %q: %w", dp, err)
			}
			for _, pn := range sub.Nodes {
				name, _ := pn["name"].(string)
				if name != "" && !known[name] {
					known[name] = true
					topo.Nodes = append(topo.Nodes, pn)
				}
			}
			for _, pg := range sub.Groups {
				name, _ := pg["name"].(string)
				if name != "" && !known[name] {
					known[name] = true
					topo.Groups = append(topo.Groups, pg)
				}
			}
		}
		return topo, nil

	default: // static
		proxies, err := s.proxies.List("", "")
		if err != nil {
			return nil, err
		}
		groups, err := s.groups.List()
		if err != nil {
			return nil, err
		}
		return ResolveDialerDeps(row.NodeName, proxies, groups)
	}
}

// processNode runs one full probe and persists the outcome.
func (s *ScoringService) processNode(ctx context.Context, row model.NodeScore) {
	logCtx := slog.With("node", row.NodeKey)
	topo, err := s.materialize(row)
	if err != nil {
		s.fail(row, ErrCodeConfigMissing, sanitizeMessage(err), s.nowFn())
		logCtx.Warn("scoring: materialize failed", "err", err)
		return
	}

	prober, perr := s.currentProber()
	if perr != nil {
		s.fail(row, ErrCodeConfigMissing, sanitizeMessage(perr), s.nowFn())
		logCtx.Warn("scoring: no usable prober", "err", perr)
		return
	}

	isoCfg := s.isolationConfig()
	iface, gerr := s.underlayInterface(isoCfg.Interface)
	if gerr != nil {
		s.fail(row, ErrCodeConfigMissing, sanitizeMessage(gerr), s.nowFn())
		logCtx.Warn("scoring: isolation guard refused probe", "err", gerr)
		return
	}
	isoCfg.Interface = iface

	probeCtx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()
	session, err := prober.Probe(probeCtx, topo, isoCfg)
	if err != nil {
		s.fail(row, ErrCodeTestFailed, sanitizeMessage(err), s.nowFn())
		logCtx.Warn("scoring: isolated probe failed", "err", err)
		return
	}
	defer session.Close()

	claudeIP, egressIP, source := s.observeEgress(probeCtx, session.Client)
	queryIP := claudeIP
	if queryIP == "" {
		queryIP = egressIP
	}
	if queryIP == "" {
		s.fail(row, ErrCodeNoEgress, "no egress IP observed through the node", s.nowFn())
		return
	}

	scores, info, rateLimit, perr := s.collectScores(probeCtx, session.Client, queryIP, claudeIP != "")
	if rateLimit != nil {
		now := s.nowFn()
		row.Status = model.ScoreStatusPending
		row.ManualRequest = false // consumed; 429 keeps its Retry-After backoff
		row.ErrorCode = ErrCodeRateLimited
		row.LastError = sanitizeMessage(rateLimit)
		row.CheckedAt = model.NewNullTime(now)
		row.NextDue = model.NewNullTime(now.Add(clampDuration(rateLimit.RetryAfter, time.Minute, MaxBackoff)))
		_ = s.scores.Upsert(&row)
		s.coolGlobal(clampDuration(rateLimit.RetryAfter, time.Minute, MaxBackoff))
		logCtx.Warn("scoring: rate limited", "retry_after", rateLimit.RetryAfter)
		return
	}
	if perr != nil && scores.claudeScore == nil && scores.lookupMatch == model.IPMatchUnknown {
		s.fail(row, ErrCodeUpstream, sanitizeMessage(perr), s.nowFn())
		logCtx.Warn("scoring: upstream failed", "err", perr)
		return
	}

	now := s.nowFn()
	row.Status = model.ScoreStatusOK
	row.ManualRequest = false // consumed
	row.ErrorCode = ""
	row.ClaudeScore = scores.claudeScore
	row.ClaudeIPMatch = scores.claudeMatch
	row.ClaudeResponseIP = scores.claudeResponseIP
	row.LookupMatch = scores.lookupMatch
	row.EgressIP = queryIP
	row.ExitIPSource = source
	row.IPInfo = model.NullJSON{Bytes: info}
	row.CheckedAt = model.NewNullTime(now)
	row.LastSuccessAt = model.NewNullTime(now)
	row.NextDue = model.NewNullTime(now.Add(RefreshInterval))
	row.Attempts = 0
	row.LastError = ""
	if perr != nil {
		// Partial success (e.g. claude source down but lookup answered).
		row.LastError = sanitizeMessage(perr)
	}
	if err := s.scores.Upsert(&row); err != nil {
		logCtx.Error("scoring: persist result failed", "err", err)
	}
	logCtx.Info("scoring: node scored", "egress", queryIP)
}

// probeResult bundles what collectScores produced.
type probeResult struct {
	claudeScore      *int
	claudeMatch      string
	claudeResponseIP string
	lookupMatch      string
}

// collectScores fetches the two independent public sources (with per-egress-IP
// dedup) and assembles the stored ip_info document.
func (s *ScoringService) collectScores(ctx context.Context, client *http.Client, queryIP string, claudeVisible bool) (probeResult, json.RawMessage, *RateLimitError, error) {
	var res probeResult
	res.claudeMatch = model.IPMatchUnknown
	res.lookupMatch = model.IPMatchUnknown

	// Dedup: reuse a fresh cached scoring for the same egress IP — the node
	// still proves its own egress via the traces, but the vendor APIs are not
	// re-queried for an IP we already scored inside the TTL.
	if cached, err := s.scores.GetEgressIPScore(queryIP); err == nil && !cached.FetchedAt.IsZero() && s.nowFn().Sub(cached.FetchedAt.Time) < EgressCacheTTL {
		var info struct {
			Claude *claudeInfoLite `json:"claude"`
		}
		if json.Unmarshal(cached.IPInfo.Bytes, &info) == nil && info.Claude != nil {
			res.claudeScore = info.Claude.TrustScore
			res.claudeMatch = info.Claude.IPMatch
			res.claudeResponseIP = info.Claude.ResponseIP
		}
		res.lookupMatch = lookupMatchFromInfo(cached.IPInfo.Bytes)
		return res, cached.IPInfo.Bytes, nil, nil
	}

	nc := &NetCoffeeClient{BaseURL: s.netcoffeeBase, HTTP: client}
	var (
		risk   *ClaudeTrustRisk
		lookup json.RawMessage
		lmatch string
		rerr   error
		lerr   error
	)
	if claudeVisible {
		risk, rerr = nc.FetchClaudeTrust(ctx, queryIP)
		if rl, ok := rerr.(*RateLimitError); ok {
			return res, nil, rl, nil
		}
	}
	lookup, lmatch, lerr = nc.FetchIPLookup(ctx, queryIP)
	if rl, ok := lerr.(*RateLimitError); ok {
		return res, nil, rl, nil
	}

	if risk != nil {
		res.claudeScore = risk.TrustScore
		res.claudeMatch = risk.IPMatch
		res.claudeResponseIP = risk.ResponseIP
	}
	res.lookupMatch = lmatch

	if rerr != nil && lerr != nil {
		return res, nil, nil, fmt.Errorf("both sources failed: %v; %v", rerr, lerr)
	}
	var partial error
	if rerr != nil {
		partial = fmt.Errorf("claude source failed: %v", rerr)
	} else if lerr != nil {
		partial = fmt.Errorf("lookup source failed: %v", lerr)
	}

	info := buildIPInfo(queryIP, risk, lookup)
	if err := s.scores.PutEgressIPScore(&store.EgressIPScore{
		EgressIP:    queryIP,
		ClaudeScore: res.claudeScore,
		IPInfo:      model.NullJSON{Bytes: info},
	}); err != nil {
		slog.Warn("scoring: egress cache write failed", "err", err)
	}
	return res, info, nil, partial
}

// claudeInfoLite mirrors the stored claude summary inside ip_info.
type claudeInfoLite struct {
	TrustScore *int   `json:"trust_score"`
	IPMatch    string `json:"ip_match"`
	ResponseIP string `json:"ip"`
	CIDR       string `json:"cidr,omitempty"`
	QueriedIP  string `json:"query_ip"`
}

// buildIPInfo assembles the stored document: {"query_ip", "claude", "lookup"}.
// The claude summary carries its own IP validation; the lookup subset its own.
func buildIPInfo(queryIP string, risk *ClaudeTrustRisk, lookup json.RawMessage) json.RawMessage {
	doc := map[string]any{"query_ip": queryIP}
	if risk != nil {
		doc["claude"] = risk
	} else {
		doc["claude"] = nil
	}
	if lookup != nil {
		doc["lookup"] = json.RawMessage(lookup)
	} else {
		doc["lookup"] = nil
	}
	return mustJSON(doc)
}

// lookupMatchFromInfo extracts the lookup source's ip_match from a stored
// ip_info document.
func lookupMatchFromInfo(info json.RawMessage) string {
	var doc struct {
		Lookup json.RawMessage `json:"lookup"`
	}
	if json.Unmarshal(info, &doc) != nil || len(doc.Lookup) == 0 {
		return model.IPMatchUnknown
	}
	var l struct {
		IPMatch string `json:"ip_match"`
	}
	if json.Unmarshal(doc.Lookup, &l) != nil {
		return model.IPMatchUnknown
	}
	if l.IPMatch == "" {
		return model.IPMatchUnknown
	}
	return l.IPMatch
}

// observeEgress determines the node's real exit IP through full HTTP traces
// against the same endpoints the public pages use. Claude-visible IP first;
// Cloudflare trace as the general fallback.
func (s *ScoringService) observeEgress(ctx context.Context, client *http.Client) (claudeIP, egressIP, source string) {
	if ip, err := fetchTraceIP(ctx, client, s.claudeTraceURL); err == nil && ip != "" {
		return ip, ip, "claude-trace"
	}
	if ip, err := fetchTraceIP(ctx, client, s.cfTraceURL); err == nil && ip != "" {
		return "", ip, "cloudflare-trace"
	}
	return "", "", ""
}

// fetchTraceIP GETs a Cloudflare-style /cdn-cgi/trace and extracts ip=…
func fetchTraceIP(ctx context.Context, client *http.Client, traceURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, traceURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", sanitizeURLError("trace", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("trace returned HTTP %d", resp.StatusCode)
	}
	for _, line := range strings.Split(string(body), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "ip="); ok {
			return strings.TrimSpace(v), nil
		}
	}
	return "", fmt.Errorf("no ip= line in trace response")
}

// fail marks a row failed with backoff. Single failures never block the queue.
func (s *ScoringService) fail(row model.NodeScore, code, msg string, now time.Time) {
	row.Status = model.ScoreStatusFailed
	row.ErrorCode = code
	row.ManualRequest = false // consumed
	row.LastError = msg
	row.CheckedAt = model.NewNullTime(now)
	row.Attempts++
	backoff := time.Duration(30*time.Minute) << min(row.Attempts-1, 6) // 30m,1h,2h,4h… capped
	row.NextDue = model.NewNullTime(now.Add(min(backoff, MaxBackoff)))
	if err := s.scores.Upsert(&row); err != nil {
		slog.Warn("scoring: persist failure failed", "node", row.NodeKey, "err", err)
	}
}

func (s *ScoringService) coolGlobal(d time.Duration) {
	s.mu.Lock()
	s.coolUntil = s.nowFn().Add(d)
	s.mu.Unlock()
}

// ── provider/settings helpers ────────────────────────────────────────────────

// providerEntries reads the native proxy-providers map from the draft config.
func (s *ScoringService) providerEntries() ([]providerEntry, error) {
	raw, err := s.configs.Get()
	if err != nil {
		return nil, err
	}
	return providerEntriesFromConfig(raw), nil
}

func providerEntriesFromConfig(raw json.RawMessage) []providerEntry {
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil
	}
	var provs map[string]map[string]any
	if v, ok := cfg["proxy-providers"]; ok {
		if err := json.Unmarshal(v, &provs); err != nil {
			return nil
		}
	}
	names := make([]string, 0, len(provs))
	for name := range provs {
		names = append(names, name)
	}
	sortStrings(names)
	out := make([]providerEntry, 0, len(provs))
	for _, name := range names {
		out = append(out, parseProviderEntry(name, provs[name]))
	}
	return out
}

func (s *ScoringService) findProviderEntry(name string) (providerEntry, error) {
	entries, err := s.providerEntries()
	if err != nil {
		return providerEntry{}, err
	}
	for _, e := range entries {
		if e.Name == name {
			return e, nil
		}
	}
	return providerEntry{}, fmt.Errorf("provider %q not found in system config draft", name)
}

// mihomoDir reads the configured mihomo working directory from settings.
func (s *ScoringService) mihomoDir() string {
	st, err := s.settings.Get()
	if err != nil {
		return ""
	}
	return st.MihomoDir
}

// isolationConfig reads the operator-configured underlay for the throwaway
// instance (physical interface, explicit DNS upstreams).
func (s *ScoringService) isolationConfig() IsolationConfig {
	st, err := s.settings.Get()
	if err != nil {
		return IsolationConfig{DNSServers: defaultIsolationDNS()}
	}
	return isolationSettings(st)
}

// underlayInterface enforces the deployment-critical isolation guard. The
// managed draft saying "no TUN / no fake-ip" is NOT proof that the production
// instance is not tunneling (it may run other config), so an unbound child
// socket is never allowed:
//
//   - an explicitly configured isolation_interface is accepted only if the
//     interface exists, is up and is NOT loopback;
//   - an empty setting falls back to a physical interface explicitly
//     configured in the managed config ("…#<iface>" nameserver suffix),
//     accepted only when every suffixed entry agrees and the interface
//     passes the same checks;
//   - no verified interface, or an unreadable/corrupt managed draft (fail
//     closed) → error; the probe is refused (config_missing) until the
//     operator sets the underlay via the settings API.
//
// No deployment-specific names live in this code.
func (s *ScoringService) underlayInterface(configured string) (string, error) {
	refuse := func(reason string) (string, error) {
		return "", fmt.Errorf("isolation_interface is not usable (%s): refusing an unbound isolated probe; "+
			"set the physical underlay interface in Settings (isolation_interface)", reason)
	}
	if configured != "" {
		if !usableNonLoopbackInterface(configured) {
			return refuse("configured interface " + configured + " missing, down or loopback")
		}
		return configured, nil
	}
	draft, err := s.configs.Get()
	if err != nil {
		return refuse("cannot read managed config")
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(draft, &cfg); err != nil {
		// Corrupt draft must fail closed, never open.
		return refuse("managed config unreadable")
	}
	suffix := deriveIfaceSuffix(cfg)
	if suffix == "" {
		return refuse("no configured interface and no verifiable #<iface> suffix in managed config nameservers")
	}
	if !usableNonLoopbackInterface(suffix) {
		return refuse("derived interface " + suffix + " missing, down or loopback")
	}
	return suffix, nil
}

// usableNonLoopbackInterface reports whether the named interface exists, is
// up, and is not loopback — the minimum for a physical underlay binding.
func usableNonLoopbackInterface(name string) bool {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return false
	}
	if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
		return false
	}
	return true
}

// deriveIfaceSuffix scans the managed config's nameserver entries for an
// explicit physical-interface suffix ("…#<iface>") and returns it when every
// suffixed entry agrees on the same interface.
func deriveIfaceSuffix(cfg map[string]json.RawMessage) string {
	var dns struct {
		Nameserver            []string `json:"nameserver"`
		ProxyServerNameserver []string `json:"proxy-server-nameserver"`
	}
	raw, ok := cfg["dns"]
	if !ok || json.Unmarshal(raw, &dns) != nil {
		return ""
	}
	suffix := ""
	for _, entry := range append(append([]string{}, dns.Nameserver...), dns.ProxyServerNameserver...) {
		i := strings.LastIndex(entry, "#")
		if i < 0 || i == len(entry)-1 {
			continue
		}
		cur := entry[i+1:]
		if suffix == "" {
			suffix = cur
		} else if suffix != cur {
			return "" // contradictory suffixes — cannot derive safely
		}
	}
	return suffix
}

func (s *ScoringService) isolationRunner() *MihomoIsolation {
	if m, ok := s.prober.(*MihomoIsolation); ok {
		return m
	}
	return &MihomoIsolation{}
}

// EnsureProber installs the default isolated runner with the given binary.
func (s *ScoringService) EnsureProber(binPath string) {
	if s.prober == nil {
		s.prober = NewMihomoIsolation(binPath)
	}
}

func clampDuration(d, min, max time.Duration) time.Duration {
	if d < min {
		return min
	}
	if d > max {
		return max
	}
	return d
}

func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// sanitizeMessage strips anything URL-shaped from messages destined for
// storage or the API so credentials/endpoints can never leak.
func sanitizeMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if i := strings.Index(msg, "http://"); i >= 0 {
		msg = msg[:i] + "[redacted]"
	}
	if i := strings.Index(msg, "https://"); i >= 0 {
		msg = msg[:i] + "[redacted]"
	}
	return msg
}
