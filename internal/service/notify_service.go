package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NotifyService delivers alert notifications through a configurable channel.
//
// Channels:
//   - ""        : disabled — Send is a no-op returning nil
//   - "bark"    : GET  <target>/<title>/<body>   (target = https://api.day.app/<key>)
//   - "ntfy"    : POST <target> with "<title>\n<body>" as the raw body
//     (title goes into the body because HTTP headers cannot carry UTF-8)
//   - "webhook" : POST <target> with JSON {"title": ..., "body": ...}
type NotifyService struct {
	http *http.Client
}

// NewNotifyService creates a new NotifyService.
func NewNotifyService() *NotifyService {
	return &NotifyService{http: &http.Client{Timeout: 15 * time.Second}}
}

// Send delivers one notification. An unknown channel is an error.
func (n *NotifyService) Send(channel, target, title, body string) error {
	if channel == "" {
		return nil
	}
	if target == "" {
		return fmt.Errorf("notify channel %q has no target configured", channel)
	}

	var req *http.Request
	var err error
	switch channel {
	case "bark":
		u := strings.TrimRight(target, "/") + "/" + url.PathEscape(title) + "/" + url.PathEscape(body)
		req, err = http.NewRequest(http.MethodGet, u, nil)
	case "ntfy":
		req, err = http.NewRequest(http.MethodPost, target, strings.NewReader(title+"\n"+body))
		if req != nil {
			req.Header.Set("Content-Type", "text/plain; charset=utf-8")
		}
	case "webhook":
		payload, merr := json.Marshal(map[string]string{"title": title, "body": body})
		if merr != nil {
			return fmt.Errorf("marshal webhook payload: %w", merr)
		}
		req, err = http.NewRequest(http.MethodPost, target, bytes.NewReader(payload))
		if req != nil {
			req.Header.Set("Content-Type", "application/json")
		}
	default:
		return fmt.Errorf("unknown notify channel %q", channel)
	}
	if err != nil {
		return fmt.Errorf("build notify request: %w", err)
	}

	resp, err := n.http.Do(req)
	if err != nil {
		return fmt.Errorf("send %s notification: %w", channel, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s notification failed: %d %s", channel, resp.StatusCode, string(respBody))
	}
	return nil
}
