package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const NativePrecheckLimit = 20 * time.Second
const nativeControllerLimit = 64 << 10

// NativePrecheckRequest carries claims to be checked, never trusted evidence.
// There is deliberately no caller-selectable target, controller or source.
type NativePrecheckRequest struct {
	Operation    string   `json:"operation"`
	ChildContext string   `json:"child_context"`
	Peers        []string `json:"peers"`
	BudgetMS     int      `json:"budget_ms"`
}

func (r NativePrecheckRequest) Valid() bool {
	if r.Operation != "native-precheck" || len(r.ChildContext) == 0 || len(r.ChildContext) > 64 || r.BudgetMS < 1 || r.BudgetMS > 20000 || len(r.Peers) == 0 || len(r.Peers) > 256 {
		return false
	}
	for _, c := range r.ChildContext {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	seen := make(map[netip.Addr]bool, len(r.Peers))
	for _, peer := range r.Peers {
		ip, err := netip.ParseAddr(peer)
		if err != nil || !ip.Is4() || ip.String() != peer || seen[ip] {
			return false
		}
		seen[ip] = true
	}
	return true
}

// NativePrecheckFacts remain UNKNOWN until a trusted runtime source and an
// adoption-owned target/context/complete-peer binding actually exist.
type NativePrecheckFacts struct {
	TargetBinding       string `json:"target_binding"`
	ChildContextBinding string `json:"child_context_binding"`
	CompletePeerBinding string `json:"complete_peer_binding"`
	DNSNoFakeIP         string `json:"dns_no_fake_ip"`
	EffectivePolicy     string `json:"effective_policy"`
	ProxyExclusion      string `json:"proxy_exclusion"`
	SourceVersion       string `json:"source_version"`
	ObservedAt          string `json:"observed_at"`
	ValidUntil          string `json:"valid_until"`
	LoadedAssociation   string `json:"loaded_association"`
}

type NativePrecheckFragments struct {
	Method            string `json:"method"`
	ProjectionVersion int    `json:"projection_version"`
	RuleCount         int    `json:"rule_count"`
	ConnectionCount   int    `json:"connection_count"`
}

type NativePrecheckResult struct {
	Status    string                  `json:"status"`
	Facts     NativePrecheckFacts     `json:"facts"`
	Fragments NativePrecheckFragments `json:"fragments"`
}

// Closed sentinel errors never contain upstream bodies, URLs or transport text.
var (
	ErrNativePrecheckInput    = errors.New("invalid native precheck request")
	ErrNativePrecheckDeadline = errors.New("native precheck deadline exceeded")
	ErrNativePrecheckSource   = errors.New("native precheck fragments unavailable")
)

type NativePrecheckService struct{ mihomo *MihomoClient }

// NewNativePrecheckService reuses the normal client; it adds no trusted-source
// setter. The current normal wiring can only return NOT_BOUND, never qualify.
func NewNativePrecheckService(mc *MihomoClient) *NativePrecheckService {
	return &NativePrecheckService{mihomo: mc}
}

func (s *NativePrecheckService) Check(parent context.Context, in NativePrecheckRequest) (NativePrecheckResult, error) {
	// Start the one local total deadline before validation, reads and projection.
	started := time.Now()
	ctx, cancel := context.WithDeadline(parent, started.Add(NativePrecheckLimit))
	defer cancel()
	if !in.Valid() {
		return NativePrecheckResult{}, ErrNativePrecheckInput
	}
	ctx, budgetCancel := context.WithDeadline(ctx, started.Add(time.Duration(in.BudgetMS)*time.Millisecond))
	defer budgetCancel()
	fail := func() (NativePrecheckResult, error) {
		if nativePrecheckExpired(ctx) {
			return NativePrecheckResult{}, ErrNativePrecheckDeadline
		}
		return NativePrecheckResult{}, ErrNativePrecheckSource
	}
	if nativePrecheckExpired(ctx) {
		return fail()
	}
	if s.mihomo == nil || s.mihomo.client == nil {
		return fail()
	}
	// Clone only this getter's client/transport. No shared client mutation, no
	// redirects, and no reused connections (net/http may retry reused GETs).
	client := *s.mihomo.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if original, ok := transport.(*http.Transport); ok {
		isolated := original.Clone()
		isolated.DisableKeepAlives = true
		// HTTP/2 may transparently retry GOAWAY/refused streams. This isolated
		// getter uses one fresh HTTP/1 connection per read instead.
		isolated.ForceAttemptHTTP2 = false
		isolated.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		if isolated.TLSClientConfig != nil {
			isolated.TLSClientConfig = isolated.TLSClientConfig.Clone()
			isolated.TLSClientConfig.NextProtos = []string{"http/1.1"}
		}
		client.Transport = isolated
		defer isolated.CloseIdleConnections()
	}
	rules, err := s.readFragment(ctx, &client, "/rules", "rules")
	if err != nil {
		return fail()
	}
	connections, err := s.readFragment(ctx, &client, "/connections", "connections")
	if err != nil {
		return fail()
	}
	result := NativePrecheckResult{
		Status: "NOT_BOUND",
		Facts: NativePrecheckFacts{
			TargetBinding: "UNKNOWN", ChildContextBinding: "UNKNOWN", CompletePeerBinding: "UNKNOWN",
			DNSNoFakeIP: "UNKNOWN", EffectivePolicy: "UNKNOWN", ProxyExclusion: "UNKNOWN",
			SourceVersion: "UNKNOWN", ObservedAt: "UNKNOWN", ValidUntil: "UNKNOWN", LoadedAssociation: "UNKNOWN",
		},
		Fragments: NativePrecheckFragments{"GET /rules; GET /connections", 1, rules, connections},
	}
	if nativePrecheckExpired(ctx) {
		return fail()
	}
	return result, nil
}

func (s *NativePrecheckService) readFragment(ctx context.Context, client *http.Client, path, field string) (int, error) {
	// Only the two literal calls above reach this private method.
	base, err := url.Parse(s.mihomo.baseURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
		return 0, ErrNativePrecheckSource
	}
	base.Path = path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return 0, ErrNativePrecheckSource
	}
	if s.mihomo.secret != "" {
		req.Header.Set("Authorization", "Bearer "+s.mihomo.secret)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, ErrNativePrecheckSource
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, ErrNativePrecheckSource
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, nativeControllerLimit+1))
	if err != nil || len(data) > nativeControllerLimit || nativePrecheckExpired(ctx) {
		return 0, ErrNativePrecheckSource
	}
	// Counts only: no rule payload, chains, metadata, node names or arbitrary
	// strings ever cross this boundary. A zero count is not exclusion evidence.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil || envelope == nil {
		return 0, ErrNativePrecheckSource
	}
	raw, ok := envelope[field]
	if !ok || strings.TrimSpace(string(raw)) == "null" {
		return 0, ErrNativePrecheckSource
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return 0, ErrNativePrecheckSource
	}
	for _, entry := range entries {
		if entry == nil {
			return 0, ErrNativePrecheckSource
		}
	}
	if nativePrecheckExpired(ctx) {
		return 0, ErrNativePrecheckSource
	}
	return len(entries), nil
}

// Compare the deadline itself as well as cancellation: timer delivery may lag
// behind a body read or parse that finishes after the monotonic cutoff.
func nativePrecheckExpired(ctx context.Context) bool {
	if ctx.Err() != nil {
		return true
	}
	deadline, ok := ctx.Deadline()
	return ok && !time.Now().Before(deadline)
}
