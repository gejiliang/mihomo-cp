package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/service"
	"github.com/gejiliang/mihomo-cp/internal/store"
	"github.com/google/uuid"
)

// ProxyHandler handles CRUD operations for proxies.
type ProxyHandler struct {
	proxies     *store.ProxyStore
	proxyGroups *store.ProxyGroupStore
	geoIPSvc    *service.GeoIPService
	settings    *store.SettingsStore
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(ps *store.ProxyStore, pgs *store.ProxyGroupStore, geoIP *service.GeoIPService, ss *store.SettingsStore) *ProxyHandler {
	return &ProxyHandler{proxies: ps, proxyGroups: pgs, geoIPSvc: geoIP, settings: ss}
}

// List handles GET /api/proxies
func (h *ProxyHandler) List(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	proxyType := r.URL.Query().Get("type")
	proxies, err := h.proxies.List(search, proxyType)
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, proxies)
}

// Get handles GET /api/proxies/{id}
func (h *ProxyHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.proxies.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "proxy not found")
		return
	}
	JSON(w, 200, p)
}

// detectCountryAsync triggers async country detection for a single proxy after create/update.
func (h *ProxyHandler) detectCountryAsync(proxyID, proxyName string) {
	go func() {
		settings, err := h.settings.Get()
		if err != nil || settings.ExtController == "" {
			return
		}
		country, err := h.geoIPSvc.DetectOne(settings.ExtController, settings.ExtSecret, proxyName)
		if err != nil || country == "" {
			return
		}
		p, err := h.proxies.GetByID(proxyID)
		if err != nil {
			return
		}
		if p.Country != country {
			p.Country = country
			if err := h.proxies.Update(p); err != nil {
				log.Printf("auto-detect country: failed to update proxy %s: %v", proxyName, err)
			}
		}
	}()
}

// Create handles POST /api/proxies
func (h *ProxyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var p model.Proxy
	if err := DecodeJSON(r, &p); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	p.ID = uuid.New().String()
	if err := h.proxies.Create(&p); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	h.detectCountryAsync(p.ID, p.Name)
	JSON(w, 201, p)
}

// Update handles PUT /api/proxies/{id}
func (h *ProxyHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var p model.Proxy
	if err := DecodeJSON(r, &p); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	p.ID = id
	if err := h.proxies.Update(&p); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	h.detectCountryAsync(p.ID, p.Name)
	JSON(w, 200, p)
}

// Delete handles DELETE /api/proxies/{id}
// If the proxy is referenced by groups, it removes the reference from those groups first.
func (h *ProxyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	proxy, err := h.proxies.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "proxy not found")
		return
	}

	// Remove this proxy from any groups that reference it
	groups, _ := h.proxyGroups.List()
	for _, g := range groups {
		var members []string
		_ = json.Unmarshal(g.Members, &members)
		filtered := make([]string, 0, len(members))
		changed := false
		for _, m := range members {
			if m == proxy.Name {
				changed = true
			} else {
				filtered = append(filtered, m)
			}
		}
		if changed {
			newMembers, _ := json.Marshal(filtered)
			g.Members = newMembers
			_ = h.proxyGroups.Update(g)
		}
	}

	if err := h.proxies.Delete(id); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "deleted"})
}

// Copy handles POST /api/proxies/{id}/copy
func (h *ProxyHandler) Copy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.proxies.GetByID(id)
	if err != nil {
		Error(w, 404, "not_found", "proxy not found")
		return
	}
	p.ID = uuid.New().String()
	p.Name = p.Name + " (copy)"
	if err := h.proxies.Create(p); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 201, p)
}

// Reorder handles POST /api/proxies/reorder
func (h *ProxyHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Error(w, 400, "bad_request", "invalid request body")
		return
	}
	if err := h.proxies.Reorder(req.IDs); err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, map[string]string{"status": "ok"})
}

// Refs handles GET /api/proxies/{id}/refs
func (h *ProxyHandler) Refs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	refs, err := h.findRefs(id)
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}
	JSON(w, 200, refs)
}

func (h *ProxyHandler) findRefs(id string) ([]string, error) {
	proxy, err := h.proxies.GetByID(id)
	if err != nil {
		return nil, err
	}
	groups, _ := h.proxyGroups.List()
	var refs []string
	for _, g := range groups {
		var members []string
		_ = json.Unmarshal(g.Members, &members)
		for _, m := range members {
			if m == proxy.Name {
				refs = append(refs, g.Name)
			}
		}
	}
	return refs, nil
}

// DetectCountries handles POST /api/proxies/detect-countries
// Accepts a JSON body with a name→country mapping, e.g.:
// {"countries": {"PS-SS-c21s1": "US", "HK-01-DX": "HK", ...}}
// If the body is empty, starts a temporary mihomo instance to detect exit countries.
func (h *ProxyHandler) DetectCountries(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Countries map[string]string `json:"countries"`
	}
	_ = DecodeJSON(r, &req)

	proxies, err := h.proxies.List("", "")
	if err != nil {
		Error(w, 500, "internal", err.Error())
		return
	}

	countryMap := req.Countries

	// If no explicit mapping, detect by routing through each proxy via mihomo
	if len(countryMap) == 0 {
		settings, err := h.settings.Get()
		if err != nil {
			Error(w, 500, "internal", "failed to read settings: "+err.Error())
			return
		}
		extController := settings.ExtController
		if extController == "" {
			Error(w, 400, "bad_request", "mihomo external controller not configured in settings")
			return
		}
		detected, err := h.geoIPSvc.DetectAll(extController, settings.ExtSecret, proxies)
		if err != nil {
			Error(w, 500, "internal", "country detection failed: "+err.Error())
			return
		}
		countryMap = detected
	}

	// Update proxies with detected countries
	updated := 0
	for _, p := range proxies {
		country, found := countryMap[p.Name]
		if !found || country == "" {
			continue
		}
		if p.Country == country {
			continue
		}
		p.Country = country
		if err := h.proxies.Update(p); err != nil {
			continue
		}
		updated++
	}

	JSON(w, 200, map[string]any{
		"status":  "ok",
		"total":   len(proxies),
		"updated": updated,
	})
}
