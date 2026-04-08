package feishu

import (
	"encoding/json"
	"regexp"
	"strings"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// mentionPlaceholderRegex matches @_user_N placeholders inserted by Feishu for mentions.
var mentionPlaceholderRegex = regexp.MustCompile(`@_user_\d+`)

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func buildMarkdownCard(content string) (string, error) {
	card := map[string]any{
		"schema": "2.0",
		"body": map[string]any{
			"elements": []map[string]any{
				{
					"tag":     "markdown",
					"content": content,
				},
			},
		},
	}
	data, err := json.Marshal(card)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func extractJSONStringField(content, field string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		return ""
	}
	raw, ok := m[field]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func extractImageKey(content string) string { return extractJSONStringField(content, "image_key") }

func extractFileKey(content string) string { return extractJSONStringField(content, "file_key") }

func extractFileName(content string) string { return extractJSONStringField(content, "file_name") }

func stripMentionPlaceholders(content string, mentions []*larkim.MentionEvent) string {
	if len(mentions) == 0 {
		return content
	}
	for _, m := range mentions {
		if m.Key != nil && *m.Key != "" {
			content = strings.ReplaceAll(content, *m.Key, "")
		}
	}
	content = mentionPlaceholderRegex.ReplaceAllString(content, "")
	return strings.TrimSpace(content)
}

func extractCardImageKeys(rawContent string) (feishuKeys []string, externalURLs []string) {
	if rawContent == "" {
		return nil, nil
	}
	var card map[string]any
	if err := json.Unmarshal([]byte(rawContent), &card); err != nil {
		return nil, nil
	}
	extractImageKeysRecursive(card, &feishuKeys, &externalURLs)
	return feishuKeys, externalURLs
}

func isExternalURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func extractImageKeysRecursive(v any, feishuKeys, externalURLs *[]string) {
	switch val := v.(type) {
	case map[string]any:
		if tag, ok := val["tag"].(string); ok {
			switch tag {
			case "img":
				if imgKey, ok := val["img_key"].(string); ok && imgKey != "" {
					*feishuKeys = append(*feishuKeys, imgKey)
				}
				if src, ok := val["src"].(string); ok && src != "" {
					if isExternalURL(src) {
						*externalURLs = append(*externalURLs, src)
					} else {
						*feishuKeys = append(*feishuKeys, src)
					}
				}
			case "icon":
				if iconKey, ok := val["icon_key"].(string); ok && iconKey != "" {
					*feishuKeys = append(*feishuKeys, iconKey)
				}
			}
		}
		for _, child := range val {
			extractImageKeysRecursive(child, feishuKeys, externalURLs)
		}
	case []any:
		for _, item := range val {
			extractImageKeysRecursive(item, feishuKeys, externalURLs)
		}
	}
}
