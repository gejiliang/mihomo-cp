package handler

import (
	"net/http"
	"os"

	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
)

// ImportHandler handles importing existing mihomo config files.
type ImportHandler struct {
	importSvc     *service.ImportService
	proxies       *store.ProxyStore
	proxyGroups   *store.ProxyGroupStore
	rules         *store.RuleStore
	ruleProviders *store.RuleProviderStore
	configs       *store.ConfigStore
	settings      *store.SettingsStore
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(
	importSvc *service.ImportService,
	proxies *store.ProxyStore,
	proxyGroups *store.ProxyGroupStore,
	rules *store.RuleStore,
	ruleProviders *store.RuleProviderStore,
	configs *store.ConfigStore,
	settings *store.SettingsStore,
) *ImportHandler {
	return &ImportHandler{
		importSvc:     importSvc,
		proxies:       proxies,
		proxyGroups:   proxyGroups,
		rules:         rules,
		ruleProviders: ruleProviders,
		configs:       configs,
		settings:      settings,
	}
}

// Import handles POST /api/import/config
func (h *ImportHandler) Import(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	_ = DecodeJSON(r, &req)

	// If no path provided, use settings
	if req.Path == "" {
		st, err := h.settings.Get()
		if err != nil {
			Error(w, 500, "internal", "failed to get settings: "+err.Error())
			return
		}
		req.Path = st.MihomoConfig
	}

	if req.Path == "" {
		Error(w, 400, "bad_request", "no config path specified")
		return
	}

	data, err := os.ReadFile(req.Path)
	if err != nil {
		Error(w, 400, "bad_request", "failed to read config file: "+err.Error())
		return
	}

	result, err := h.importSvc.ParseConfig(data)
	if err != nil {
		Error(w, 422, "parse_error", "failed to parse config: "+err.Error())
		return
	}

	// Clear existing data and insert imported data
	// We use a simple sequential approach: delete all, then insert all.
	// Note: existing proxies/groups/rules are fetched and deleted individually.
	if err := h.clearAll(); err != nil {
		Error(w, 500, "internal", "failed to clear existing data: "+err.Error())
		return
	}

	// Insert proxies
	for i := range result.Proxies {
		if err := h.proxies.Create(&result.Proxies[i]); err != nil {
			Error(w, 500, "internal", "failed to insert proxy: "+err.Error())
			return
		}
	}

	// Insert proxy groups
	for i := range result.ProxyGroups {
		if err := h.proxyGroups.Create(&result.ProxyGroups[i]); err != nil {
			Error(w, 500, "internal", "failed to insert proxy group: "+err.Error())
			return
		}
	}

	// Insert rules
	for i := range result.Rules {
		if err := h.rules.Create(&result.Rules[i]); err != nil {
			Error(w, 500, "internal", "failed to insert rule: "+err.Error())
			return
		}
	}

	// Insert rule providers
	for _, p := range result.RuleProviders {
		pCopy := p
		if err := h.ruleProviders.Create(&pCopy); err != nil {
			Error(w, 500, "internal", "failed to insert rule provider: "+err.Error())
			return
		}
	}

	// Update system config if present
	if len(result.SystemConfig) > 0 {
		if err := h.configs.Update(result.SystemConfig); err != nil {
			Error(w, 500, "internal", "failed to update system config: "+err.Error())
			return
		}
	}

	JSON(w, 200, map[string]any{
		"status":          "imported",
		"proxies":         len(result.Proxies),
		"proxy_groups":    len(result.ProxyGroups),
		"rules":           len(result.Rules),
		"rule_providers":  len(result.RuleProviders),
		"has_system_config": len(result.SystemConfig) > 0,
	})
}

// clearAll removes all existing proxies, proxy groups, rules, and rule providers.
func (h *ImportHandler) clearAll() error {
	proxies, err := h.proxies.List("", "")
	if err != nil {
		return err
	}
	for _, p := range proxies {
		if err := h.proxies.Delete(p.ID); err != nil {
			return err
		}
	}

	groups, err := h.proxyGroups.List()
	if err != nil {
		return err
	}
	for _, g := range groups {
		if err := h.proxyGroups.Delete(g.ID); err != nil {
			return err
		}
	}

	rules, err := h.rules.List("", "")
	if err != nil {
		return err
	}
	for _, r := range rules {
		if err := h.rules.Delete(r.ID); err != nil {
			return err
		}
	}

	providers, err := h.ruleProviders.List()
	if err != nil {
		return err
	}
	for _, p := range providers {
		if err := h.ruleProviders.Delete(p.ID); err != nil {
			return err
		}
	}

	return nil
}
