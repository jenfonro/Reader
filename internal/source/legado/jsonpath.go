package legado

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

func evalJSONPath(input any, rule string) any {
	jsonText := jsonString(input)
	if strings.TrimSpace(jsonText) == "" {
		return ""
	}
	for _, part := range splitTopLevel(rule, "||") {
		value := evalJSONPathOne(jsonText, strings.TrimSpace(part))
		if hasValue(value) {
			return value
		}
	}
	return ""
}

func evalJSONPathOne(jsonText string, rule string) any {
	if rule == "$" {
		var out any
		if err := json.Unmarshal([]byte(jsonText), &out); err == nil {
			return out
		}
		return jsonText
	}
	if strings.Contains(rule, "..") {
		return evalRecursiveJSONPath(jsonText, rule)
	}
	path := strings.TrimSpace(rule)
	path = strings.TrimPrefix(path, "$")
	path = strings.TrimPrefix(path, ".")
	path = strings.ReplaceAll(path, "[*]", "")
	path = strings.ReplaceAll(path, "[]", "")
	if path == "" {
		return jsonText
	}
	result := gjson.Get(jsonText, path)
	return gjsonValue(result)
}

func evalRecursiveJSONPath(jsonText string, rule string) any {
	prefix, key, ok := splitRecursivePath(rule)
	if !ok {
		return ""
	}
	root := evalJSONPathOne(jsonText, prefix)
	values := []any{}
	collectJSONKey(root, key, &values)
	if len(values) == 1 {
		return values[0]
	}
	return values
}

func splitRecursivePath(rule string) (string, string, bool) {
	index := strings.LastIndex(rule, "..")
	if index < 0 || index+2 >= len(rule) {
		return "", "", false
	}
	prefix := strings.TrimSpace(rule[:index])
	key := strings.TrimSpace(rule[index+2:])
	key = strings.TrimSuffix(key, "[*]")
	key = strings.TrimSuffix(key, "[]")
	key = strings.Trim(key, ".")
	return prefix, key, key != ""
}

func collectJSONKey(value any, key string, out *[]any) {
	switch v := value.(type) {
	case map[string]any:
		for itemKey, itemValue := range v {
			if itemKey == key {
				*out = append(*out, itemValue)
			}
			collectJSONKey(itemValue, key, out)
		}
	case []any:
		for _, item := range v {
			collectJSONKey(item, key, out)
		}
	}
}

func gjsonValue(result gjson.Result) any {
	if !result.Exists() {
		return ""
	}
	if result.IsArray() || result.IsObject() {
		var out any
		if err := json.Unmarshal([]byte(result.Raw), &out); err == nil {
			return out
		}
		return result.Raw
	}
	return result.Value()
}

func jsonString(input any) string {
	switch v := input.(type) {
	case nil:
		return ""
	case string:
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return trimmed
		}
		content, _ := json.Marshal(v)
		return string(content)
	case []byte:
		return string(v)
	default:
		content, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(content)
	}
}
