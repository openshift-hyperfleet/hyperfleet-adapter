package utils

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// TemplateFuncs provides common functions for Go templates.
var TemplateFuncs = template.FuncMap{
	"now": time.Now,
	"date": func(layout string, t time.Time) string {
		return t.Format(layout)
	},
	"dateFormat": func(layout string, t time.Time) string {
		return t.Format(layout)
	},
	"lower": strings.ToLower,
	"upper": strings.ToUpper,
	"title": func(s string) string {
		return cases.Title(language.English).String(s)
	},
	"trim":      strings.TrimSpace,
	"replace":   strings.ReplaceAll,
	"contains":  strings.Contains,
	"hasPrefix": strings.HasPrefix,
	"hasSuffix": strings.HasSuffix,
	"default": func(defaultVal, val interface{}) interface{} {
		if val == nil || val == "" {
			return defaultVal
		}
		return val
	},
	"quote": func(s string) string {
		return fmt.Sprintf("%q", s)
	},
	"int": func(v interface{}) int {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		case string:
			i, _ := strconv.Atoi(val) //nolint:errcheck // returns 0 on error, which is acceptable
			return i
		default:
			return 0
		}
	},
	"int64": func(v interface{}) int64 {
		switch val := v.(type) {
		case int:
			return int64(val)
		case int64:
			return val
		case float64:
			return int64(val)
		case string:
			i, _ := strconv.ParseInt(val, 10, 64) //nolint:errcheck // returns 0 on error, which is acceptable
			return i
		default:
			return 0
		}
	},
	"float64": func(v interface{}) float64 {
		switch val := v.(type) {
		case int:
			return float64(val)
		case int64:
			return float64(val)
		case float64:
			return val
		case string:
			f, _ := strconv.ParseFloat(val, 64) //nolint:errcheck // returns 0 on error, which is acceptable
			return f
		default:
			return 0
		}
	},
	"string": func(v interface{}) string {
		return fmt.Sprintf("%v", v)
	},
}

// RenderTemplate renders a Go template string with the given data.
func RenderTemplate(templateStr string, data map[string]interface{}) (string, error) {
	if !strings.Contains(templateStr, "{{") {
		return templateStr, nil
	}

	tmpl, err := template.New("template").Funcs(TemplateFuncs).Option("missingkey=error").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}
