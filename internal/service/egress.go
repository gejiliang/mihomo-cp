package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"gopkg.in/yaml.v3"
)

// IsolationConfig carries the generic underlay settings for the throwaway
// scoring instance. Operators set deployment-specific values (physical egress
// interface, explicit DNS upstreams) via the settings API; empty fields fall
// back to safe defaults. Deployment-specific interface names/IPs never appear
// in public code, tests or docs.
type IsolationConfig struct {
	// Interface is the physical underlay interface the isolated instance's
	// sockets bind to (mihomo interface-name). Empty = not bound. On hosts
	// with production TUN/fake-ip this is what keeps child sockets and node
	// server resolution on the physical underlay instead of the tunnel.
	Interface string
	// DNSServers are explicit mihomo nameserver entries used by the isolated
	// instance. Empty = public DoH-by-IP defaults. The host system resolver
	// is never relied upon (it may be fake-ip hijacked).
	DNSServers []string
}

// defaultIsolationDNS returns public, IP-literal DoH upstreams so resolution
// neither depends on the (possibly fake-ip hijacked) system resolver nor on
// any deployment-internal DNS server.
func defaultIsolationDNS() []string {
	return []string{
		"https://223.5.5.5/dns-query",
		"https://1.1.1.1/dns-query",
	}
}

// isolationSettings loads the operator-configured underlay from app settings.
func isolationSettings(st *model.AppSettings) IsolationConfig {
	cfg := IsolationConfig{}
	if st != nil {
		cfg.Interface = st.IsolationInterface
		for _, s := range splitNonEmpty(st.IsolationDNS, ',') {
			cfg.DNSServers = append(cfg.DNSServers, s)
		}
	}
	if len(cfg.DNSServers) == 0 {
		cfg.DNSServers = defaultIsolationDNS()
	}
	return cfg
}

func splitNonEmpty(s string, sep byte) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == sep {
			if v := trimSpace(s[start:i]); v != "" {
				out = append(out, v)
			}
			start = i + 1
		}
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

// IsolationTopology is the split node/group materialization for one isolated
// test. Mihomo groups belong under proxy-groups — putting them into proxies
// yields an invalid config.
type IsolationTopology struct {
	Nodes  []map[string]any
	Groups []map[string]any
	Target string
}

// Fingerprint returns a stable hash of the topology's effective content.
func (t *IsolationTopology) Fingerprint() string {
	h := sha256.New()
	data, _ := yaml.Marshal(map[string]any{"nodes": t.Nodes, "groups": t.Groups})
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// EgressSession is a live, loopback-only transport bound to exactly one node
// under test. Close kills the isolated process and removes the temp dir.
type EgressSession struct {
	Client *http.Client
	Close  func()
}

// MihomoIsolation runs throwaway mihomo instances for real full-node egress
// tests. Each instance:
//   - gets a fresh private temp dir and a generated config containing only
//     the nodes/groups under test;
//   - binds its mixed port to 127.0.0.1 only (allow-lan false);
//   - has NO TUN, NO DNS listener/hijack and NO external controller;
//   - uses explicit DNS upstreams and, when configured, binds its sockets to
//     the physical underlay interface (see IsolationConfig) so child traffic
//     and node-server resolution are not captured by a production TUN or
//     fake-ip DNS;
//   - routes everything to the target via a single MATCH rule;
//   - never touches the production instance's mode, selectors or config.
//
// Note: the absence of TUN in the child config alone does NOT prove physical
// isolation on a TUN-enabled gateway — that is what Interface is for.
type MihomoIsolation struct {
	Binary string
}

// NewMihomoIsolation creates the runner. Binary may be empty in tests that
// replace Probe with a fake.
func NewMihomoIsolation(binary string) *MihomoIsolation {
	return &MihomoIsolation{Binary: binary}
}

// providerEntry is one native proxy-provider from the system config draft.
type providerEntry struct {
	Name     string
	Type     string
	URL      string
	Path     string
	Interval int
	Override map[string]any
	Raw      map[string]any
}

// parseProviderEntry extracts a providerEntry from a system-config
// proxy-providers map value.
func parseProviderEntry(name string, raw map[string]any) providerEntry {
	e := providerEntry{Name: name, Raw: raw}
	e.Type, _ = raw["type"].(string)
	e.URL, _ = raw["url"].(string)
	e.Path, _ = raw["path"].(string)
	if iv, ok := raw["interval"].(int); ok {
		e.Interval = iv
	} else if fv, ok := raw["interval"].(float64); ok {
		e.Interval = int(fv)
	}
	if ov, ok := raw["override"].(map[string]any); ok {
		e.Override = ov
	}
	return e
}

// runtimePrefix returns the configured override name prefix.
func (e providerEntry) runtimePrefix() string {
	if e.Override == nil {
		return ""
	}
	p, _ := e.Override["additional-prefix"].(string)
	return p
}

// runtimeSuffix returns the configured override name suffix.
func (e providerEntry) runtimeSuffix() string {
	if e.Override == nil {
		return ""
	}
	s, _ := e.Override["additional-suffix"].(string)
	return s
}

// RuntimeName maps a cache-file node name to its runtime (display) name.
func (e providerEntry) RuntimeName(cacheName string) string {
	return e.runtimePrefix() + cacheName + e.runtimeSuffix()
}

// CacheName maps a runtime name back to the cache-file node name.
func (e providerEntry) CacheName(runtimeName string) (string, bool) {
	prefix, suffix := e.runtimePrefix(), e.runtimeSuffix()
	if prefix == "" && suffix == "" {
		return runtimeName, true
	}
	if prefix != "" {
		if len(runtimeName) < len(prefix) || runtimeName[:len(prefix)] != prefix {
			return "", false
		}
		runtimeName = runtimeName[len(prefix):]
	}
	if suffix != "" {
		if len(runtimeName) < len(suffix) || runtimeName[len(runtimeName)-len(suffix):] != suffix {
			return "", false
		}
		runtimeName = runtimeName[:len(runtimeName)-len(suffix)]
	}
	return runtimeName, true
}

// ApplyOverride returns the cache node with the provider's override fields
// applied on top. The node's own name is preserved (runtime mapping is the
// separate RuntimeName/CacheName concern) and override keys never touch it.
// This makes the isolated node behave like the runtime node, including
// injected dialer-proxy, udp, skip-cert-verify etc.
func (e providerEntry) ApplyOverride(node map[string]any) map[string]any {
	out := make(map[string]any, len(node)+len(e.Override))
	for k, v := range node {
		out[k] = v
	}
	for k, v := range e.Override {
		if k == "additional-prefix" || k == "additional-suffix" || k == "name" {
			continue
		}
		out[k] = v
	}
	return out
}

// LoadProviderNodes reads the provider's cache file ONCE and returns the raw
// Clash proxies list. The path is taken verbatim from the provider entry
// (system config is the source of truth) and resolved relative to the mihomo
// working directory. The subscription URL is never re-downloaded here.
func (m *MihomoIsolation) LoadProviderNodes(e providerEntry, mihomoDir string) ([]map[string]any, error) {
	if e.Path == "" {
		return nil, fmt.Errorf("provider %q has no cache path configured", e.Name)
	}
	path := e.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(mihomoDir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read provider cache for %q: %w", e.Name, err)
	}
	var doc struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse provider cache for %q: %w", e.Name, err)
	}
	if len(doc.Proxies) == 0 {
		return nil, fmt.Errorf("provider cache for %q contains no proxies", e.Name)
	}
	return doc.Proxies, nil
}

// buildIsolationConfig renders the throwaway mihomo config. Pure function —
// unit-tested to guarantee loopback binding, no TUN/DNS-listener, explicit
// nameservers and the node/group split.
func buildIsolationConfig(topo *IsolationTopology, port int, iso IsolationConfig) []byte {
	cfg := map[string]any{
		"mixed-port":   port,
		"bind-address": "127.0.0.1",
		"allow-lan":    false,
		"mode":         "rule",
		"log-level":    "warning",
		"proxies":      topo.Nodes,
		"rules":        []string{"MATCH," + topo.Target},
	}
	if len(topo.Groups) > 0 {
		cfg["proxy-groups"] = topo.Groups
	}
	if iso.Interface != "" {
		cfg["interface-name"] = iso.Interface
	}
	cfg["dns"] = map[string]any{
		"enable":        true,
		"enhanced-mode": "redir-host", // never fake-ip in the isolated instance
		"nameserver":    iso.DNSServers,
	}
	out, _ := yaml.Marshal(cfg)
	return out
}

// pickFreePort asks the kernel for a free loopback TCP port.
func pickFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// Probe spins up an isolated mihomo for the topology and returns an HTTP
// client proxied through its loopback mixed port.
func (m *MihomoIsolation) Probe(ctx context.Context, topo *IsolationTopology, iso IsolationConfig) (*EgressSession, error) {
	const startTimeout = StartupTimeout
	if m.Binary == "" {
		return nil, fmt.Errorf("mihomo binary not configured")
	}
	port, err := pickFreePort()
	if err != nil {
		return nil, fmt.Errorf("allocate loopback port: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "mihomo-egress-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, buildIsolationConfig(topo, port, iso), 0o600); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("write temp config: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, startTimeout+30*time.Second)
	cmd := exec.CommandContext(runCtx, m.Binary, "-d", tmpDir, "-f", cfgPath)
	var output truncatedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		cancel()
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("start isolated mihomo: %w", err)
	}

	closed := false
	closeFn := func() {
		if closed {
			return
		}
		closed = true
		cancel()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		os.RemoveAll(tmpDir)
	}

	// Wait for the mixed port to accept connections, or the process to die.
	deadline := time.Now().Add(startTimeout)
	addr := net.JoinHostPort("127.0.0.1", fmt.Sprint(port))
	for {
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			closeFn()
			return nil, fmt.Errorf("isolated mihomo exited during startup: %s", output.String())
		}
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			closeFn()
			return nil, fmt.Errorf("isolated mihomo did not open its loopback port in %s", startTimeout)
		}
		select {
		case <-ctx.Done():
			closeFn()
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	client := &http.Client{
		Timeout: 25 * time.Second,
		Transport: &http.Transport{
			Proxy:               http.ProxyURL(mustParseURL(fmt.Sprintf("http://127.0.0.1:%d", port))),
			DialContext:         (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
			TLSHandshakeTimeout: 15 * time.Second,
		},
	}
	return &EgressSession{Client: client, Close: closeFn}, nil
}

// truncatedBuffer keeps the tail of process output for error reporting.
type truncatedBuffer struct {
	buf []byte
}

func (b *truncatedBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	const max = 4096
	if len(b.buf) > max {
		b.buf = b.buf[len(b.buf)-max:]
	}
	return len(p), nil
}

func (b *truncatedBuffer) String() string { return string(b.buf) }

func mustParseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		return &url.URL{Scheme: "http", Host: s}
	}
	return u
}

// ResolveDialerDeps returns the full transitive topology for a static proxy:
// the target proxy, its dialer-proxy chain and any (usable) groups involved.
// Groups land in Groups (proxy-groups), never in Nodes. Groups relying on
// proxy-providers (`use`) cannot be materialized in the isolated instance.
func ResolveDialerDeps(targetName string, proxies []*model.Proxy, groups []*model.ProxyGroup) (*IsolationTopology, error) {
	proxyByName := make(map[string]map[string]any, len(proxies))
	for _, p := range proxies {
		var m map[string]any
		if err := json.Unmarshal(p.Config, &m); err != nil {
			continue
		}
		m["name"] = p.Name
		m["type"] = p.Type
		proxyByName[p.Name] = m
	}
	groupByName := make(map[string]*model.ProxyGroup, len(groups))
	for _, g := range groups {
		groupByName[g.Name] = g
	}

	topo := &IsolationTopology{Target: targetName}
	added := map[string]bool{}

	var addProxy func(name string, depth int) error
	var addGroup func(name string, depth int) error
	addGroup = func(name string, depth int) error {
		if added[name] {
			return nil
		}
		g, ok := groupByName[name]
		if !ok {
			return nil // unresolved member; mihomo validation will surface it
		}
		var cfgExtra map[string]any
		_ = json.Unmarshal(g.Config, &cfgExtra)
		if _, has := cfgExtra["use"]; has {
			return fmt.Errorf("group %q relies on a proxy-provider and cannot be used for isolated testing", name)
		}
		added[name] = true
		var members []string
		_ = json.Unmarshal(g.Members, &members)
		for _, m := range members {
			if _, isProxy := proxyByName[m]; isProxy {
				if err := addProxy(m, depth+1); err != nil {
					return err
				}
			} else if _, isGroup := groupByName[m]; isGroup {
				if err := addGroup(m, depth+1); err != nil {
					return err
				}
			}
		}
		gmap := map[string]any{"name": g.Name, "type": g.Type, "proxies": members}
		for k, v := range cfgExtra {
			if k == "use" {
				continue
			}
			gmap[k] = v
		}
		topo.Groups = append(topo.Groups, gmap)
		return nil
	}
	addProxy = func(name string, depth int) error {
		if depth > 32 || added[name] {
			return nil
		}
		if _, ok := proxyByName[name]; ok {
			added[name] = true
			p := proxyByName[name]
			topo.Nodes = append(topo.Nodes, p)
			if dp, _ := p["dialer-proxy"].(string); dp != "" {
				if _, isGroup := groupByName[dp]; isGroup {
					return addGroup(dp, depth+1)
				}
				return addProxy(dp, depth+1)
			}
			return nil
		}
		if _, isGroup := groupByName[name]; isGroup {
			return addGroup(name, depth+1)
		}
		return nil
	}
	if err := addProxy(targetName, 0); err != nil {
		return nil, err
	}
	if len(topo.Nodes) == 0 && len(topo.Groups) == 0 {
		return nil, fmt.Errorf("proxy %q not found", targetName)
	}
	return topo, nil
}

// NodeFingerprint hashes one node map for inventory change detection.
func NodeFingerprint(node map[string]any) string {
	data, _ := yaml.Marshal(node)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16]
}
