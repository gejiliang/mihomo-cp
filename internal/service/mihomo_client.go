package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// MihomoClient is an HTTP client wrapper for mihomo's external-controller API.
type MihomoClient struct {
	baseURL string
	secret  string
	client  *http.Client
}

// NewMihomoClient creates a new MihomoClient.
func NewMihomoClient(baseURL, secret string) *MihomoClient {
	return &MihomoClient{
		baseURL: baseURL,
		secret:  secret,
		client:  &http.Client{},
	}
}

// do is the internal HTTP helper. It sets Content-Type and Authorization Bearer header.
func (c *MihomoClient) do(method, path string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mihomo API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// ReloadConfig sends PUT /configs with {"path": configDir} to reload mihomo's config.
func (c *MihomoClient) ReloadConfig(configDir string) error {
	_, err := c.do(http.MethodPut, "/configs", map[string]string{"path": configDir})
	return err
}

// GetConnections returns raw JSON from GET /connections.
func (c *MihomoClient) GetConnections() (json.RawMessage, error) {
	data, err := c.do(http.MethodGet, "/connections", nil)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// CloseConnection sends DELETE /connections/:id to close a specific connection.
func (c *MihomoClient) CloseConnection(id string) error {
	_, err := c.do(http.MethodDelete, "/connections/"+id, nil)
	return err
}

// GetProxies returns raw JSON from GET /proxies.
func (c *MihomoClient) GetProxies() (json.RawMessage, error) {
	data, err := c.do(http.MethodGet, "/proxies", nil)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// GetProxyNow returns the currently selected member ("now") of a proxy or
// group from the running mihomo. Read-only — it never changes any selection.
func (c *MihomoClient) GetProxyNow(name string) (string, error) {
	data, err := c.do(http.MethodGet, "/proxies/"+url.PathEscape(name), nil)
	if err != nil {
		return "", err
	}
	var info struct {
		Now string `json:"now"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return "", fmt.Errorf("parse proxy %q: %w", name, err)
	}
	return info.Now, nil
}

// GetProxyDelay returns raw JSON from GET /proxies/:name/delay?url=&timeout=.
func (c *MihomoClient) GetProxyDelay(name, testURL string, timeout int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("url", testURL)
	q.Set("timeout", fmt.Sprintf("%d", timeout))
	path := fmt.Sprintf("/proxies/%s/delay?%s", url.PathEscape(name), q.Encode())
	data, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// SwitchProxy sends PUT /proxies/:group with {"name": proxy} to switch the active proxy in a group.
func (c *MihomoClient) SwitchProxy(group, proxy string) error {
	path := fmt.Sprintf("/proxies/%s", url.PathEscape(group))
	_, err := c.do(http.MethodPut, path, map[string]string{"name": proxy})
	return err
}

// GetRules returns raw JSON from GET /rules.
func (c *MihomoClient) GetRules() (json.RawMessage, error) {
	data, err := c.do(http.MethodGet, "/rules", nil)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// GetProviders returns raw JSON from GET /providers/rules.
func (c *MihomoClient) GetProviders() (json.RawMessage, error) {
	data, err := c.do(http.MethodGet, "/providers/rules", nil)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// RefreshProvider sends PUT /providers/rules/:name to refresh a rule provider.
func (c *MihomoClient) RefreshProvider(name string) error {
	path := fmt.Sprintf("/providers/rules/%s", url.PathEscape(name))
	_, err := c.do(http.MethodPut, path, nil)
	return err
}

// GetProxyProviders returns raw JSON from GET /providers/proxies. The raw
// response contains only runtime proxy metadata (mihomo does not include node
// credentials); callers still project it through an allow-list before serving
// it to clients.
func (c *MihomoClient) GetProxyProviders() (json.RawMessage, error) {
	data, err := c.do(http.MethodGet, "/providers/proxies", nil)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// ProviderNodeHealthcheck sends GET /providers/proxies/:provider/:node/healthcheck
// to test one native provider node. Provider and node are path-escaped
// separately, so same-named nodes under different providers address distinct
// upstream paths and names containing "/", spaces or "#" stay in one segment.
// Only the reported delay is returned; the raw upstream body is never exposed.
func (c *MihomoClient) ProviderNodeHealthcheck(provider, node, testURL string, timeout int) (int, error) {
	q := url.Values{}
	if testURL != "" {
		q.Set("url", testURL)
	}
	q.Set("timeout", fmt.Sprintf("%d", timeout))
	path := fmt.Sprintf("/providers/proxies/%s/%s/healthcheck?%s",
		url.PathEscape(provider), url.PathEscape(node), q.Encode())
	data, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return 0, err
	}
	var result struct {
		Delay int `json:"delay"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, fmt.Errorf("parse provider node healthcheck: %w", err)
	}
	return result.Delay, nil
}

// RefreshProxyProvider sends PUT /providers/proxies/:name to refresh a native
// PROXY provider (subscription). The rule-provider endpoint answers 404 for
// proxy providers, so subscription refresh must use this distinct method.
func (c *MihomoClient) RefreshProxyProvider(name string) error {
	path := fmt.Sprintf("/providers/proxies/%s", url.PathEscape(name))
	_, err := c.do(http.MethodPut, path, nil)
	return err
}

// GetVersion returns raw JSON from GET /version.
func (c *MihomoClient) GetVersion() (json.RawMessage, error) {
	data, err := c.do(http.MethodGet, "/version", nil)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
