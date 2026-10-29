package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildProxyProviderInventoryProjectsSafeMetadataOnly(t *testing.T) {
	raw := json.RawMessage(`{
	  "providers": {
	    "Example Provider": {
	      "name": "Example Provider",
	      "type": "Proxy",
	      "vehicleType": "HTTP",
	      "updatedAt": "2026-10-20T08:00:00.123456789Z",
	      "url": "https://subscription.example.com/secret-token",
	      "subscriptionInfo": {"Upload": 1, "Download": 2, "Total": 3},
	      "proxies": [
	        {
	          "name": "Demo HK / 01 #A",
	          "type": "AnyTLS",
	          "alive": true,
	          "udp": true,
	          "server": "203.0.113.10",
	          "port": 443,
	          "password": "super-secret",
	          "uuid": "11111111-2222-3333-4444-555555555555",
	          "sni": "node.example.com",
	          "history": [
	            {"time": "2026-10-20T08:00:00Z", "delay": 0},
	            {"time": "2026-10-20T08:01:00Z", "delay": 123}
	          ]
	        },
	        {"name": "Demo JP 02", "type": "Shadowsocks", "history": [{"time": "2026-10-20T08:02:00Z", "delay": 0}]}
	      ]
	    },
	    "Broken Provider": {
	      "name": "Broken Provider",
	      "type": "Proxy",
	      "vehicleType": "HTTP",
	      "updatedAt": "0001-01-01T00:00:00Z",
	      "proxies": null
	    }
	  }
	}`)

	inventory, err := BuildProxyProviderInventory(raw)
	if err != nil {
		t.Fatalf("BuildProxyProviderInventory: %v", err)
	}
	if len(inventory.Providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(inventory.Providers))
	}
	// Providers are sorted by name so the API is deterministic.
	if inventory.Providers[0].Name != "Broken Provider" || inventory.Providers[1].Name != "Example Provider" {
		t.Fatalf("provider order = %q, %q", inventory.Providers[0].Name, inventory.Providers[1].Name)
	}

	broken := inventory.Providers[0]
	if broken.Status != ProviderStatusUnknown {
		t.Errorf("broken status = %q, want %q", broken.Status, ProviderStatusUnknown)
	}
	if broken.NodeCount != nil {
		t.Errorf("broken node_count = %v, want nil (unknown, never 0)", *broken.NodeCount)
	}
	if broken.Nodes != nil {
		t.Errorf("broken nodes = %#v, want nil", broken.Nodes)
	}
	if broken.UpdatedAt != "" {
		t.Errorf("broken updated_at = %q, want empty for mihomo zero time", broken.UpdatedAt)
	}

	provider := inventory.Providers[1]
	if provider.Status != ProviderStatusOK {
		t.Errorf("status = %q, want %q", provider.Status, ProviderStatusOK)
	}
	if provider.Type != "Proxy" || provider.VehicleType != "HTTP" {
		t.Errorf("provider type/vehicle = %q/%q", provider.Type, provider.VehicleType)
	}
	if provider.UpdatedAt != "2026-10-20T08:00:00Z" {
		t.Errorf("updated_at = %q", provider.UpdatedAt)
	}
	if provider.NodeCount == nil || *provider.NodeCount != 2 {
		t.Errorf("node_count = %v, want 2", provider.NodeCount)
	}
	if len(provider.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(provider.Nodes))
	}

	anytls := provider.Nodes[0]
	wantID := ProviderNodeID("Example Provider", "Demo HK / 01 #A")
	if anytls.ID != wantID {
		t.Errorf("node id = %q, want %q", anytls.ID, wantID)
	}
	if !strings.HasPrefix(anytls.ID, "provider/") {
		t.Errorf("node id = %q, want provider/ prefix to avoid static UUID collisions", anytls.ID)
	}
	if strings.Count(anytls.ID, "/") != 2 {
		t.Errorf("node id = %q, want exactly two separators so names cannot collide", anytls.ID)
	}
	if anytls.Name != "Demo HK / 01 #A" || anytls.Type != "AnyTLS" || anytls.Provider != "Example Provider" {
		t.Errorf("node = %#v", anytls)
	}
	if anytls.Alive == nil || !*anytls.Alive || anytls.UDP == nil || !*anytls.UDP {
		t.Errorf("alive/udp = %v/%v, want true/true", anytls.Alive, anytls.UDP)
	}
	if anytls.Delay == nil || *anytls.Delay != 123 {
		t.Errorf("delay = %v, want 123 (last history entry)", anytls.Delay)
	}

	noDelay := provider.Nodes[1]
	if noDelay.Alive != nil || noDelay.UDP != nil {
		t.Errorf("missing alive/udp should stay nil, got %v/%v", noDelay.Alive, noDelay.UDP)
	}
	if noDelay.Delay != nil {
		t.Errorf("delay = %v, want nil for a failed (0 ms) last test", *noDelay.Delay)
	}

	// The serialized view must not contain any provider URL or node credential.
	encoded, err := json.Marshal(inventory)
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	for _, forbidden := range []string{
		"subscription.example.com", "secret-token", "203.0.113.10", "super-secret",
		"11111111-2222-3333-4444-555555555555", "node.example.com", "subscriptionInfo",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("inventory leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestBuildProxyProviderInventoryExcludesCompatiblePseudoProviders(t *testing.T) {
	// mihomo reports "Compatible" pseudo providers for proxy groups/static config
	// views. They duplicate the static node list and must never be merged into the
	// subscription inventory, while real providers (HTTP/File/Inline) stay included
	// and a real provider without a node list keeps its unknown status.
	raw := json.RawMessage(`{
	  "providers": {
	    "Demo Group View": {
	      "name": "Demo Group View",
	      "type": "Proxy",
	      "vehicleType": "Compatible",
	      "proxies": [{"name": "Demo Static HK 01", "type": "Shadowsocks"}]
	    },
	    "Demo Compatible Lower": {
	      "name": "Demo Compatible Lower",
	      "type": "Proxy",
	      "vehicleType": " compatible ",
	      "proxies": [{"name": "Demo Static JP 02", "type": "VLESS"}]
	    },
	    "Demo HTTP Subscription": {
	      "name": "Demo HTTP Subscription",
	      "type": "Proxy",
	      "vehicleType": "HTTP",
	      "proxies": [{"name": "Demo Subscription HK 01", "type": "AnyTLS"}]
	    },
	    "Demo File Subscription": {
	      "name": "Demo File Subscription",
	      "type": "Proxy",
	      "vehicleType": "File",
	      "proxies": [{"name": "Demo Subscription JP 01", "type": "VMess"}]
	    },
	    "Demo Unknown Subscription": {
	      "name": "Demo Unknown Subscription",
	      "type": "Proxy",
	      "vehicleType": "HTTP",
	      "proxies": null
	    }
	  }
	}`)

	inventory, err := BuildProxyProviderInventory(raw)
	if err != nil {
		t.Fatalf("BuildProxyProviderInventory: %v", err)
	}
	names := make([]string, 0, len(inventory.Providers))
	for _, provider := range inventory.Providers {
		names = append(names, provider.Name)
		if provider.VehicleType == ProviderVehicleCompatible ||
			strings.EqualFold(strings.TrimSpace(provider.VehicleType), ProviderVehicleCompatible) {
			t.Errorf("Compatible pseudo provider %q was included", provider.Name)
		}
	}
	want := []string{
		"Demo File Subscription",
		"Demo HTTP Subscription",
		"Demo Unknown Subscription",
	}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("providers = %v, want %v", names, want)
	}
	for _, provider := range inventory.Providers {
		for _, node := range provider.Nodes {
			if strings.HasPrefix(node.Name, "Demo Static ") {
				t.Errorf("pseudo provider node %q leaked into the inventory", node.Name)
			}
		}
	}
	// A real provider without nodes is still reported, as unknown (never 0).
	unknown := inventory.Providers[2]
	if unknown.Status != ProviderStatusUnknown || unknown.NodeCount != nil || unknown.Nodes != nil {
		t.Errorf("unknown provider = %#v, want unknown status with nil count/nodes", unknown)
	}
}

func TestProviderNodeIDStableAndCollisionFree(t *testing.T) {
	// Same names under different providers must stay distinct.
	a := ProviderNodeID("Provider One", "Shared Node")
	b := ProviderNodeID("Provider Two", "Shared Node")
	if a == b {
		t.Fatalf("ids collide across providers: %q", a)
	}
	// "/" inside either component must not be able to shift the separator.
	c := ProviderNodeID("Provider/One", "Node")
	d := ProviderNodeID("Provider", "One/Node")
	if c == d {
		t.Fatalf("ids collide for names containing /: %q", c)
	}
	// Stable across calls (ids are used as React keys and delay-test state keys).
	if ProviderNodeID("Provider One", "Shared Node") != a {
		t.Fatalf("id is not deterministic")
	}
}

func TestBuildProxyProviderInventoryCollapsesDuplicateNodeNames(t *testing.T) {
	raw := json.RawMessage(`{"providers":{"P1":{"name":"P1","proxies":[
	  {"name":"Same","type":"Shadowsocks"},
	  {"name":"Same","type":"Shadowsocks"}
	]}}}`)
	inventory, err := BuildProxyProviderInventory(raw)
	if err != nil {
		t.Fatalf("BuildProxyProviderInventory: %v", err)
	}
	p := inventory.Providers[0]
	if p.NodeCount == nil || *p.NodeCount != 1 || len(p.Nodes) != 1 {
		t.Fatalf("duplicate node names should collapse to one node, got count=%v nodes=%d", p.NodeCount, len(p.Nodes))
	}
}

func TestBuildProxyProviderInventoryEmptyIsZeroNotUnknown(t *testing.T) {
	raw := json.RawMessage(`{"providers":{"P1":{"name":"P1","proxies":[]}}}`)
	inventory, err := BuildProxyProviderInventory(raw)
	if err != nil {
		t.Fatalf("BuildProxyProviderInventory: %v", err)
	}
	p := inventory.Providers[0]
	if p.Status != ProviderStatusOK {
		t.Errorf("status = %q, want ok for a loaded empty provider", p.Status)
	}
	if p.NodeCount == nil || *p.NodeCount != 0 {
		t.Errorf("node_count = %v, want 0 for a loaded empty provider", p.NodeCount)
	}
	if p.Nodes == nil {
		t.Errorf("nodes = nil, want [] for a loaded empty provider")
	}
}

func TestBuildProxyProviderInventoryRejectsMalformedPayload(t *testing.T) {
	if _, err := BuildProxyProviderInventory(json.RawMessage(`[]`)); err == nil {
		t.Fatal("expected an error for a non-object providers payload")
	}
}
