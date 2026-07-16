package service

import (
	"encoding/json"
	"testing"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"gopkg.in/yaml.v3"
)

func TestRenderProxies(t *testing.T) {
	svc := NewConfigService()

	proxies := []model.Proxy{
		{
			Name:   "jp-01",
			Type:   "ss",
			Config: json.RawMessage(`{"server":"1.2.3.4","port":443,"cipher":"aes-256-gcm","password":"secret"}`),
		},
	}

	out, err := svc.RenderYAML(nil, proxies, nil, nil, nil)
	if err != nil {
		t.Fatalf("RenderYAML error: %v", err)
	}

	var result map[string]any
	if err := yaml.Unmarshal(out, &result); err != nil {
		t.Fatalf("yaml unmarshal error: %v", err)
	}

	proxyList, ok := result["proxies"].([]any)
	if !ok || len(proxyList) != 1 {
		t.Fatalf("expected 1 proxy, got %v", result["proxies"])
	}

	p := proxyList[0].(map[string]any)
	if p["name"] != "jp-01" {
		t.Errorf("expected name jp-01, got %v", p["name"])
	}
	if p["type"] != "ss" {
		t.Errorf("expected type ss, got %v", p["type"])
	}
	if p["server"] != "1.2.3.4" {
		t.Errorf("expected server 1.2.3.4, got %v", p["server"])
	}
}

func TestRenderRules(t *testing.T) {
	svc := NewConfigService()

	rules := []model.Rule{
		{Type: "DOMAIN-SUFFIX", Payload: "google.com", Target: "Proxy"},
		{Type: "MATCH", Target: "DIRECT"},
	}

	out, err := svc.RenderYAML(nil, nil, nil, rules, nil)
	if err != nil {
		t.Fatalf("RenderYAML error: %v", err)
	}

	var result map[string]any
	if err := yaml.Unmarshal(out, &result); err != nil {
		t.Fatalf("yaml unmarshal error: %v", err)
	}

	ruleList, ok := result["rules"].([]any)
	if !ok || len(ruleList) != 2 {
		t.Fatalf("expected 2 rules, got %v", result["rules"])
	}

	if ruleList[0] != "DOMAIN-SUFFIX,google.com,Proxy" {
		t.Errorf("unexpected rule[0]: %v", ruleList[0])
	}
	if ruleList[1] != "MATCH,DIRECT" {
		t.Errorf("unexpected rule[1]: %v", ruleList[1])
	}
}

func TestRenderFullConfig(t *testing.T) {
	svc := NewConfigService()

	sysConfig := json.RawMessage(`{"port":7890,"mixed-port":7891,"mode":"rule"}`)

	proxies := []model.Proxy{
		{
			Name:   "jp-01",
			Type:   "ss",
			Config: json.RawMessage(`{"server":"1.2.3.4","port":443}`),
		},
	}

	groups := []model.ProxyGroup{
		{
			Name:    "Auto",
			Type:    "url-test",
			Members: json.RawMessage(`["jp-01"]`),
			Config:  json.RawMessage(`{"url":"http://www.gstatic.com/generate_204","interval":300}`),
		},
	}

	rules := []model.Rule{
		{Type: "MATCH", Target: "DIRECT"},
	}

	out, err := svc.RenderYAML(sysConfig, proxies, groups, rules, nil)
	if err != nil {
		t.Fatalf("RenderYAML error: %v", err)
	}

	var result map[string]any
	if err := yaml.Unmarshal(out, &result); err != nil {
		t.Fatalf("yaml unmarshal error: %v", err)
	}

	// Check port from system config
	port, ok := result["port"]
	if !ok {
		t.Error("expected port in config")
	}
	if port.(int) != 7890 {
		t.Errorf("expected port 7890, got %v", port)
	}

	// Check proxy
	proxyList := result["proxies"].([]any)
	if len(proxyList) != 1 {
		t.Fatalf("expected 1 proxy")
	}

	// Check group
	groupList := result["proxy-groups"].([]any)
	if len(groupList) != 1 {
		t.Fatalf("expected 1 group")
	}
	g := groupList[0].(map[string]any)
	if g["name"] != "Auto" {
		t.Errorf("expected group name Auto, got %v", g["name"])
	}
	members, ok := g["proxies"].([]any)
	if !ok || len(members) != 1 || members[0] != "jp-01" {
		t.Errorf("expected group members [jp-01], got %v", g["proxies"])
	}

	// Check MATCH rule
	ruleList := result["rules"].([]any)
	if len(ruleList) != 1 || ruleList[0] != "MATCH,DIRECT" {
		t.Errorf("expected [MATCH,DIRECT], got %v", ruleList)
	}
}
