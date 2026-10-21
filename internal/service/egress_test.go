package service

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/store"
	"gopkg.in/yaml.v3"
)

func egressTestTopology() *IsolationTopology {
	return &IsolationTopology{
		Target: "US-edge-01",
		Nodes: []map[string]any{
			{
				"name": "US-edge-01", "type": "ss", "server": "203.0.113.10",
				"port": 8388, "cipher": "aes-128-gcm", "password": "example-pass",
				"dialer-proxy": "relay-group",
			},
			{
				"name": "HK-relay-hop", "type": "ss", "server": "198.51.100.10",
				"port": 8388, "cipher": "aes-128-gcm", "password": "example-pass",
				"udp": true,
			},
		},
		Groups: []map[string]any{
			{"name": "relay-group", "type": "select", "proxies": []string{"HK-relay-hop"}},
		},
	}
}

func TestBuildIsolationConfigLoopbackNoTunAndGroupSplit(t *testing.T) {
	cfg := buildIsolationConfig(egressTestTopology(), 34567, IsolationConfig{
		Interface:  "test-underlay0",
		DNSServers: []string{"https://223.5.5.5/dns-query", "https://1.1.1.1/dns-query"},
	})
	var m map[string]any
	if err := yaml.Unmarshal(cfg, &m); err != nil {
		t.Fatalf("unmarshal generated config: %v\n%s", err, cfg)
	}

	if m["bind-address"] != "127.0.0.1" {
		t.Errorf("bind-address = %v, want 127.0.0.1", m["bind-address"])
	}
	if m["allow-lan"] != false {
		t.Errorf("allow-lan = %v, want false", m["allow-lan"])
	}
	if m["mode"] != "rule" {
		t.Errorf("mode = %v, want rule (never global/direct mutation)", m["mode"])
	}
	if m["mixed-port"] != 34567 {
		t.Errorf("mixed-port = %v, want 34567", m["mixed-port"])
	}
	if _, ok := m["tun"]; ok {
		t.Error("isolated config must not contain a TUN block")
	}
	if _, ok := m["external-controller"]; ok {
		t.Error("isolated config must not expose an external controller")
	}
	if _, ok := m["secret"]; ok {
		t.Error("isolated config must not define an API secret")
	}
	if m["interface-name"] != "test-underlay0" {
		t.Errorf("interface-name = %v, want configured underlay", m["interface-name"])
	}

	dns, ok := m["dns"].(map[string]any)
	if !ok {
		t.Fatalf("dns block missing: %#v", m["dns"])
	}
	if dns["enable"] != true || dns["enhanced-mode"] != "redir-host" {
		t.Errorf("dns = %#v, want enabled redir-host (never fake-ip)", dns)
	}
	nameservers, _ := dns["nameserver"].([]any)
	if len(nameservers) != 2 || nameservers[0] != "https://223.5.5.5/dns-query" {
		t.Errorf("nameserver = %#v, want explicit configured servers", dns["nameserver"])
	}

	proxies, _ := m["proxies"].([]any)
	if len(proxies) != 2 {
		t.Errorf("proxies length = %d, want 2", len(proxies))
	}
	for _, p := range proxies {
		pm, _ := p.(map[string]any)
		if pm["name"] == "relay-group" {
			t.Error("proxy group must never be materialized under proxies")
		}
	}
	groups, ok := m["proxy-groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("proxy-groups = %#v, want one group", m["proxy-groups"])
	}
	gm, _ := groups[0].(map[string]any)
	if gm["name"] != "relay-group" || gm["type"] != "select" {
		t.Errorf("group = %#v, want relay-group/select", gm)
	}
	rules, _ := m["rules"].([]any)
	if len(rules) != 1 || rules[0] != "MATCH,US-edge-01" {
		t.Errorf("rules = %#v, want MATCH target only", rules)
	}
}

func TestBuildIsolationConfigWithoutGroups(t *testing.T) {
	topo := &IsolationTopology{
		Target: "Direct-Test",
		Nodes: []map[string]any{
			{"name": "Direct-Test", "type": "ss", "server": "203.0.113.10", "port": 8388,
				"cipher": "aes-128-gcm", "password": "example-pass"},
		},
	}
	cfg := buildIsolationConfig(topo, 45678, IsolationConfig{})
	var m map[string]any
	if err := yaml.Unmarshal(cfg, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["proxy-groups"]; ok {
		t.Error("no groups should be emitted when topology has none")
	}
	if _, ok := m["interface-name"]; ok {
		t.Error("interface-name must be omitted when no underlay is configured")
	}
}

func TestIsolationSettingsConfiguredUnderlay(t *testing.T) {
	got := isolationSettings(&model.AppSettings{
		IsolationInterface: "test-underlay0",
		IsolationDNS:       " https://223.5.5.5/dns-query , tls://1.1.1.1 ",
	})
	if got.Interface != "test-underlay0" {
		t.Errorf("interface = %q, want configured value", got.Interface)
	}
	if len(got.DNSServers) != 2 || got.DNSServers[0] != "https://223.5.5.5/dns-query" || got.DNSServers[1] != "tls://1.1.1.1" {
		t.Errorf("nameservers = %#v, want trimmed configured values", got.DNSServers)
	}

	defaults := isolationSettings(nil)
	if defaults.Interface != "" {
		t.Errorf("default interface = %q, want empty", defaults.Interface)
	}
	if len(defaults.DNSServers) == 0 {
		t.Error("default DNS must not be empty (system resolver is never relied on)")
	}
	if !strings.HasPrefix(defaults.DNSServers[0], "https://") {
		t.Errorf("default DNS = %#v, want DoH IP literals", defaults.DNSServers)
	}
}

func TestResolveDialerDepsFullChainGroups(t *testing.T) {
	proxies := []*model.Proxy{
		{Name: "US-edge-01", Type: "ss", Config: json.RawMessage(`{"server":"203.0.113.10","port":8388,"dialer-proxy":"relay-group"}`)},
		{Name: "HK-relay-hop", Type: "ss", Config: json.RawMessage(`{"server":"198.51.100.10","port":8388,"dialer-proxy":"SG-hop"}`)},
		{Name: "SG-hop", Type: "ss", Config: json.RawMessage(`{"server":"192.0.2.20","port":8388}`)},
	}
	groups := []*model.ProxyGroup{
		{
			Name: "relay-group", Type: "select",
			Config:  json.RawMessage(`{"url":"http://www.gstatic.com/generate_204","interval":300}`),
			Members: json.RawMessage(`["HK-relay-hop"]`),
		},
	}

	topo, err := ResolveDialerDeps("US-edge-01", proxies, groups)
	if err != nil {
		t.Fatalf("ResolveDialerDeps: %v", err)
	}
	if topo.Target != "US-edge-01" {
		t.Errorf("target = %q", topo.Target)
	}
	nodeNames := map[string]bool{}
	for _, n := range topo.Nodes {
		name, _ := n["name"].(string)
		nodeNames[name] = true
	}
	for _, want := range []string{"US-edge-01", "HK-relay-hop", "SG-hop"} {
		if !nodeNames[want] {
			t.Errorf("missing node %q in full dialer chain: %#v", want, nodeNames)
		}
	}
	if len(topo.Groups) != 1 {
		t.Fatalf("groups = %#v, want exactly relay-group", topo.Groups)
	}
	g := topo.Groups[0]
	if g["name"] != "relay-group" || g["type"] != "select" {
		t.Errorf("group metadata lost: %#v", g)
	}
	if fmt.Sprint(g["interval"]) != "300" {
		t.Errorf("group extra config lost: %#v", g)
	}
	if nodeNames["relay-group"] {
		t.Error("group leaked into proxies")
	}

	// The source models must not be mutated while materializing the topology.
	if string(proxies[0].Config) != `{"server":"203.0.113.10","port":8388,"dialer-proxy":"relay-group"}` {
		t.Errorf("source proxy config mutated: %s", proxies[0].Config)
	}
	if string(groups[0].Members) != `["HK-relay-hop"]` {
		t.Errorf("source group members mutated: %s", groups[0].Members)
	}
}

func TestResolveDialerDepsGroupTarget(t *testing.T) {
	proxies := []*model.Proxy{
		{Name: "A", Type: "ss", Config: json.RawMessage(`{"server":"203.0.113.10","port":8388}`)},
	}
	groups := []*model.ProxyGroup{
		{Name: "G", Type: "select", Members: json.RawMessage(`["A"]`), Config: json.RawMessage(`{}`)},
	}
	topo, err := ResolveDialerDeps("G", proxies, groups)
	if err != nil {
		t.Fatalf("ResolveDialerDeps: %v", err)
	}
	if topo.Target != "G" {
		t.Errorf("target = %q, want G", topo.Target)
	}
	if len(topo.Nodes) != 1 || topo.Nodes[0]["name"] != "A" {
		t.Errorf("nodes = %#v, want A", topo.Nodes)
	}
	if len(topo.Groups) != 1 || topo.Groups[0]["name"] != "G" {
		t.Errorf("groups = %#v, want G", topo.Groups)
	}
}

func TestResolveDialerDepsRejectsProviderGroups(t *testing.T) {
	proxies := []*model.Proxy{
		{Name: "A", Type: "ss", Config: json.RawMessage(`{"server":"203.0.113.10","port":8388,"dialer-proxy":"provider-group"}`)},
	}
	groups := []*model.ProxyGroup{
		{
			Name: "provider-group", Type: "select",
			Config:  json.RawMessage(`{"use":["example-provider"]}`),
			Members: json.RawMessage(`["A"]`),
		},
	}
	_, err := ResolveDialerDeps("A", proxies, groups)
	if err == nil {
		t.Fatal("expected error for a group relying on a proxy-provider")
	}
	if !strings.Contains(err.Error(), "proxy-provider") {
		t.Errorf("error = %v, want proxy-provider explanation", err)
	}
}

func TestResolveDialerDepsUnknownTarget(t *testing.T) {
	if _, err := ResolveDialerDeps("missing", nil, nil); err == nil {
		t.Fatal("expected error for missing target")
	}
}

func TestResolveDialerDepsCycleDoesNotHang(t *testing.T) {
	proxies := []*model.Proxy{
		{Name: "A", Type: "ss", Config: json.RawMessage(`{"server":"203.0.113.10","port":8388,"dialer-proxy":"B"}`)},
		{Name: "B", Type: "ss", Config: json.RawMessage(`{"server":"198.51.100.10","port":8388,"dialer-proxy":"A"}`)},
	}
	topo, err := ResolveDialerDeps("A", proxies, nil)
	if err != nil {
		t.Fatalf("cycle should stop safely: %v", err)
	}
	if len(topo.Nodes) != 2 {
		t.Errorf("nodes = %#v, want A and B once each", topo.Nodes)
	}
}

func TestIsolationTopologyFingerprint(t *testing.T) {
	a := &IsolationTopology{
		Target: "T",
		Nodes: []map[string]any{
			{"name": "A", "type": "ss", "server": "203.0.113.10", "port": 8388},
		},
		Groups: []map[string]any{{"name": "G", "type": "select", "proxies": []string{"A"}}},
	}
	b := &IsolationTopology{
		Target: "T",
		Nodes: []map[string]any{
			{"port": 8388, "server": "203.0.113.10", "type": "ss", "name": "A"},
		},
		Groups: []map[string]any{{"proxies": []string{"A"}, "type": "select", "name": "G"}},
	}
	if a.Fingerprint() != b.Fingerprint() {
		t.Errorf("fingerprint should be stable across map insertion order: %s vs %s", a.Fingerprint(), b.Fingerprint())
	}
	b.Nodes[0]["server"] = "198.51.100.10"
	if a.Fingerprint() == b.Fingerprint() {
		t.Error("fingerprint must change when a node changes")
	}
}

func TestProviderEntryRuntimeMappingAndOverride(t *testing.T) {
	e := parseProviderEntry("example-provider", map[string]any{
		"type": "http", "url": "https://sub.example.com/…", "path": "providers/example.yaml",
		"override": map[string]any{
			"additional-prefix": "P-",
			"additional-suffix": "-S",
			"dialer-proxy":      "relay-group",
			"udp":               true,
		},
	})
	if got := e.RuntimeName("HK-01"); got != "P-HK-01-S" {
		t.Errorf("RuntimeName = %q", got)
	}
	if got, ok := e.CacheName("P-HK-01-S"); !ok || got != "HK-01" {
		t.Errorf("CacheName = %q, %v", got, ok)
	}
	if _, ok := e.CacheName("HK-01"); ok {
		t.Error("CacheName should reject names without the configured prefix/suffix")
	}
	out := e.ApplyOverride(map[string]any{
		"name": "HK-01", "type": "ss", "server": "203.0.113.10", "port": 8388,
	})
	if out["dialer-proxy"] != "relay-group" || out["udp"] != true {
		t.Errorf("override not applied: %#v", out)
	}
	if _, ok := out["additional-prefix"]; ok {
		t.Error("name-mapping override must not leak into the node config")
	}
	if out["server"] != "203.0.113.10" {
		t.Errorf("base node fields lost: %#v", out)
	}
}

func TestLoadProviderNodesFromCache(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "providers"), 0o700); err != nil {
		t.Fatal(err)
	}
	cache := `proxies:
  - name: HK-01
    type: ss
    server: 203.0.113.10
    port: 8388
    cipher: aes-128-gcm
    password: example-pass
`
	if err := os.WriteFile(filepath.Join(dir, "providers", "example.yaml"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	e := providerEntry{Name: "example", Path: "providers/example.yaml"}
	nodes, err := (&MihomoIsolation{}).LoadProviderNodes(e, dir)
	if err != nil {
		t.Fatalf("LoadProviderNodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0]["name"] != "HK-01" || nodes[0]["server"] != "203.0.113.10" {
		t.Errorf("nodes = %#v", nodes)
	}
	if _, err := (&MihomoIsolation{}).LoadProviderNodes(providerEntry{Name: "missing", Path: "providers/missing.yaml"}, dir); err == nil {
		t.Error("expected error for a missing provider cache")
	}
}

func TestNeedsUnderlayBindingAndInterfaceSuffix(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "plain", raw: `{}`, want: false},
		{name: "TUN enabled", raw: `{"tun":{"enable":true}}`, want: true},
		{name: "TUN disabled", raw: `{"tun":{"enable":false}}`, want: false},
		{name: "fake-ip DNS", raw: `{"dns":{"enhanced-mode":"fake-ip"}}`, want: true},
		{name: "redir-host DNS", raw: `{"dns":{"enhanced-mode":"redir-host"}}`, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.raw), &cfg); err != nil {
				t.Fatal(err)
			}
			if got := needsUnderlayBinding(cfg); got != tc.want {
				t.Errorf("needsUnderlayBinding = %v, want %v", got, tc.want)
			}
		})
	}

	suffixCases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "one suffix", raw: `{"dns":{"nameserver":["https://223.5.5.5/dns-query#test0"]}}`, want: "test0"},
		{name: "same suffix in both lists", raw: `{"dns":{"nameserver":["https://223.5.5.5/dns-query#test0"],"proxy-server-nameserver":["tls://1.1.1.1#test0"]}}`, want: "test0"},
		{name: "contradictory suffixes", raw: `{"dns":{"nameserver":["https://223.5.5.5/dns-query#test0","tls://1.1.1.1#test1"]}}`, want: ""},
		{name: "no suffix", raw: `{"dns":{"nameserver":["https://223.5.5.5/dns-query"]}}`, want: ""},
	}
	for _, tc := range suffixCases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.raw), &cfg); err != nil {
				t.Fatal(err)
			}
			if got := deriveIfaceSuffix(cfg); got != tc.want {
				t.Errorf("deriveIfaceSuffix = %q, want %q", got, tc.want)
			}
		})
	}
}

func newUnderlayTestService(t *testing.T) (*ScoringService, *store.ConfigStore) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "underlay.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	configs := store.NewConfigStore(db)
	return &ScoringService{configs: configs}, configs
}

func TestUnderlayInterfaceGuard(t *testing.T) {
	svc, configs := newUnderlayTestService(t)

	if got, err := svc.underlayInterface(""); err != nil || got != "" {
		t.Errorf("plain config: got %q, %v; want empty and no error", got, err)
	}

	if err := configs.Update(json.RawMessage(`{"tun":{"enable":true}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.underlayInterface(""); err == nil {
		t.Fatal("TUN-enabled config without an underlay must refuse the probe")
	}
	if got, err := svc.underlayInterface("test-underlay0"); err != nil || got != "test-underlay0" {
		t.Errorf("explicit underlay = %q, %v; want configured value", got, err)
	}

	if _, err := net.InterfaceByName("lo"); err != nil {
		t.Skip("loopback interface not available")
	}
	if err := configs.Update(json.RawMessage(`{"dns":{"enhanced-mode":"fake-ip","nameserver":["https://223.5.5.5/dns-query#lo"]}}`)); err != nil {
		t.Fatal(err)
	}
	if got, err := svc.underlayInterface(""); err != nil || got != "lo" {
		t.Errorf("derived underlay = %q, %v; want lo", got, err)
	}

	if err := configs.Update(json.RawMessage(`{"dns":{"enhanced-mode":"fake-ip","nameserver":["https://223.5.5.5/dns-query#no-such-iface-xyz"]}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.underlayInterface(""); err == nil {
		t.Fatal("non-existent derived interface must refuse the probe")
	}
}

func TestBuildConfigValidatedByLocalMihomo(t *testing.T) {
	bin := os.Getenv("MIHOMO_TEST_BINARY")
	if bin == "" {
		t.Skip("set MIHOMO_TEST_BINARY to the local mihomo binary to validate generated configs")
	}
	dir := t.TempDir()
	cfg := buildIsolationConfig(egressTestTopology(), 34567, IsolationConfig{
		DNSServers: []string{"https://223.5.5.5/dns-query"},
	})
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "-t", "-d", dir, "-f", cfgPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mihomo -t rejected generated isolated config: %v\n%s\nconfig:\n%s", err, out, cfg)
	}
}
