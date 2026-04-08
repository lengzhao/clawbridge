package feishu

import (
	"strings"

	"github.com/lengzhao/clawbridge/bus"
)

// groupTrigger mirrors PicoClaw GroupTriggerConfig for YAML credentials.
type groupTrigger struct {
	MentionOnly bool
	Prefixes    []string
}

func shouldRespondInGroup(gt groupTrigger, isMentioned bool, content string) (bool, string) {
	if isMentioned {
		return true, strings.TrimSpace(content)
	}
	if gt.MentionOnly {
		return false, content
	}
	if len(gt.Prefixes) > 0 {
		for _, prefix := range gt.Prefixes {
			if prefix != "" && strings.HasPrefix(content, prefix) {
				return true, strings.TrimSpace(strings.TrimPrefix(content, prefix))
			}
		}
		return false, content
	}
	return true, strings.TrimSpace(content)
}

func isAllowedSender(sender bus.SenderInfo, allowList []string) bool {
	if len(allowList) == 0 {
		return true
	}
	for _, allowed := range allowList {
		if allowed == "*" || matchAllowed(sender, allowed) {
			return true
		}
	}
	return false
}

// matchAllowed is aligned with PicoClaw identity.MatchAllowed but uses clawbridge bus.SenderInfo.
func matchAllowed(sender bus.SenderInfo, allowed string) bool {
	allowed = strings.TrimSpace(allowed)
	if allowed == "" {
		return false
	}
	if platform, id, ok := parseCanonicalID(allowed); ok {
		if !isNumeric(platform) {
			candidate := buildCanonicalID(platform, id)
			if candidate != "" && sender.CanonicalID != "" {
				return strings.EqualFold(sender.CanonicalID, candidate)
			}
			return strings.EqualFold(platform, sender.Platform) && sender.PlatformID == id
		}
	}
	isAtUsername := strings.HasPrefix(allowed, "@")
	trimmed := strings.TrimPrefix(allowed, "@")
	allowedID := trimmed
	allowedUser := ""
	if idx := strings.Index(trimmed, "|"); idx > 0 {
		allowedID = trimmed[:idx]
		allowedUser = trimmed[idx+1:]
	}
	if sender.PlatformID != "" && sender.PlatformID == allowedID {
		return true
	}
	if isAtUsername && sender.Username != "" && sender.Username == trimmed {
		return true
	}
	if allowedUser != "" && sender.PlatformID != "" && sender.PlatformID == allowedID {
		return true
	}
	if allowedUser != "" && sender.Username != "" && sender.Username == allowedUser {
		return true
	}
	return false
}

func buildCanonicalID(platform, platformID string) string {
	p := strings.ToLower(strings.TrimSpace(platform))
	id := strings.TrimSpace(platformID)
	if p == "" || id == "" {
		return ""
	}
	return p + ":" + id
}

func parseCanonicalID(canonical string) (platform, id string, ok bool) {
	canonical = strings.TrimSpace(canonical)
	idx := strings.Index(canonical, ":")
	if idx <= 0 || idx == len(canonical)-1 {
		return "", "", false
	}
	return canonical[:idx], canonical[idx+1:], true
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	start := 0
	if s[0] == '-' && len(s) > 1 {
		start = 1
	}
	for i := start; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
