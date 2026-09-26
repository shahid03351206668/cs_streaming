package main

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

// APIClient talks to the running Tasksy server.
type APIClient struct {
	baseURL string
	http    *http.Client
}

func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 20 * time.Second}}
}

// Do sends a JSON request (body may be nil) and decodes the JSON response into out (may be nil).
func (c *APIClient) Do(method, path, token string, body any, out any) (int, map[string]any, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}

	var generic map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &generic)
	}

	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, generic, fmt.Errorf("decode response: %w (raw: %s)", err, truncate(raw, 300))
		}
	}

	return resp.StatusCode, generic, nil
}

// PostForm sends a multipart/form-data-free, urlencoded form POST (used for a couple
// of legacy endpoints that bind with `form:` tags instead of JSON).
func (c *APIClient) PostForm(path, token string, form url.Values, out any) (int, map[string]any, error) {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	var generic map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &generic)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, generic, fmt.Errorf("decode response: %w (raw: %s)", err, truncate(raw, 300))
		}
	}
	return resp.StatusCode, generic, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

// StripeClient makes raw form-encoded calls to the Stripe API. Kept dependency-free
// (no stripe-go) since this tool only needs a handful of endpoints.
type StripeClient struct {
	secretKey string
	http      *http.Client
}

func NewStripeClient(secretKey string) *StripeClient {
	return &StripeClient{secretKey: secretKey, http: &http.Client{Timeout: 20 * time.Second}}
}

func (s *StripeClient) call(method, path string, form url.Values, connectedAccount string) (map[string]any, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, "https://api.stripe.com/v1"+path, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(s.secretKey, "")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if connectedAccount != "" {
		req.Header.Set("Stripe-Account", connectedAccount)
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode stripe response: %w (raw: %s)", err, truncate(raw, 300))
	}
	if errObj, ok := out["error"].(map[string]any); ok {
		return out, fmt.Errorf("stripe error: %v", errObj["message"])
	}
	return out, nil
}

func (s *StripeClient) Get(path string, connectedAccount string) (map[string]any, error) {
	return s.call(http.MethodGet, path, nil, connectedAccount)
}

func (s *StripeClient) Post(path string, form url.Values, connectedAccount string) (map[string]any, error) {
	return s.call(http.MethodPost, path, form, connectedAccount)
}
