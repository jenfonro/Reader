package legado

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type runContext struct {
	engine  *Engine
	source  Source
	baseURL string
	key     string
	page    int
	result  any
	book    Book
	chapter Chapter
	vars    map[string]string
}

func newRunContext(engine *Engine, source Source) *runContext {
	base := sourceBaseURL(source.BookSourceURL)
	return &runContext{
		engine:  engine,
		source:  source,
		baseURL: base,
		vars:    map[string]string{},
	}
}

func (rc *runContext) clone() *runContext {
	if rc == nil {
		return nil
	}
	vars := make(map[string]string, len(rc.vars))
	for key, value := range rc.vars {
		vars[key] = value
	}
	return &runContext{
		engine:  rc.engine,
		source:  rc.source,
		baseURL: rc.baseURL,
		key:     rc.key,
		page:    rc.page,
		result:  rc.result,
		book:    rc.book,
		chapter: rc.chapter,
		vars:    vars,
	}
}

func sourceBaseURL(raw string) string {
	base := strings.TrimSpace(raw)
	if index := strings.Index(base, "##"); index >= 0 {
		base = strings.TrimSpace(base[:index])
	}
	return strings.TrimRight(base, "/")
}

func (rc *runContext) resolveURL(ref string) string {
	base := rc.baseURL
	if base == "" {
		base = sourceBaseURL(rc.source.BookSourceURL)
	}
	return joinURL(base, ref)
}

func (rc *runContext) origin() string {
	target := rc.baseURL
	if target == "" {
		target = sourceBaseURL(rc.source.BookSourceURL)
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return target
	}
	return parsed.Scheme + "://" + parsed.Host
}

func (rc *runContext) setResponse(res httpResult) {
	rc.baseURL = res.URL
	rc.result = res.Body
}

func (rc *runContext) evalRuleString(ctx context.Context, rule string, input any) (string, error) {
	value, err := rc.evalRule(ctx, rule, input)
	if err != nil {
		return "", err
	}
	return normalizeText(anyString(value)), nil
}

func anyString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	case fmt.Stringer:
		return v.String()
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			text := strings.TrimSpace(anyString(item))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		content, _ := json.Marshal(v)
		return string(content)
	default:
		content, err := json.Marshal(v)
		if err == nil && string(content) != "null" {
			var s string
			if json.Unmarshal(content, &s) == nil {
				return s
			}
			return string(content)
		}
		return fmt.Sprint(v)
	}
}

func anyBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		normalized := strings.ToLower(strings.TrimSpace(v))
		return normalized == "true" || normalized == "1" || normalized == "yes"
	case float64:
		return v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	default:
		return false
	}
}

func anyMap(value any) (map[string]any, bool) {
	switch v := value.(type) {
	case map[string]any:
		return v, true
	case map[string]string:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = item
		}
		return out, true
	default:
		return nil, false
	}
}

func normalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

func compactNonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
