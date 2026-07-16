package handler

import (
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/gejiliang/mihomo-cp/internal/middleware"
	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
)

// PublishHandler handles publish workflow endpoints.
type PublishHandler struct {
	proxies       *store.ProxyStore
	proxyGroups   *store.ProxyGroupStore
	rules         *store.RuleStore
	ruleProviders *store.RuleProviderStore
	configs       *store.ConfigStore
	publishStore  *store.PublishStore
	settings      *store.SettingsStore
	configSvc     *service.ConfigService
	validator     *service.Validator
	publishSvc    *service.PublishService
	importSvc     *service.ImportService
}

// NewPublishHandler creates a new PublishHandler.
func NewPublishHandler(
	proxies *store.ProxyStore,
	proxyGroups *store.ProxyGroupStore,
	rules *store.RuleStore,
	ruleProviders *store.RuleProviderStore,
	configs *store.ConfigStore,
	publishStore *store.PublishStore,
	settings *store.SettingsStore,
	configSvc *service.ConfigService,
	validator *service.Validator,
	publishSvc *service.PublishService,
	importSvc *service.ImportService,
) *PublishHandler {
	return &PublishHandler{
		proxies:       proxies,
		proxyGroups:   proxyGroups,
		rules:         rules,
		ruleProviders: ruleProviders,
		configs:       configs,
		publishStore:  publishStore,
		settings:      settings,
		configSvc:     configSvc,
		validator:     validator,
		publishSvc:    publishSvc,
		importSvc:     importSvc,
	}
}

// renderDraft reads all draft data from stores and renders YAML.
// If a raw config draft exists (from the config editor), it takes precedence.
func (h *PublishHandler) renderDraft() ([]byte, []model.Proxy, []model.ProxyGroup, []model.Rule, map[string]model.RuleProvider, error) {
	// Check for raw config draft first
	st, err := h.settings.Get()
	if err == nil && st.RawConfigDraft != "" {
		return []byte(st.RawConfigDraft), nil, nil, nil, nil, nil
	}

	sysConfig, err := h.configs.Get()
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("get system config: %w", err)
	}

	proxies, err := h.proxies.List("", "")
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("list proxies: %w", err)
	}

	groupPtrs, err := h.proxyGroups.List()
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("list proxy groups: %w", err)
	}

	rules, err := h.rules.List("", "")
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("list rules: %w", err)
	}

	providerPtrs, err := h.ruleProviders.List()
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("list rule providers: %w", err)
	}

	// Dereference slices
	proxySlice := make([]model.Proxy, len(proxies))
	for i, p := range proxies {
		proxySlice[i] = *p
	}
	groupSlice := make([]model.ProxyGroup, len(groupPtrs))
	for i, g := range groupPtrs {
		groupSlice[i] = *g
	}
	ruleSlice := make([]model.Rule, len(rules))
	for i, r := range rules {
		ruleSlice[i] = *r
	}
	providerMap := make(map[string]model.RuleProvider, len(providerPtrs))
	for _, p := range providerPtrs {
		providerMap[p.Name] = *p
	}

	yamlBytes, err := h.configSvc.RenderYAML(sysConfig, proxySlice, groupSlice, ruleSlice, providerMap)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("render yaml: %w", err)
	}

	return yamlBytes, proxySlice, groupSlice, ruleSlice, providerMap, nil
}

// Preview handles GET /api/publish/preview
func (h *PublishHandler) Preview(w http.ResponseWriter, r *http.Request) {
	yamlBytes, _, _, _, _, err := h.renderDraft()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}

	st, err := h.settings.Get()
	if err != nil {
		Error(w, 500, "internal", "failed to get settings: "+err.Error())
		return
	}

	var currentContent []byte
	currentContent, _ = os.ReadFile(st.MihomoConfig)

	diff := generateSimpleDiff(string(currentContent), string(yamlBytes))

	JSON(w, 200, map[string]string{
		"yaml": string(yamlBytes),
		"diff": diff,
	})
}

// Validate handles POST /api/publish/validate
func (h *PublishHandler) Validate(w http.ResponseWriter, r *http.Request) {
	yamlBytes, proxySlice, groupSlice, ruleSlice, providerMap, err := h.renderDraft()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}

	// Skip structural validation when using raw config draft (entities are nil)
	isRawDraft := proxySlice == nil && groupSlice == nil && ruleSlice == nil && providerMap == nil
	var errs []service.ValidationError
	if !isRawDraft {
		errs = h.validator.Validate(proxySlice, groupSlice, ruleSlice, providerMap)
	}
	if errs == nil {
		errs = []service.ValidationError{}
	}

	st, err := h.settings.Get()
	if err != nil {
		Error(w, 500, "internal", "failed to get settings: "+err.Error())
		return
	}

	var output string
	if st.MihomoBinary != "" {
		output, _ = h.publishSvc.ValidateWithMihomo(yamlBytes, st.MihomoDir, st.MihomoBinary)
	}

	// valid = no error-level structural issues
	valid := true
	for _, e := range errs {
		if e.Level == "error" {
			valid = false
			break
		}
	}

	JSON(w, 200, map[string]any{
		"yaml":   string(yamlBytes),
		"errors": errs,
		"output": output,
		"valid":  valid,
	})
}

// Publish handles POST /api/publish
func (h *PublishHandler) Publish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Note string `json:"note"`
	}
	_ = DecodeJSON(r, &req)

	yamlBytes, proxySlice, groupSlice, ruleSlice, providerMap, err := h.renderDraft()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}

	// Structural validation — reject if any error-level issues (skip for raw config drafts)
	isRawDraft := proxySlice == nil && groupSlice == nil && ruleSlice == nil && providerMap == nil
	if !isRawDraft {
		errs := h.validator.Validate(proxySlice, groupSlice, ruleSlice, providerMap)
		for _, e := range errs {
			if e.Level == "error" {
				Error(w, 422, "validation_error", e.Message)
				return
			}
		}
	}

	st, err := h.settings.Get()
	if err != nil {
		Error(w, 500, "internal", "failed to get settings: "+err.Error())
		return
	}

	operator := middleware.GetUserID(r.Context())

	record, err := h.publishSvc.Publish(service.PublishRequest{
		ConfigYAML: yamlBytes,
		ConfigPath: st.MihomoConfig,
		ConfigDir:  st.MihomoDir,
		MihomoBin:  st.MihomoBinary,
		Operator:   operator,
		Note:       req.Note,
	})
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}

	// Clear raw config draft after successful publish
	if record.Status == "success" {
		_ = h.settings.ClearRawConfigDraft()
	}

	JSON(w, 200, record)
}

// Rollback handles POST /api/publish/rollback
func (h *PublishHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	st, err := h.settings.Get()
	if err != nil {
		Error(w, 500, "internal", "failed to get settings: "+err.Error())
		return
	}

	operator := middleware.GetUserID(r.Context())

	record, err := h.publishSvc.Rollback(st.MihomoConfig, st.MihomoDir, operator)
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}

	JSON(w, 200, record)
}

// Discard handles POST /api/publish/discard
// Re-imports the running config into DB and overwrites the config file with
// the re-rendered YAML so that the draft and running config are in sync.
func (h *PublishHandler) Discard(w http.ResponseWriter, r *http.Request) {
	st, err := h.settings.Get()
	if err != nil {
		Error(w, 500, "internal", "failed to get settings: "+err.Error())
		return
	}

	// Read current running config
	data, err := os.ReadFile(st.MihomoConfig)
	if err != nil {
		Error(w, 500, "internal", "failed to read config file: "+err.Error())
		return
	}

	// Parse and re-import into DB
	result, err := h.importSvc.ParseConfig(data)
	if err != nil {
		Error(w, 500, "internal", "failed to parse config: "+err.Error())
		return
	}

	// Clear existing data
	if err := h.clearAllData(); err != nil {
		Error(w, 500, "internal", "failed to clear data: "+err.Error())
		return
	}

	// Insert imported data
	for i := range result.Proxies {
		_ = h.proxies.Create(&result.Proxies[i])
	}
	for i := range result.ProxyGroups {
		_ = h.proxyGroups.Create(&result.ProxyGroups[i])
	}
	for i := range result.Rules {
		_ = h.rules.Create(&result.Rules[i])
	}
	for _, p := range result.RuleProviders {
		pCopy := p
		_ = h.ruleProviders.Create(&pCopy)
	}
	if len(result.SystemConfig) > 0 {
		_ = h.configs.Update(result.SystemConfig)
	}

	// Clear any raw config draft
	_ = h.settings.ClearRawConfigDraft()

	// Re-render from DB and write back to config file so they match
	yamlBytes, _, _, _, _, err := h.renderDraft()
	if err != nil {
		Error(w, 500, "internal", "failed to render draft: "+err.Error())
		return
	}
	if err := os.WriteFile(st.MihomoConfig, yamlBytes, 0644); err != nil {
		Error(w, 500, "internal", "failed to write config file: "+err.Error())
		return
	}

	JSON(w, 200, map[string]string{"status": "discarded"})
}

// clearAllData removes all proxies, proxy groups, rules, and rule providers.
func (h *PublishHandler) clearAllData() error {
	proxies, _ := h.proxies.List("", "")
	for _, p := range proxies {
		_ = h.proxies.Delete(p.ID)
	}
	groups, _ := h.proxyGroups.List()
	for _, g := range groups {
		_ = h.proxyGroups.Delete(g.ID)
	}
	rules, _ := h.rules.List("", "")
	for _, r := range rules {
		_ = h.rules.Delete(r.ID)
	}
	providers, _ := h.ruleProviders.List()
	for _, p := range providers {
		_ = h.ruleProviders.Delete(p.ID)
	}
	return nil
}

// History handles GET /api/publish/history
func (h *PublishHandler) History(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	records, err := h.publishStore.List(limit)
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, records)
}

// HistoryDetail handles GET /api/publish/history/{id}
func (h *PublishHandler) HistoryDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	record, err := h.publishStore.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "publish record not found")
		return
	}
	JSON(w, 200, record)
}

// Status handles GET /api/publish/status
func (h *PublishHandler) Status(w http.ResponseWriter, r *http.Request) {
	yamlBytes, _, _, _, _, err := h.renderDraft()
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}

	st, err := h.settings.Get()
	if err != nil {
		Error(w, 500, "internal", "failed to get settings: "+err.Error())
		return
	}

	currentContent, _ := os.ReadFile(st.MihomoConfig)
	hasChanges := string(currentContent) != string(yamlBytes)

	var runningVersion string
	if records, err := h.publishStore.List(1); err == nil && len(records) > 0 {
		runningVersion = records[0].Version
	}

	JSON(w, 200, map[string]any{
		"has_changes":     hasChanges,
		"running_version": runningVersion,
	})
}

// generateSimpleDiff produces a simple line-level diff between old and new content.
func generateSimpleDiff(oldContent, newContent string) string {
	if oldContent == newContent {
		return ""
	}
	// Return a simple indicator that content has changed; the full YAML is returned separately.
	return fmt.Sprintf("--- current\n+++ draft\n(content differs, %d bytes → %d bytes)", len(oldContent), len(newContent))
}
