package service

import (
	"encoding/json"
	"fmt"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// builtinTargets are valid policy targets that don't need to be defined explicitly.
var builtinTargets = map[string]bool{
	"DIRECT": true,
	"REJECT": true,
	"PASS":   true,
}

// ValidationError represents a single validation issue.
type ValidationError struct {
	Code    string `json:"code"`
	Level   string `json:"level"` // "error" or "warning"
	Message string `json:"message"`
}

// Validator checks a set of mihomo config entities for correctness.
type Validator struct{}

// NewValidator creates a new Validator.
func NewValidator() *Validator {
	return &Validator{}
}

// Validate runs all checks and returns a slice of ValidationErrors (may be empty).
func (v *Validator) Validate(
	proxies []model.Proxy,
	groups []model.ProxyGroup,
	rules []model.Rule,
	providers map[string]model.RuleProvider,
) []ValidationError {
	var errs []ValidationError

	// Build lookup sets
	proxyNames := make(map[string]bool, len(proxies))
	for _, p := range proxies {
		proxyNames[p.Name] = true
	}

	groupNames := make(map[string]bool, len(groups))
	for _, g := range groups {
		groupNames[g.Name] = true
	}

	validTargets := make(map[string]bool)
	for k := range builtinTargets {
		validTargets[k] = true
	}
	for k := range proxyNames {
		validTargets[k] = true
	}
	for k := range groupNames {
		validTargets[k] = true
	}

	providerNames := make(map[string]bool, len(providers))
	for name := range providers {
		providerNames[name] = true
	}

	// 1. Dangling references & empty groups
	groupMembers := make(map[string][]string, len(groups))
	for _, g := range groups {
		var members []string
		if len(g.Members) > 0 {
			if err := json.Unmarshal(g.Members, &members); err != nil {
				errs = append(errs, ValidationError{
					Code:    "INVALID_MEMBERS_JSON",
					Level:   "error",
					Message: fmt.Sprintf("group %q has invalid members JSON: %v", g.Name, err),
				})
				continue
			}
		}
		groupMembers[g.Name] = members

		if len(members) == 0 {
			errs = append(errs, ValidationError{
				Code:    "EMPTY_GROUP",
				Level:   "warning",
				Message: fmt.Sprintf("group %q has no members", g.Name),
			})
		}

		for _, member := range members {
			if !proxyNames[member] && !groupNames[member] && !builtinTargets[member] {
				errs = append(errs, ValidationError{
					Code:    "DANGLING_REFERENCE",
					Level:   "error",
					Message: fmt.Sprintf("group %q references unknown member %q", g.Name, member),
				})
			}
		}
	}

	// 2. Rule target validity & RULE-SET provider check
	matchFound := false
	matchIsLast := false
	for i, r := range rules {
		if r.Type == "MATCH" {
			matchFound = true
			matchIsLast = (i == len(rules)-1)
		}

		if r.Type != "MATCH" && !validTargets[r.Target] {
			errs = append(errs, ValidationError{
				Code:    "INVALID_RULE_TARGET",
				Level:   "error",
				Message: fmt.Sprintf("rule %s,%s references unknown target %q", r.Type, r.Payload, r.Target),
			})
		}

		if r.Type == "RULE-SET" && !providerNames[r.Payload] {
			errs = append(errs, ValidationError{
				Code:    "MISSING_PROVIDER",
				Level:   "error",
				Message: fmt.Sprintf("RULE-SET rule references unknown provider %q", r.Payload),
			})
		}
	}

	// 3. MATCH rule warnings
	if !matchFound {
		errs = append(errs, ValidationError{
			Code:    "MISSING_MATCH",
			Level:   "warning",
			Message: "no MATCH rule found; traffic may not be handled",
		})
	} else if !matchIsLast {
		errs = append(errs, ValidationError{
			Code:    "MATCH_NOT_LAST",
			Level:   "warning",
			Message: "MATCH rule should be the last rule",
		})
	}

	// 4. Circular references in group → group relationships
	circularErrs := v.detectCircularRefs(groups, groupMembers, groupNames)
	errs = append(errs, circularErrs...)

	return errs
}

// detectCircularRefs uses DFS to find cycles in group membership.
func (v *Validator) detectCircularRefs(
	groups []model.ProxyGroup,
	groupMembers map[string][]string,
	groupNames map[string]bool,
) []ValidationError {
	var errs []ValidationError

	// visited: permanently marked (no cycle through this node)
	// inStack: currently on the DFS stack (cycle if we see it again)
	visited := make(map[string]bool)
	inStack := make(map[string]bool)
	reported := make(map[string]bool)

	var dfs func(name string) bool
	dfs = func(name string) bool {
		if inStack[name] {
			return true // cycle detected
		}
		if visited[name] {
			return false
		}
		inStack[name] = true
		for _, member := range groupMembers[name] {
			if groupNames[member] {
				if dfs(member) && !reported[name] {
					reported[name] = true
					errs = append(errs, ValidationError{
						Code:    "CIRCULAR_REFERENCE",
						Level:   "error",
						Message: fmt.Sprintf("group %q is part of a circular reference", name),
					})
				}
			}
		}
		inStack[name] = false
		visited[name] = true
		return false
	}

	for _, g := range groups {
		dfs(g.Name)
	}

	return errs
}
