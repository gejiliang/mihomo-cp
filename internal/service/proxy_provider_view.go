package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"time"
)

// Native proxy-provider (subscription) inventory, as served to the node
// management page. mihomo's GET /providers/proxies returns each provider with
// its nodes; this projection is an allow-list: only display-safe fields are
// copied out (name/type/alive/udp/last delay). Provider URLs and node
// credentials (server, port, password, uuid, sni, …) are never read into the
// view, so they can never leak through the API.

// Provider status values. "unknown" means the provider exists in the running
// mihomo but did not report a node list (not loaded yet / load failed). It is
// deliberately distinct from a loaded provider with zero nodes.
const (
	ProviderStatusOK      = "ok"
	ProviderStatusUnknown = "unknown"
)

// ProxyProviderNodeView is one read-only subscription node.
type ProxyProviderNodeView struct {
	// ID is a provider-scoped, stable identifier (see ProviderNodeID). Static
	// proxies use UUIDs from the database, so the two id spaces never collide.
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Provider string `json:"provider"`
	Alive    *bool  `json:"alive"`
	UDP      *bool  `json:"udp"`
	Delay    *int   `json:"delay"`
}

// ProxyProviderView is one native proxy-provider with its read-only nodes.
type ProxyProviderView struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	VehicleType string `json:"vehicle_type"`
	// Status is ProviderStatusOK or ProviderStatusUnknown.
	Status string `json:"status"`
	// UpdatedAt is the provider's last update timestamp (RFC3339, UTC) or ""
	// when the running mihomo does not report one.
	UpdatedAt string `json:"updated_at"`
	// NodeCount is nil when the node list is unknown; never a fake 0.
	NodeCount *int                    `json:"node_count"`
	Nodes     []ProxyProviderNodeView `json:"nodes"`
}

// ProxyProviderInventory is the response payload of
// GET /api/runtime/proxy-providers.
type ProxyProviderInventory struct {
	Providers []ProxyProviderView `json:"providers"`
}

// ProviderNodeID builds the stable identifier of a subscription node. Both
// components are path-escaped, so the only "/" characters in the result are
// the two separators: distinct (provider, node) pairs always produce distinct
// ids, including names that contain "/", spaces, "+", "#" or non-ASCII text.
func ProviderNodeID(provider, node string) string {
	return "provider/" + url.PathEscape(provider) + "/" + url.PathEscape(node)
}

type proxyProviderRaw struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	VehicleType string          `json:"vehicleType"`
	Proxies     json.RawMessage `json:"proxies"`
	UpdatedAt   string          `json:"updatedAt"`
}

type proxyProviderNodeRaw struct {
	Name    string              `json:"name"`
	Type    string              `json:"type"`
	Alive   *bool               `json:"alive"`
	UDP     *bool               `json:"udp"`
	History []proxyDelayHistory `json:"history"`
}

type proxyDelayHistory struct {
	Time  string `json:"time"`
	Delay int    `json:"delay"`
}

// BuildProxyProviderInventory projects mihomo's raw GET /providers/proxies
// response into the safe node-management view.
func BuildProxyProviderInventory(raw json.RawMessage) (ProxyProviderInventory, error) {
	var envelope struct {
		Providers map[string]json.RawMessage `json:"providers"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ProxyProviderInventory{}, fmt.Errorf("parse proxy providers: %w", err)
	}

	names := make([]string, 0, len(envelope.Providers))
	for name := range envelope.Providers {
		names = append(names, name)
	}
	sort.Strings(names)

	inv := ProxyProviderInventory{Providers: make([]ProxyProviderView, 0, len(names))}
	for _, name := range names {
		inv.Providers = append(inv.Providers, buildProxyProviderView(name, envelope.Providers[name]))
	}
	return inv, nil
}

func buildProxyProviderView(key string, raw json.RawMessage) ProxyProviderView {
	view := ProxyProviderView{
		Name:   key,
		Status: ProviderStatusUnknown,
		Nodes:  nil, // unknown: JSON null, so clients show "unknown" instead of 0
	}

	var p proxyProviderRaw
	if err := json.Unmarshal(raw, &p); err != nil {
		return view
	}
	if p.Name != "" {
		view.Name = p.Name
	}
	view.Type = p.Type
	view.VehicleType = p.VehicleType
	view.UpdatedAt = normalizeProviderUpdatedAt(p.UpdatedAt)

	if len(p.Proxies) == 0 || bytes.Equal(bytes.TrimSpace(p.Proxies), []byte("null")) {
		return view
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(p.Proxies, &raws); err != nil {
		return view
	}

	nodes := make([]ProxyProviderNodeView, 0, len(raws))
	seen := make(map[string]bool, len(raws))
	for _, nodeRaw := range raws {
		var n proxyProviderNodeRaw
		if err := json.Unmarshal(nodeRaw, &n); err != nil {
			continue
		}
		if n.Name == "" {
			continue
		}
		id := ProviderNodeID(view.Name, n.Name)
		if seen[id] {
			// Duplicate names inside one provider share a runtime name, so
			// they are one testable node; keep the first occurrence.
			continue
		}
		seen[id] = true
		nodes = append(nodes, ProxyProviderNodeView{
			ID:       id,
			Name:     n.Name,
			Type:     n.Type,
			Provider: view.Name,
			Alive:    n.Alive,
			UDP:      n.UDP,
			Delay:    lastPositiveDelay(n.History),
		})
	}

	count := len(nodes)
	view.NodeCount = &count
	view.Nodes = nodes
	view.Status = ProviderStatusOK
	return view
}

// lastPositiveDelay returns the most recent successful delay measurement. A
// trailing 0 (failed test) is reported as unknown, never as a fake 0 ms.
func lastPositiveDelay(history []proxyDelayHistory) *int {
	if len(history) == 0 {
		return nil
	}
	delay := history[len(history)-1].Delay
	if delay <= 0 {
		return nil
	}
	return &delay
}

// normalizeProviderUpdatedAt keeps only real RFC3339 timestamps; mihomo's
// zero time and unknown formats collapse to "" (rendered as unknown).
func normalizeProviderUpdatedAt(raw string) string {
	if raw == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil || t.IsZero() || t.Year() <= 1 {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
