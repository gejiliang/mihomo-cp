package service

import (
	"encoding/json"
	"testing"
)

func TestImportConfig(t *testing.T) {
	yamlContent := `
port: 7890
mixed-port: 7891
allow-lan: true
mode: rule
log-level: info
external-controller: 0.0.0.0:9090
proxies:
  - name: jp-01
    type: ss
    server: 1.2.3.4
    port: 443
    cipher: aes-256-gcm
    password: secret
proxy-groups:
  - name: Auto
    type: url-test
    proxies:
      - jp-01
    url: http://www.gstatic.com/generate_204
    interval: 300
rules:
  - DOMAIN-SUFFIX,google.com,Auto
  - MATCH,DIRECT
`

	svc := NewImportService()
	result, err := svc.ParseConfig([]byte(yamlContent))
	if err != nil {
		t.Fatalf("ParseConfig error: %v", err)
	}

	// Verify 1 proxy named jp-01
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}
	if result.Proxies[0].Name != "jp-01" {
		t.Errorf("expected proxy name jp-01, got %q", result.Proxies[0].Name)
	}
	if result.Proxies[0].Type != "ss" {
		t.Errorf("expected proxy type ss, got %q", result.Proxies[0].Type)
	}
	if result.Proxies[0].ID == "" {
		t.Error("expected proxy to have an ID")
	}

	// Verify proxy config contains server field
	var proxyConfig map[string]any
	if err := json.Unmarshal(result.Proxies[0].Config, &proxyConfig); err != nil {
		t.Fatalf("unmarshal proxy config: %v", err)
	}
	if proxyConfig["server"] != "1.2.3.4" {
		t.Errorf("expected server 1.2.3.4, got %v", proxyConfig["server"])
	}

	// Verify 1 group named Auto
	if len(result.ProxyGroups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result.ProxyGroups))
	}
	if result.ProxyGroups[0].Name != "Auto" {
		t.Errorf("expected group name Auto, got %q", result.ProxyGroups[0].Name)
	}
	if result.ProxyGroups[0].Type != "url-test" {
		t.Errorf("expected group type url-test, got %q", result.ProxyGroups[0].Type)
	}

	// Verify group members
	var members []string
	if err := json.Unmarshal(result.ProxyGroups[0].Members, &members); err != nil {
		t.Fatalf("unmarshal members: %v", err)
	}
	if len(members) != 1 || members[0] != "jp-01" {
		t.Errorf("expected members [jp-01], got %v", members)
	}

	// Verify 2 rules (DOMAIN-SUFFIX + MATCH)
	if len(result.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(result.Rules))
	}
	if result.Rules[0].Type != "DOMAIN-SUFFIX" {
		t.Errorf("expected rule[0] type DOMAIN-SUFFIX, got %q", result.Rules[0].Type)
	}
	if result.Rules[0].Payload != "google.com" {
		t.Errorf("expected rule[0] payload google.com, got %q", result.Rules[0].Payload)
	}
	if result.Rules[0].Target != "Auto" {
		t.Errorf("expected rule[0] target Auto, got %q", result.Rules[0].Target)
	}
	if result.Rules[1].Type != "MATCH" {
		t.Errorf("expected rule[1] type MATCH, got %q", result.Rules[1].Type)
	}
	if result.Rules[1].Target != "DIRECT" {
		t.Errorf("expected rule[1] target DIRECT, got %q", result.Rules[1].Target)
	}

	// Verify system config contains port: 7890, allow-lan: true
	if result.SystemConfig == nil {
		t.Fatal("expected system config to be non-nil")
	}
	var sysConfig map[string]any
	if err := json.Unmarshal(result.SystemConfig, &sysConfig); err != nil {
		t.Fatalf("unmarshal system config: %v", err)
	}
	if port, ok := sysConfig["port"]; !ok || port.(float64) != 7890 {
		t.Errorf("expected port 7890 in system config, got %v", sysConfig["port"])
	}
	if allowLan, ok := sysConfig["allow-lan"]; !ok || allowLan.(bool) != true {
		t.Errorf("expected allow-lan true in system config, got %v", sysConfig["allow-lan"])
	}
	// Verify reserved keys are NOT in system config
	for _, key := range []string{"proxies", "proxy-groups", "rules", "rule-providers"} {
		if _, ok := sysConfig[key]; ok {
			t.Errorf("system config should not contain %q", key)
		}
	}
}

func TestImportConfigNoResolve(t *testing.T) {
	yamlContent := `
rules:
  - GEOIP,CN,DIRECT,no-resolve
  - MATCH,DIRECT
`

	svc := NewImportService()
	result, err := svc.ParseConfig([]byte(yamlContent))
	if err != nil {
		t.Fatalf("ParseConfig error: %v", err)
	}

	if len(result.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(result.Rules))
	}

	r := result.Rules[0]
	if r.Type != "GEOIP" || r.Payload != "CN" || r.Target != "DIRECT" {
		t.Errorf("unexpected rule: %+v", r)
	}

	var params map[string]any
	if err := json.Unmarshal(r.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["no-resolve"] != true {
		t.Errorf("expected no-resolve true, got %v", params["no-resolve"])
	}
}

func TestImportEmptyConfig(t *testing.T) {
	yamlContent := `port: 7890`

	svc := NewImportService()
	result, err := svc.ParseConfig([]byte(yamlContent))
	if err != nil {
		t.Fatalf("ParseConfig error: %v", err)
	}

	if len(result.Proxies) != 0 {
		t.Errorf("expected 0 proxies")
	}
	if len(result.ProxyGroups) != 0 {
		t.Errorf("expected 0 groups")
	}
	if len(result.Rules) != 0 {
		t.Errorf("expected 0 rules")
	}
}
