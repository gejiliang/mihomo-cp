package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// ImportResult holds all parsed entities from a mihomo config YAML.
type ImportResult struct {
	Proxies       []model.Proxy
	ProxyGroups   []model.ProxyGroup
	Rules         []model.Rule
	RuleProviders map[string]model.RuleProvider
	SystemConfig  json.RawMessage
}

// ImportService parses existing mihomo config YAML into structured models.
type ImportService struct{}

// NewImportService creates a new ImportService.
func NewImportService() *ImportService {
	return &ImportService{}
}

// reservedKeys are the top-level keys that map to structured entities, not system config.
var reservedKeys = map[string]bool{
	"proxies":        true,
	"proxy-groups":   true,
	"rules":          true,
	"rule-providers": true,
}

// ParseConfig parses a raw mihomo YAML config into an ImportResult.
func (s *ImportService) ParseConfig(data []byte) (*ImportResult, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}

	result := &ImportResult{
		RuleProviders: make(map[string]model.RuleProvider),
	}

	// Extract proxies
	if proxiesRaw, ok := raw["proxies"]; ok {
		proxiesList, ok := proxiesRaw.([]any)
		if !ok {
			return nil, fmt.Errorf("proxies must be a list")
		}
		for i, item := range proxiesList {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("proxy[%d] is not a map", i)
			}
			proxy, err := s.extractProxy(m, i)
			if err != nil {
				return nil, fmt.Errorf("proxy[%d]: %w", i, err)
			}
			result.Proxies = append(result.Proxies, proxy)
		}
	}

	// Extract proxy-groups
	if groupsRaw, ok := raw["proxy-groups"]; ok {
		groupsList, ok := groupsRaw.([]any)
		if !ok {
			return nil, fmt.Errorf("proxy-groups must be a list")
		}
		for i, item := range groupsList {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("proxy-group[%d] is not a map", i)
			}
			group, err := s.extractGroup(m, i)
			if err != nil {
				return nil, fmt.Errorf("proxy-group[%d]: %w", i, err)
			}
			result.ProxyGroups = append(result.ProxyGroups, group)
		}
	}

	// Extract rules
	if rulesRaw, ok := raw["rules"]; ok {
		rulesList, ok := rulesRaw.([]any)
		if !ok {
			return nil, fmt.Errorf("rules must be a list")
		}
		for i, item := range rulesList {
			ruleStr, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("rule[%d] is not a string", i)
			}
			rule, err := s.parseRuleString(ruleStr, i)
			if err != nil {
				return nil, fmt.Errorf("rule[%d] %q: %w", i, ruleStr, err)
			}
			result.Rules = append(result.Rules, rule)
		}
	}

	// Extract rule-providers
	if providersRaw, ok := raw["rule-providers"]; ok {
		providersMap, ok := providersRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("rule-providers must be a map")
		}
		for name, item := range providersMap {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("rule-provider %q is not a map", name)
			}
			provider, err := s.extractProvider(name, m)
			if err != nil {
				return nil, fmt.Errorf("rule-provider %q: %w", name, err)
			}
			result.RuleProviders[name] = provider
		}
	}

	// System config: everything except reserved keys
	sysMap := make(map[string]any)
	for k, v := range raw {
		if !reservedKeys[k] {
			sysMap[k] = v
		}
	}
	if len(sysMap) > 0 {
		sysJSON, err := json.Marshal(sysMap)
		if err != nil {
			return nil, fmt.Errorf("marshal system config: %w", err)
		}
		result.SystemConfig = sysJSON
	}

	return result, nil
}

func (s *ImportService) extractProxy(m map[string]any, idx int) (model.Proxy, error) {
	name, _ := m["name"].(string)
	proxyType, _ := m["type"].(string)
	if name == "" {
		return model.Proxy{}, fmt.Errorf("missing name")
	}

	// Config is everything except name and type
	configMap := make(map[string]any)
	for k, v := range m {
		if k != "name" && k != "type" {
			configMap[k] = v
		}
	}
	configJSON, err := json.Marshal(configMap)
	if err != nil {
		return model.Proxy{}, fmt.Errorf("marshal config: %w", err)
	}

	return model.Proxy{
		ID:        uuid.New().String(),
		Name:      name,
		Type:      proxyType,
		Config:    configJSON,
		SortOrder: idx,
	}, nil
}

func (s *ImportService) extractGroup(m map[string]any, idx int) (model.ProxyGroup, error) {
	name, _ := m["name"].(string)
	groupType, _ := m["type"].(string)
	if name == "" {
		return model.ProxyGroup{}, fmt.Errorf("missing name")
	}

	// Extract members (proxies list)
	var members []string
	if proxiesRaw, ok := m["proxies"]; ok {
		if proxiesList, ok := proxiesRaw.([]any); ok {
			for _, p := range proxiesList {
				if pName, ok := p.(string); ok {
					members = append(members, pName)
				}
			}
		}
	}
	membersJSON, err := json.Marshal(members)
	if err != nil {
		return model.ProxyGroup{}, fmt.Errorf("marshal members: %w", err)
	}

	// Config is everything except name, type, proxies
	configMap := make(map[string]any)
	for k, v := range m {
		if k != "name" && k != "type" && k != "proxies" {
			configMap[k] = v
		}
	}
	configJSON, err := json.Marshal(configMap)
	if err != nil {
		return model.ProxyGroup{}, fmt.Errorf("marshal config: %w", err)
	}

	return model.ProxyGroup{
		ID:        uuid.New().String(),
		Name:      name,
		Type:      groupType,
		Config:    configJSON,
		Members:   membersJSON,
		SortOrder: idx,
	}, nil
}

func (s *ImportService) parseRuleString(ruleStr string, idx int) (model.Rule, error) {
	parts := strings.Split(ruleStr, ",")
	if len(parts) < 2 {
		return model.Rule{}, fmt.Errorf("invalid rule format")
	}

	ruleType := strings.TrimSpace(parts[0])
	rule := model.Rule{
		ID:        uuid.New().String(),
		Type:      ruleType,
		SortOrder: idx,
		Params:    json.RawMessage("{}"),
	}

	if ruleType == "MATCH" {
		// MATCH,TARGET
		rule.Target = strings.TrimSpace(parts[1])
	} else {
		if len(parts) < 3 {
			return model.Rule{}, fmt.Errorf("rule %q requires at least TYPE,PAYLOAD,TARGET", ruleType)
		}
		rule.Payload = strings.TrimSpace(parts[1])
		rule.Target = strings.TrimSpace(parts[2])

		// Check for no-resolve
		if len(parts) >= 4 && strings.TrimSpace(parts[3]) == "no-resolve" {
			paramsJSON, err := json.Marshal(map[string]any{"no-resolve": true})
			if err != nil {
				return model.Rule{}, fmt.Errorf("marshal params: %w", err)
			}
			rule.Params = paramsJSON
		}
	}

	return rule, nil
}

func (s *ImportService) extractProvider(name string, m map[string]any) (model.RuleProvider, error) {
	providerType, _ := m["type"].(string)
	behavior, _ := m["behavior"].(string)

	// Config is everything except type and behavior
	configMap := make(map[string]any)
	for k, v := range m {
		if k != "type" && k != "behavior" {
			configMap[k] = v
		}
	}
	configJSON, err := json.Marshal(configMap)
	if err != nil {
		return model.RuleProvider{}, fmt.Errorf("marshal config: %w", err)
	}

	return model.RuleProvider{
		ID:       uuid.New().String(),
		Name:     name,
		Type:     providerType,
		Behavior: behavior,
		Config:   configJSON,
	}, nil
}
