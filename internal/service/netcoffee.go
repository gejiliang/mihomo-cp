package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// NetCoffeeBaseURL is the public base URL of the ip.net.coffee service.
const NetCoffeeBaseURL = "https://ip.net.coffee"

// MaxNetCoffeeResponse caps how much of a response body we read (2 MiB —
// lookup payloads are large but bounded).
const MaxNetCoffeeResponse = 2 << 20

// RateLimitError is returned when the upstream answers 429. RetryAfter is
// the delay requested by the server (Retry-After header), defaulting to 60s.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("upstream rate limited (retry after %s)", e.RetryAfter)
}

// NetCoffeeClient is a read-only adapter for two public ip.net.coffee
// endpoints:
//
//	GET /api/iprisk/{ip}     — Claude-facing risk data incl. trust_score
//	GET /api/ip/lookup/{ip}  — comprehensive IP intelligence
//
// It only calls the documented public JSON APIs; it does not simulate any
// browser-side checks (DNS/STUN/device fingerprinting) and never merges the
// two sources into one score. Every response's IP field is validated against
// the queried IP and the scope recorded (exact vs same-/24|/64 CIDR).
type NetCoffeeClient struct {
	BaseURL string
	HTTP    *http.Client
}

// NewNetCoffeeClient creates a client. The http.Client is supplied by the
// caller — during scoring it is bound to the isolated node under test.
func NewNetCoffeeClient(httpClient *http.Client) *NetCoffeeClient {
	return &NetCoffeeClient{BaseURL: NetCoffeeBaseURL, HTTP: httpClient}
}

// ClaudeTrustRisk is the parsed /api/iprisk/{ip} payload (Claude source).
type ClaudeTrustRisk struct {
	QueryIP    string `json:"query_ip"` // IP we asked about
	ResponseIP string `json:"ip"`       // IP the API says it scored
	IPMatch    string `json:"ip_match"` // exact | cidr | mismatch
	CIDR       string `json:"cidr,omitempty"`
	TrustScore *int   `json:"trust_score,omitempty"`

	ASN         int64  `json:"asn,omitempty"`
	ASNOrg      string `json:"as_organization,omitempty"`
	Country     string `json:"country,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	Region      string `json:"region,omitempty"`
	City        string `json:"city,omitempty"`
	Timezone    string `json:"timezone,omitempty"`
	RDNS        string `json:"rdns,omitempty"`

	IsDatacenter   *bool  `json:"is_datacenter,omitempty"`
	IsResidential  *bool  `json:"is_residential,omitempty"`
	IsBroadcast    *bool  `json:"is_broadcast,omitempty"`
	IsVPN          *bool  `json:"is_vpn,omitempty"`
	IsProxy        *bool  `json:"is_proxy,omitempty"`
	IsTor          *bool  `json:"is_tor,omitempty"`
	IsAbuser       *bool  `json:"is_abuser,omitempty"`
	IsMobile       *bool  `json:"is_mobile,omitempty"`
	IsCrawler      *bool  `json:"is_crawler,omitempty"`
	CompanyType    string `json:"company_type,omitempty"`
	CompanyName    string `json:"company_name,omitempty"`
	DatacenterName string `json:"datacenter_name,omitempty"`
	AbuserScore    string `json:"abuser_score,omitempty"`
	ASNKind        string `json:"asn_kind,omitempty"`
}

// FetchClaudeTrust queries GET /api/iprisk/{ip} through the given HTTP
// client (normally bound to the isolated node under test). trustScore is
// nil when the response carries no usable score — unknown is never
// fabricated as 0.
func (c *NetCoffeeClient) FetchClaudeTrust(ctx context.Context, ip string) (*ClaudeTrustRisk, error) {
	body, err := c.getJSON(ctx, "/api/iprisk/"+url.PathEscape(ip))
	if err != nil {
		return nil, err
	}
	var raw struct {
		IP             string      `json:"ip"`
		CIDR           string      `json:"cidr"`
		TrustScore     json.Number `json:"trust_score"`
		ASN            json.Number `json:"asn"`
		ASNOrg         string      `json:"asOrganization"`
		Country        string      `json:"country"`
		CountryCode    string      `json:"countryCode"`
		Region         string      `json:"region"`
		City           string      `json:"city"`
		Timezone       string      `json:"timezone"`
		RDNS           string      `json:"rdns"`
		IsDatacenter   *bool       `json:"is_datacenter"`
		IsResidential  *bool       `json:"isResidential"`
		IsBroadcast    *bool       `json:"isBroadcast"`
		IsVPN          *bool       `json:"is_vpn"`
		IsProxy        *bool       `json:"is_proxy"`
		IsTor          *bool       `json:"is_tor"`
		IsAbuser       *bool       `json:"is_abuser"`
		IsMobile       *bool       `json:"is_mobile"`
		IsCrawler      *bool       `json:"is_crawler"`
		CompanyType    string      `json:"company_type"`
		CompanyName    string      `json:"company_name"`
		DatacenterName string      `json:"datacenter_name"`
		AbuserScore    string      `json:"abuser_score"`
		ASNKind        string      `json:"asn_kind"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("iprisk: unparsable response: %w", err)
	}

	risk := &ClaudeTrustRisk{
		QueryIP:        ip,
		ResponseIP:     raw.IP,
		IPMatch:        IPMatchScope(ip, raw.IP, raw.CIDR),
		CIDR:           raw.CIDR,
		ASNOrg:         raw.ASNOrg,
		Country:        raw.Country,
		CountryCode:    raw.CountryCode,
		Region:         raw.Region,
		City:           raw.City,
		Timezone:       raw.Timezone,
		RDNS:           raw.RDNS,
		IsDatacenter:   raw.IsDatacenter,
		IsResidential:  raw.IsResidential,
		IsBroadcast:    raw.IsBroadcast,
		IsVPN:          raw.IsVPN,
		IsProxy:        raw.IsProxy,
		IsTor:          raw.IsTor,
		IsAbuser:       raw.IsAbuser,
		IsMobile:       raw.IsMobile,
		IsCrawler:      raw.IsCrawler,
		CompanyType:    raw.CompanyType,
		CompanyName:    raw.CompanyName,
		DatacenterName: raw.DatacenterName,
		AbuserScore:    raw.AbuserScore,
		ASNKind:        raw.ASNKind,
	}
	if v, err := raw.ASN.Int64(); err == nil && v != 0 {
		risk.ASN = v
	}
	if v, err := raw.TrustScore.Int64(); err == nil {
		s := int(v)
		if s >= 0 && s <= 100 {
			risk.TrustScore = &s
		}
	}
	return risk, nil
}

// FetchIPLookup queries GET /api/ip/lookup/{ip} (comprehensive source) and
// returns a trimmed subset of the public payload, plus the response-IP
// validation result. Fields absent upstream stay absent — unknown is never
// invented.
func (c *NetCoffeeClient) FetchIPLookup(ctx context.Context, ip string) (json.RawMessage, string, error) {
	body, err := c.getJSON(ctx, "/api/ip/lookup/"+url.PathEscape(ip))
	if err != nil {
		return nil, model.IPMatchUnknown, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, model.IPMatchUnknown, fmt.Errorf("lookup: unparsable response: %w", err)
	}
	responseIP := jsonString(raw["ip"])
	declaredCIDR := jsonString(raw["cidr"])
	match := IPMatchScope(ip, responseIP, declaredCIDR)

	// Keep only the documented public fields the GUI displays; drop bulky
	// secondary lists (related_domains, histories) to bound storage.
	keep := []string{
		"ip", "cidr", "trust_score", "asn", "asOrganization", "asn_kind", "asn_allocated",
		"country", "countryCode", "registered_country", "registered_country_code",
		"region", "city", "isp", "timezone", "rdns", "rpki_status", "src",
		"is_bogon", "is_datacenter", "isResidential", "isBroadcast", "is_vpn",
		"is_proxy", "is_tor", "is_abuser", "is_mobile", "is_crawler",
		"company_type", "company_name", "datacenter_name", "abuser_score",
		"ai_verdict", "intelligence", "geo_sources",
	}
	subset := make(map[string]json.RawMessage, len(keep)+2)
	for _, k := range keep {
		if v, ok := raw[k]; ok {
			subset[k] = v
		}
	}
	subset["ip_match"] = mustJSON(match)
	subset["query_ip"] = mustJSON(ip)
	out, err := json.Marshal(subset)
	if err != nil {
		return nil, match, fmt.Errorf("lookup: marshal subset: %w", err)
	}
	return out, match, nil
}

// getJSON performs a bounded GET and returns the parsed body. Errors are
// sanitized: upstream URLs (which contain no credentials here but still
// mirror our request path) are never embedded in returned error strings.
func (c *NetCoffeeClient) getJSON(ctx context.Context, path string) ([]byte, error) {
	base := c.BaseURL
	if base == "" {
		base = NetCoffeeBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, fmt.Errorf("net.coffee: bad request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "mihomo-cp-scorer/1.0")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, sanitizeURLError("net.coffee", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxNetCoffeeResponse))
	if err != nil {
		return nil, sanitizeURLError("net.coffee", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("net.coffee %s returned HTTP %d", path[:strings.IndexByte(path, '/')+4], resp.StatusCode)
	}
	return body, nil
}

// parseRetryAfter parses the Retry-After header (seconds or HTTP-date).
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return time.Minute
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(strings.TrimSpace(v)); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return time.Minute
}

// sanitizeURLError strips URLs from network errors so no endpoint/credential
// material can leak into logs, stored error strings, or API responses.
func sanitizeURLError(what string, err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s request failed: %v", what, ue.Err)
	}
	return fmt.Errorf("%s request failed: %v", what, err)
}

// IPMatchScope validates a response IP against the queried IP. A response
// naming a different host in the same /24 (IPv4) or /64 (IPv6) is recorded
// as CIDR-level data — never presented as a precise single-IP score.
func IPMatchScope(queryIP, responseIP, declaredCIDR string) string {
	if responseIP == "" {
		return model.IPMatchUnknown
	}
	q, r := net.ParseIP(queryIP), net.ParseIP(responseIP)
	if q == nil || r == nil {
		return model.IPMatchMismatch
	}
	// Canonical equality: parsed-IP equality, not string comparison.
	if q.Equal(r) {
		return model.IPMatchExact
	}
	// A different IP only counts as CIDR-level (neighbor) data when the
	// vendor's DECLARED CIDR from the same response parses and covers BOTH
	// addresses. Same /24 or /64 alone is not proof; no valid covering CIDR
	// means mismatch. A neighbor match is a reference datapoint about a
	// nearby address — never a precise score for the queried IP and never
	// proof that the whole CIDR shares that score.
	_, cidrNet, err := net.ParseCIDR(strings.TrimSpace(declaredCIDR))
	if err != nil {
		return model.IPMatchMismatch
	}
	if cidrNet.Contains(q) && cidrNet.Contains(r) {
		return model.IPMatchCIDR
	}
	return model.IPMatchMismatch
}

func jsonString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
