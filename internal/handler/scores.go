package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
)

// ScoresHandler exposes the node-score list and manual refresh endpoints.
type ScoresHandler struct {
	scores *store.NodeScoreStore
	svc    *service.ScoringService
}

// NewScoresHandler creates the handler.
func NewScoresHandler(scores *store.NodeScoreStore, svc *service.ScoringService) *ScoresHandler {
	return &ScoresHandler{scores: scores, svc: svc}
}

// ScoreView is the API projection of a node score row (contract:
// docs/subscription-score-api.md). Unknown stays null; scores from the two
// public sources are never merged.
type ScoreView struct {
	ID           string  `json:"id"`
	NodeName     string  `json:"node_name"`
	NodeKind     string  `json:"node_kind"` // static | provider
	ProviderName *string `json:"provider_name"`
	Status       string  `json:"status"` // pending | checking | ok | error | skipped
	Error        string  `json:"error"`
	ErrorCode    string  `json:"error_code"`

	ExitIP       string  `json:"exit_ip"`
	ExitIPCIDR   *string `json:"exit_ip_cidr"`
	ExitIPSource string  `json:"exit_ip_source"`

	TrustScore  *int   `json:"trust_score"` // 0..100, higher = more trusted; null = unknown
	ScoreSource string `json:"score_source"`
	ScoreURL    string `json:"score_url"`
	DetailURL   string `json:"detail_url"`

	QueriedIP  string `json:"queried_ip"`
	MatchedIP  string `json:"matched_ip"`
	IPMismatch bool   `json:"ip_mismatch"`
	RangeOnly  bool   `json:"range_only"`

	Country       string          `json:"country"`
	CountryCode   string          `json:"country_code"`
	ASN           *int64          `json:"asn"`
	ASNOrg        string          `json:"asn_org"`
	CompanyType   string          `json:"company_type"`
	IsResidential *bool           `json:"is_residential"`
	IsDatacenter  *bool           `json:"is_datacenter"`
	IsVPN         *bool           `json:"is_vpn"`
	IsProxy       *bool           `json:"is_proxy"`
	IsTor         *bool           `json:"is_tor"`
	AbuserScore   string          `json:"abuser_score"`
	AIVerdict     json.RawMessage `json:"ai_verdict"`

	Sources []string `json:"sources"`

	IPInfo json.RawMessage `json:"ip_info,omitempty"`

	CheckedAt     *string `json:"checked_at"`
	LastSuccessAt *string `json:"last_success_at"`
	NextDue       *string `json:"next_due"`
	UpdatedAt     string  `json:"updated_at"`
}

// ipInfoDoc mirrors the stored ip_info document.
type ipInfoDoc struct {
	Claude *service.ClaudeTrustRisk `json:"claude"`
	Lookup struct {
		IP            string          `json:"ip"`
		CIDR          string          `json:"cidr"`
		Country       string          `json:"country"`
		CountryCode   string          `json:"countryCode"`
		ASN           json.Number     `json:"asn"`
		ASNOrg        string          `json:"asOrganization"`
		CompanyType   string          `json:"company_type"`
		IsResidential *bool           `json:"isResidential"`
		IsDatacenter  *bool           `json:"is_datacenter"`
		IsVPN         *bool           `json:"is_vpn"`
		IsProxy       *bool           `json:"is_proxy"`
		IsTor         *bool           `json:"is_tor"`
		AbuserScore   string          `json:"abuser_score"`
		AIVerdict     json.RawMessage `json:"ai_verdict"`
	} `json:"lookup"`
}

// statusForAPI maps internal statuses to the contract's enum.
func statusForAPI(s string) string {
	switch s {
	case model.ScoreStatusTesting:
		return "checking"
	case model.ScoreStatusFailed:
		return "error"
	default:
		return s // pending | ok
	}
}

// toScoreView projects a stored row into the contract shape.
func toScoreView(row *model.NodeScore) *ScoreView {
	v := &ScoreView{
		ID:          row.NodeKey,
		NodeName:    row.NodeName,
		NodeKind:    row.Source,
		Status:      statusForAPI(row.Status),
		Error:       row.LastError,
		ErrorCode:   row.ErrorCode,
		ExitIP:      row.EgressIP,
		TrustScore:  row.ClaudeScore,
		ScoreSource: "netcoffee-claude",
		ScoreURL:    "https://ip.net.coffee/claude/",
		IPInfo:      row.IPInfo.Bytes,
		UpdatedAt:   row.UpdatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
	}
	if row.ProviderName != "" {
		p := row.ProviderName
		v.ProviderName = &p
	}
	if row.EgressIP != "" {
		v.DetailURL = "https://ip.net.coffee/ip/" + row.EgressIP
	}
	switch row.ClaudeIPMatch {
	case model.IPMatchCIDR:
		v.RangeOnly = true
	case model.IPMatchMismatch:
		v.IPMismatch = true
	}
	v.MatchedIP = row.ClaudeResponseIP
	v.QueriedIP = row.EgressIP
	if !row.CheckedAt.IsZero() {
		v.CheckedAt = rfc3339(row.CheckedAt.Time)
	}
	if !row.LastSuccessAt.IsZero() {
		v.LastSuccessAt = rfc3339(row.LastSuccessAt.Time)
	}
	if !row.NextDue.IsZero() {
		v.NextDue = rfc3339(row.NextDue.Time)
	}

	var doc ipInfoDoc
	if len(row.IPInfo.Bytes) > 0 {
		if err := json.Unmarshal(row.IPInfo.Bytes, &doc); err != nil {
			doc = ipInfoDoc{}
		}
	}
	if doc.Claude != nil {
		v.QueriedIP = doc.Claude.QueryIP
		v.MatchedIP = doc.Claude.ResponseIP
		v.IPMismatch = doc.Claude.IPMatch == model.IPMatchMismatch
		v.RangeOnly = doc.Claude.IPMatch == model.IPMatchCIDR
		v.Country = doc.Claude.Country
		v.CountryCode = doc.Claude.CountryCode
		if doc.Claude.ASN != 0 {
			a := doc.Claude.ASN
			v.ASN = &a
		}
		v.ASNOrg = doc.Claude.ASNOrg
		v.IsDatacenter = doc.Claude.IsDatacenter
		v.IsResidential = doc.Claude.IsResidential
		v.IsVPN = doc.Claude.IsVPN
		v.IsProxy = doc.Claude.IsProxy
		v.IsTor = doc.Claude.IsTor
		v.AbuserScore = doc.Claude.AbuserScore
		if doc.Claude.CIDR != "" {
			c := doc.Claude.CIDR
			v.ExitIPCIDR = &c
		}
	}
	if doc.Lookup.CIDR != "" && v.ExitIPCIDR == nil {
		c := doc.Lookup.CIDR
		v.ExitIPCIDR = &c
	}
	if v.Country == "" {
		v.Country = doc.Lookup.Country
	}
	if v.CountryCode == "" {
		v.CountryCode = doc.Lookup.CountryCode
	}
	if v.ASN == nil {
		if a, err := doc.Lookup.ASN.Int64(); err == nil && a != 0 {
			v.ASN = &a
		}
	}
	if v.ASNOrg == "" {
		v.ASNOrg = doc.Lookup.ASNOrg
	}
	if v.CompanyType == "" {
		v.CompanyType = doc.Lookup.CompanyType
	}
	if v.IsResidential == nil {
		v.IsResidential = doc.Lookup.IsResidential
	}
	if v.IsDatacenter == nil {
		v.IsDatacenter = doc.Lookup.IsDatacenter
	}
	if v.IsVPN == nil {
		v.IsVPN = doc.Lookup.IsVPN
	}
	if v.IsProxy == nil {
		v.IsProxy = doc.Lookup.IsProxy
	}
	if v.IsTor == nil {
		v.IsTor = doc.Lookup.IsTor
	}
	if v.AbuserScore == "" {
		v.AbuserScore = doc.Lookup.AbuserScore
	}
	if len(doc.Lookup.AIVerdict) > 0 {
		v.AIVerdict = doc.Lookup.AIVerdict
	}

	// Which public sources answered for this row.
	var sources []string
	if doc.Claude != nil {
		sources = append(sources, "netcoffee-claude")
	}
	if len(row.IPInfo.Bytes) > 0 && (doc.Lookup.IP != "" || (row.LookupMatch != model.IPMatchUnknown && row.LookupMatch != "")) {
		sources = append(sources, "netcoffee-ip")
	}
	if sources == nil {
		sources = []string{}
	}
	v.Sources = sources
	return v
}

// List handles GET /api/node-scores
func (h *ScoresHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.scores.List()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	views := make([]*ScoreView, 0, len(rows))
	for i := range rows {
		views = append(views, toScoreView(&rows[i]))
	}
	JSON(w, 200, views)
}

// Get handles GET /api/node-scores/{id}
func (h *ScoresHandler) Get(w http.ResponseWriter, r *http.Request) {
	row, err := h.scores.Get(r.PathValue("id"))
	if err != nil {
		Error(w, 404, "not_found", "node score not found")
		return
	}
	JSON(w, 200, toScoreView(row))
}

// RefreshOne handles POST /api/node-scores/{id}/refresh
func (h *ScoresHandler) RefreshOne(w http.ResponseWriter, r *http.Request) {
	queued, running, skipped, err := h.svc.RefreshNodes([]string{r.PathValue("id")}, false)
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	if queued == 0 && skipped > 0 {
		Error(w, 404, "not_found", "node score not found")
		return
	}
	JSON(w, 200, map[string]any{"queued": queued, "running": running, "skipped": skipped})
}

// RefreshBatch handles POST /api/node-scores/refresh
func (h *ScoresHandler) RefreshBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
		All bool     `json:"all"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	if !req.All && len(req.IDs) == 0 {
		Error(w, 400, "bad_request", "ids or all is required")
		return
	}
	queued, running, skipped, err := h.svc.RefreshNodes(req.IDs, req.All)
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]any{"queued": queued, "running": running, "skipped": skipped})
}
