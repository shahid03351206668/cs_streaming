package main

// Small helpers for navigating the map[string]any shapes returned by
// encoding/json, so response handling above doesn't need per-endpoint types.

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

func getFloat(m map[string]any, path ...string) float64 {
	switch v := dig(m, path...).(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		return 0
	}
}

func getBool(m map[string]any, path ...string) bool {
	v, _ := dig(m, path...).(bool)
	return v
}

// getSlice returns a []string from a JSON array of strings (used for
// requirements.currently_due-style fields).
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

// getSliceRaw returns a raw []any, for arrays of objects (e.g. transactions, data rows).
func getSliceRaw(m map[string]any, path ...string) []any {
	v, _ := dig(m, path...).([]any)
	return v
}
