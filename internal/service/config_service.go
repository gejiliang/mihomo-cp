package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"gopkg.in/yaml.v3"
)

// ConfigService renders complete mihomo YAML from structured model data.
type ConfigService struct{}

// NewConfigService creates a new ConfigService.
func NewConfigService() *ConfigService {
	return &ConfigService{}
}

// RenderYAML takes system config + all draft entities and produces a complete mihomo YAML.
func (s *ConfigService) RenderYAML(
	sysConfig json.RawMessage,
	proxies []model.Proxy,
	groups []model.ProxyGroup,
	rules []model.Rule,
	providers map[string]model.RuleProvider,
) ([]byte, error) {
	// Start with system config as base map
	base := make(map[string]any)
	if len(sysConfig) > 0 {
		if err := json.Unmarshal(sysConfig, &base); err != nil {
			return nil, fmt.Errorf("unmarshal system config: %w", err)
		}
	}

	renderedProxies, err := s.renderProxies(proxies)
	if err != nil {
		return nil, fmt.Errorf("render proxies: %w", err)
	}
	base["proxies"] = renderedProxies

	renderedGroups, err := s.renderGroups(groups)
	if err != nil {
		return nil, fmt.Errorf("render groups: %w", err)
	}
	base["proxy-groups"] = renderedGroups

	renderedRules, err := s.renderRules(rules)
	if err != nil {
		return nil, fmt.Errorf("render rules: %w", err)
	}
	base["rules"] = renderedRules

	if len(providers) > 0 {
		renderedProviders, err := s.renderProviders(providers)
		if err != nil {
			return nil, fmt.Errorf("render providers: %w", err)
		}
		base["rule-providers"] = renderedProviders
	}

	out, err := yaml.Marshal(base)
	if err != nil {
		return nil, fmt.Errorf("marshal yaml: %w", err)
	}
	return out, nil
}

func (s *ConfigService) renderProxies(proxies []model.Proxy) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(proxies))
	for _, p := range proxies {
		m := make(map[string]any)
		if len(p.Config) > 0 {
			if err := json.Unmarshal(p.Config, &m); err != nil {
				return nil, fmt.Errorf("unmarshal proxy %s config: %w", p.Name, err)
			}
		}
		m["name"] = p.Name
		m["type"] = p.Type
		result = append(result, m)
	}
	return result, nil
}

func (s *ConfigService) renderGroups(groups []model.ProxyGroup) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		m := make(map[string]any)
		if len(g.Config) > 0 {
			if err := json.Unmarshal(g.Config, &m); err != nil {
				return nil, fmt.Errorf("unmarshal group %s config: %w", g.Name, err)
			}
		}
		m["name"] = g.Name
		m["type"] = g.Type

		// Members JSON is a list of proxy/group names
		var members []string
		if len(g.Members) > 0 {
			if err := json.Unmarshal(g.Members, &members); err != nil {
				return nil, fmt.Errorf("unmarshal group %s members: %w", g.Name, err)
			}
		}
		m["proxies"] = members
		result = append(result, m)
	}
	return result, nil
}

func (s *ConfigService) renderRules(rules []model.Rule) ([]string, error) {
	result := make([]string, 0, len(rules))
	for _, r := range rules {
		var ruleStr string
		if r.Type == "MATCH" {
			ruleStr = fmt.Sprintf("MATCH,%s", r.Target)
		} else {
			ruleStr = fmt.Sprintf("%s,%s,%s", r.Type, r.Payload, r.Target)
			// Check for no-resolve param
			if len(r.Params) > 0 {
				var params map[string]any
				if err := json.Unmarshal(r.Params, &params); err == nil {
					if noResolve, ok := params["no-resolve"]; ok {
						if b, ok := noResolve.(bool); ok && b {
							ruleStr += ",no-resolve"
						}
					}
				}
			}
		}
		result = append(result, ruleStr)
	}
	return result, nil
}

func (s *ConfigService) renderProviders(providers map[string]model.RuleProvider) (map[string]map[string]any, error) {
	result := make(map[string]map[string]any, len(providers))
	for name, p := range providers {
		m := make(map[string]any)
		if len(p.Config) > 0 {
			if err := json.Unmarshal(p.Config, &m); err != nil {
				return nil, fmt.Errorf("unmarshal provider %s config: %w", name, err)
			}
		}
		m["type"] = p.Type
		m["behavior"] = p.Behavior
		result[name] = m
	}
	return result, nil
}

// splitRuleString splits a rule string by comma, respecting that payloads may not contain commas.
// This is a simple helper used internally.
func splitRuleString(s string) []string {
	return strings.SplitN(s, ",", 4)
}
