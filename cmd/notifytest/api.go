package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type API struct {
	base string
	http *http.Client
}

func newAPI(base string) *API {
	return &API{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 60 * time.Second}}
}

func (a *API) do(req *http.Request) (int, map[string]any, error) {
	resp, err := a.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, nil
}

func (a *API) JSON(method, path, token string, body any) (int, map[string]any, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, a.base+path, rd)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return a.do(req)
}

func (a *API) Form(path, token string, form url.Values) (int, map[string]any, error) {
	req, err := http.NewRequest(http.MethodPost, a.base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return a.do(req)
}

func dig(m map[string]any, path ...string) any {
	var cur any = m
	for _, p := range path {
		cm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = cm[p]
	}
	return cur
}

func str(m map[string]any, path ...string) string {
	s, _ := dig(m, path...).(string)
	return s
}

func errMsg(m map[string]any) string {
	if s := str(m, "error"); s != "" {
		return s
	}
	return str(m, "message")
}

func (a *API) List(path, token string) (int, []map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, a.base+path, nil)
	if err != nil {
		return 0, nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out []map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, nil
}

func num(m map[string]any, path ...string) (float64, bool) {
	v, ok := dig(m, path...).(float64)
	return v, ok
}
