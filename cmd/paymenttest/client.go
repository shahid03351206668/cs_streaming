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

type APIClient struct {
	baseURL string
	http    *http.Client
}

func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 60 * time.Second}}
}

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

func (c *APIClient) DoRaw(path string, body []byte, headers map[string]string) (int, map[string]any, error) {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var generic map[string]any
	_ = json.Unmarshal(raw, &generic)
	return resp.StatusCode, generic, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

// Raw form-encoded Stripe calls; no stripe-go dependency, since only a few endpoints are needed.
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

// Exact response bytes, so webhook payloads can be re-signed.
func (s *StripeClient) GetRaw(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.stripe.com/v1"+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(s.secretKey, "")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("stripe GET %s: status %d", path, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// Retries transient network failures; POSTs are never retried.
func (s *StripeClient) Get(path string, connectedAccount string) (map[string]any, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		out, err := s.call(http.MethodGet, path, nil, connectedAccount)
		if err == nil || out != nil { // out != nil: Stripe answered (possibly with an API error)
			return out, err
		}
		lastErr = err
		time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
	}
	return nil, lastErr
}

func (s *StripeClient) Post(path string, form url.Values, connectedAccount string) (map[string]any, error) {
	return s.call(http.MethodPost, path, form, connectedAccount)
}
