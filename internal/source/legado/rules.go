package legado

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

func (rc *runContext) evalRule(ctx context.Context, rule string, input any) (any, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return "", nil
	}
	parts := splitTopLevel(rule, "||")
	var lastErr error
	for _, part := range parts {
		value, err := rc.evalRuleNoFallback(ctx, strings.TrimSpace(part), input)
		if err != nil {
			lastErr = err
			continue
		}
		if hasValue(value) {
			return value, nil
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", nil
}

func (rc *runContext) evalRuleNoFallback(ctx context.Context, rule string, input any) (any, error) {
	if rule == "" {
		return "", nil
	}
	if parts := splitTopLevel(rule, "&&"); len(parts) > 1 {
		values := make([]string, 0, len(parts))
		for _, part := range parts {
			value, err := rc.evalRule(ctx, strings.TrimSpace(part), input)
			if err != nil {
				return "", err
			}
			values = append(values, compactNonEmpty(anyString(value))...)
		}
		return strings.Join(values, "\n"), nil
	}

	if before, code, after, ok := splitJSBlock(rule); ok {
		value := input
		var err error
		if strings.TrimSpace(before) != "" {
			value, err = rc.evalRule(ctx, before, input)
			if err != nil {
				return "", err
			}
		}
		value, err = rc.evalJS(ctx, code, value)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(after) != "" {
			return rc.evalRule(ctx, after, value)
		}
		return value, nil
	}

	if before, code, ok := splitAtJS(rule); ok {
		value := input
		var err error
		if strings.TrimSpace(before) != "" {
			value, err = rc.evalRule(ctx, before, input)
			if err != nil {
				return "", err
			}
		}
		return rc.evalJS(ctx, code, value)
	}

	processed, err := rc.replaceTemplates(ctx, rule, input)
	if err != nil {
		return "", err
	}
	if processed != rule {
		if containsRuleOperator(processed) {
			return rc.evalRule(ctx, processed, input)
		}
		return processed, nil
	}

	if selector, pattern, replace, ok := splitRegexRule(rule); ok {
		value := input
		if strings.TrimSpace(selector) != "" {
			var err error
			value, err = rc.evalRule(ctx, selector, input)
			if err != nil {
				return "", err
			}
		}
		return rc.regexReplace(ctx, anyString(value), pattern, replace)
	}

	return rc.evalSelector(ctx, rule, input)
}

func (rc *runContext) evalSelector(ctx context.Context, rule string, input any) (any, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return "", nil
	}
	if strings.HasPrefix(rule, "@css:") {
		return rc.evalHTMLRule(strings.TrimSpace(strings.TrimPrefix(rule, "@css:")), input)
	}
	if strings.HasPrefix(rule, "@") {
		rule = strings.TrimLeft(rule, "@")
	}
	if strings.HasPrefix(rule, "$") {
		return evalJSONPath(input, rule), nil
	}
	if strings.HasPrefix(rule, "/") {
		return evalXPath(rule, input), nil
	}
	if looksLikeHTMLRule(rule) {
		return rc.evalHTMLRule(rule, input)
	}
	return rule, nil
}

func (rc *runContext) replaceTemplates(ctx context.Context, rule string, input any) (string, error) {
	if !strings.Contains(rule, "{{") {
		return rule, nil
	}
	var out strings.Builder
	for {
		start := strings.Index(rule, "{{")
		if start < 0 {
			out.WriteString(rule)
			return out.String(), nil
		}
		out.WriteString(rule[:start])
		rule = rule[start+2:]
		end := strings.Index(rule, "}}")
		if end < 0 {
			out.WriteString("{{")
			out.WriteString(rule)
			return out.String(), nil
		}
		expr := strings.TrimSpace(rule[:end])
		value, err := rc.evalTemplateExpr(ctx, expr, input)
		if err != nil {
			return "", err
		}
		out.WriteString(anyString(value))
		rule = rule[end+2:]
	}
}

func (rc *runContext) evalTemplateExpr(ctx context.Context, expr string, input any) (any, error) {
	switch expr {
	case "key":
		return rc.key, nil
	case "page":
		if rc.page <= 0 {
			return "1", nil
		}
		return rc.page, nil
	}
	if strings.HasPrefix(expr, "@@") {
		return rc.evalRule(ctx, strings.TrimLeft(expr, "@"), input)
	}
	if strings.HasPrefix(expr, "$") {
		return evalJSONPath(input, expr), nil
	}
	return rc.evalJS(ctx, expr, input)
}

func containsRuleOperator(rule string) bool {
	return strings.Contains(rule, "##") || strings.Contains(rule, "<js>") || strings.Contains(rule, "@js:") || strings.Contains(rule, "&&") || strings.Contains(rule, "||")
}

func hasValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case []any:
		return len(v) > 0
	case []htmlNode:
		return len(v) > 0
	default:
		return true
	}
}

func splitJSBlock(rule string) (string, string, string, bool) {
	start := strings.Index(rule, "<js>")
	if start < 0 {
		return "", "", "", false
	}
	end := strings.Index(rule[start+4:], "</js>")
	if end < 0 {
		return "", "", "", false
	}
	end += start + 4
	return rule[:start], rule[start+4 : end], rule[end+5:], true
}

func splitAtJS(rule string) (string, string, bool) {
	index := strings.Index(rule, "@js:")
	if index < 0 {
		return "", "", false
	}
	return rule[:index], rule[index+4:], true
}

func splitRegexRule(rule string) (string, string, string, bool) {
	index := strings.Index(rule, "##")
	if index < 0 {
		return "", "", "", false
	}
	selector := rule[:index]
	rest := rule[index+2:]
	second := strings.Index(rest, "##")
	if second < 0 {
		return selector, rest, "", true
	}
	return selector, rest[:second], rest[second+2:], true
}

func splitTopLevel(input string, sep string) []string {
	parts := []string{}
	start := 0
	depth := 0
	quote := rune(0)
	escaped := false
	for index, char := range input {
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if char == '\\' {
				escaped = true
				continue
			}
			if char == quote {
				quote = 0
			}
			continue
		}
		switch char {
		case '\'', '"', '`':
			quote = char
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 && strings.HasPrefix(input[index:], sep) {
				parts = append(parts, input[start:index])
				start = index + len(sep)
			}
		}
	}
	if len(parts) == 0 {
		return []string{input}
	}
	parts = append(parts, input[start:])
	return parts
}

func looksLikeHTMLRule(rule string) bool {
	if strings.Contains(rule, "@") {
		return true
	}
	if strings.HasPrefix(rule, ".") || strings.HasPrefix(rule, "#") || strings.HasPrefix(rule, "[") {
		return true
	}
	prefixes := []string{"tag.", "class.", "id.", "text.", "href", "src", "html", "text", "ownText", "children", "content"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(rule, prefix) {
			return true
		}
	}
	return false
}

func parseRuleObject(raw string) (map[string]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]string{}, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(fields))
	for key, rawValue := range fields {
		if len(rawValue) == 0 || string(rawValue) == "null" {
			out[key] = ""
			continue
		}
		var text string
		if err := json.Unmarshal(rawValue, &text); err == nil {
			out[key] = text
			continue
		}
		out[key] = string(rawValue)
	}
	return out, nil
}

func requireRuleObject(raw string, name string) (map[string]string, error) {
	fields, err := parseRuleObject(raw)
	if err != nil {
		return nil, errors.New(name + " 规则格式不正确")
	}
	return fields, nil
}
