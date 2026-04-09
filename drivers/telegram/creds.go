package telegram

import (
	"fmt"
	"strings"
)

func credString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func credBool(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true") || t == "1"
	default:
		return false
	}
}

func credStringSlice(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	return anyToStringSlice(v)
}

func anyToStringSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		var out []string
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			} else if e != nil {
				out = append(out, fmt.Sprint(e))
			}
		}
		return out
	case string:
		if x == "" {
			return nil
		}
		parts := strings.Split(x, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	default:
		return nil
	}
}

func credGroupTrigger(m map[string]any, key string) groupTrigger {
	v, ok := m[key]
	if !ok || v == nil {
		return groupTrigger{}
	}
	sub, ok := v.(map[string]any)
	if !ok {
		return groupTrigger{}
	}
	gt := groupTrigger{}
	if mo, ok := sub["mention_only"].(bool); ok {
		gt.MentionOnly = mo
	}
	gt.Prefixes = anyToStringSlice(sub["prefixes"])
	return gt
}
