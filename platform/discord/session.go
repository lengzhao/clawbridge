package discord

import (
	"fmt"
	"strings"

	"github.com/lengzhao/agentkit"
)

func buildSessionKey(channelID, userID string, shareInChannel bool) agentkit.SessionID {
	if shareInChannel {
		return agentkit.SessionID(fmt.Sprintf("discord:%s", channelID))
	}
	return agentkit.SessionID(fmt.Sprintf("discord:%s:%s", channelID, userID))
}

func parseGroupReplyAllGuilds(all bool, csv string) []string {
	if all {
		return []string{"*"}
	}
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return nil
	}
	var out []string
	for _, g := range strings.Split(csv, ",") {
		g = strings.TrimSpace(g)
		if g != "" {
			out = append(out, g)
		}
	}
	return out
}

func (p *Platform) isGroupReplyAllGuild(guildID string) bool {
	for _, g := range p.groupReplyAllGuilds {
		if g == "*" || g == guildID {
			return true
		}
	}
	return false
}
