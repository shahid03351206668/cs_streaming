package main

import (
	"fmt"
	"strconv"
)

// Helpers for navigating the map[string]any shapes from encoding/json, so
// response handling doesn't need a struct per endpoint.

func dig(m map[string]any, path ...string) any {
	var cur any = m
	for _, p := range path {
		cm, ok := cur.(map[string]any)
		if !ok || cm == nil {
			return nil
		}
		cur = cm[p]
	}
	return cur
}

func getMap(m map[string]any, path ...string) map[string]any {
	v, _ := dig(m, path...).(map[string]any)
	return v
}

func getStr(m map[string]any, path ...string) string {
	v, _ := dig(m, path...).(string)
	return v
}

// getNum reads a JSON number, or a numeric string — decimal.Decimal fields
// (amounts on withdrawals, payment rows) marshal as strings.
func getNum(m map[string]any, path ...string) float64 {
	switch v := dig(m, path...).(type) {
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	default:
		return 0
	}
}

func getBool(m map[string]any, path ...string) bool {
	v, _ := dig(m, path...).(bool)
	return v
}

func getSlice(m map[string]any, path ...string) []string {
	raw := getSliceRaw(m, path...)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func getSliceRaw(m map[string]any, path ...string) []any {
	v, _ := dig(m, path...).([]any)
	return v
}

func getRows(m map[string]any, path ...string) []map[string]any {
	raw := getSliceRaw(m, path...)
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		if row, ok := v.(map[string]any); ok {
			out = append(out, row)
		}
	}
	return out
}

// brief renders any value as a short single-line string for failure messages.
func brief(v any) string {
	s := fmt.Sprint(v)
	if len(s) > 140 {
		return s[:140] + "…"
	}
	return s
}

func errMsg(resp map[string]any) string {
	if s := getStr(resp, "error"); s != "" {
		return s
	}
	return getStr(resp, "message")
}
