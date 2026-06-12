package lib

import (
	"bytes"
	"fmt"
	"html/template"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var TEMPLATE_FUNCTIONS_MAP template.FuncMap = template.FuncMap{
	"default": func(def, val any) any {
		if val == nil || val == "" {
			return def
		}
		return val
	},
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
	"trim":  strings.TrimSpace,
}

func MakePassword(value string) string {
	hashed, err := bcrypt.GenerateFromPassword([]byte(value), bcrypt.DefaultCost)

	if err != nil {
		return ""
	}
	return string(hashed)
}

func Float(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case int32:
		return float64(t)
	case int16:
		return float64(t)
	case int8:
		return float64(t)
	case uint:
		return float64(t)
	case uint64:
		return float64(t)
	case uint32:
		return float64(t)
	case uint16:
		return float64(t)
	case uint8:
		return float64(t)
	case string:
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return f
		}
		return 0.0
	default:
		return 0.0
	}
}

func RenderTemplate(tmpl string, context map[string]any) (string, error) {
	t, err := template.New("email").Funcs(TEMPLATE_FUNCTIONS_MAP).Parse(tmpl)

	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, context); err != nil {
		return "", fmt.Errorf("template render error: %w", err)
	}

	return buf.String(), nil
}
