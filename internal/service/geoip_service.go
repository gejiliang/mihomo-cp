package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
)

// GeoIPService detects proxy exit countries by switching the running mihomo's
// GLOBAL selector to each proxy and querying ip-api.com through the SOCKS port.
type GeoIPService struct{}

func NewGeoIPService() *GeoIPService {
	return &GeoIPService{}
}

// DetectAll uses the running mihomo instance to detect exit countries.
// It switches the GLOBAL selector to each proxy, makes a request through
// the SOCKS proxy to ip-api.com, and records the exit country.
func (s *GeoIPService) DetectAll(extController, extSecret string, proxies []*model.Proxy) (map[string]string, error) {
	if len(proxies) == 0 {
		return map[string]string{}, nil
	}

	// Get mihomo's SOCKS port and current mode from /configs
	socksPort, origMode, err := s.getConfig(extController, extSecret)
	if err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}

	// Save current GLOBAL selection so we can restore it
	origSelection, _ := s.getGroupSelection(extController, extSecret, "GLOBAL")

	// Temporarily switch to global mode so SOCKS traffic uses GLOBAL selector
	if origMode != "global" {
		if err := s.patchConfig(extController, extSecret, "global"); err != nil {
			return nil, fmt.Errorf("switch to global mode: %w", err)
		}
	}

	result := make(map[string]string)
	for _, p := range proxies {
		country := s.testProxy(p.Name, socksPort, extController, extSecret)
		if country != "" {
			result[p.Name] = country
		}
	}

	// Restore original mode and GLOBAL selection
	if origMode != "global" {
		_ = s.patchConfig(extController, extSecret, origMode)
	}
	if origSelection != "" {
		_ = s.switchProxy(extController, extSecret, "GLOBAL", origSelection)
	}

	return result, nil
}

func (s *GeoIPService) getConfig(extController, secret string) (socksPort int, mode string, err error) {
	req, err := http.NewRequest("GET", extController+"/configs", nil)
	if err != nil {
		return 0, "", err
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	var cfg struct {
		SocksPort int    `json:"socks-port"`
		MixedPort int    `json:"mixed-port"`
		Mode      string `json:"mode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return 0, "", err
	}
	port := cfg.SocksPort
	if port == 0 {
		port = cfg.MixedPort
	}
	if port == 0 {
		return 0, "", fmt.Errorf("no socks or mixed port configured")
	}
	return port, cfg.Mode, nil
}

func (s *GeoIPService) patchConfig(extController, secret, mode string) error {
	body, _ := json.Marshal(map[string]string{"mode": mode})
	req, err := http.NewRequest("PATCH", extController+"/configs", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *GeoIPService) getGroupSelection(extController, secret, group string) (string, error) {
	req, err := http.NewRequest("GET", extController+"/proxies/"+url.PathEscape(group), nil)
	if err != nil {
		return "", err
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var info struct {
		Now string `json:"now"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	return info.Now, nil
}

func (s *GeoIPService) switchProxy(extController, secret, group, proxy string) error {
	body, _ := json.Marshal(map[string]string{"name": proxy})
	req, err := http.NewRequest("PUT", extController+"/proxies/"+url.PathEscape(group), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("switch proxy returned %d", resp.StatusCode)
	}
	return nil
}

func (s *GeoIPService) testProxy(proxyName string, socksPort int, extController, secret string) string {
	// Switch GLOBAL to this proxy
	if err := s.switchProxy(extController, secret, "GLOBAL", proxyName); err != nil {
		return ""
	}

	// Small delay for switch to take effect
	time.Sleep(200 * time.Millisecond)

	// Make request through SOCKS5 proxy to ip-api.com
	proxyURL, _ := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", socksPort))
	testClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	testResp, err := testClient.Get("http://ip-api.com/json?fields=status,countryCode")
	if err != nil {
		return ""
	}
	defer testResp.Body.Close()

	var result struct {
		Status      string `json:"status"`
		CountryCode string `json:"countryCode"`
	}
	if err := json.NewDecoder(testResp.Body).Decode(&result); err != nil {
		return ""
	}
	if result.Status == "success" {
		return result.CountryCode
	}
	return ""
}

// DetectOne detects the exit country for a single proxy.
func (s *GeoIPService) DetectOne(extController, extSecret string, proxyName string) (string, error) {
	socksPort, origMode, err := s.getConfig(extController, extSecret)
	if err != nil {
		return "", fmt.Errorf("get config: %w", err)
	}

	origSelection, _ := s.getGroupSelection(extController, extSecret, "GLOBAL")

	if origMode != "global" {
		if err := s.patchConfig(extController, extSecret, "global"); err != nil {
			return "", fmt.Errorf("switch to global mode: %w", err)
		}
	}

	country := s.testProxy(proxyName, socksPort, extController, extSecret)

	if origMode != "global" {
		_ = s.patchConfig(extController, extSecret, origMode)
	}
	if origSelection != "" {
		_ = s.switchProxy(extController, extSecret, "GLOBAL", origSelection)
	}

	return country, nil
}

// CountryFromFlag extracts ISO 3166-1 alpha-2 country code from flag emoji.
// Regional Indicator Symbols: 🇺🇸 = U+1F1FA U+1F1F8 → "US"
func CountryFromFlag(s string) string {
	runes := []rune(s)
	for i := 0; i < len(runes)-1; i++ {
		r1 := runes[i]
		r2 := runes[i+1]
		if r1 >= 0x1F1E6 && r1 <= 0x1F1FF && r2 >= 0x1F1E6 && r2 <= 0x1F1FF {
			c1 := 'A' + (r1 - 0x1F1E6)
			c2 := 'A' + (r2 - 0x1F1E6)
			return string([]rune{c1, c2})
		}
	}
	return ""
}
