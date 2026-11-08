package discord

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

func stripDiscordMentionWithRole(text, botID, botRoleID string) string {
	text = strings.ReplaceAll(text, "<@!"+botID+">", "")
	text = strings.ReplaceAll(text, "<@"+botID+">", "")
	if botRoleID != "" {
		text = strings.ReplaceAll(text, "<@&"+botRoleID+">", "")
	}
	return strings.TrimSpace(text)
}

func stripEveryoneHere(text string) string {
	text = strings.ReplaceAll(text, "@everyone", "")
	text = strings.ReplaceAll(text, "@here", "")
	return strings.TrimSpace(text)
}

func isDiscordBotMention(m *discordgo.MessageCreate, botID, botRoleID string, respondToAtEveryoneAndHere bool) bool {
	if respondToAtEveryoneAndHere && m.MentionEveryone {
		return true
	}
	for _, u := range m.Mentions {
		if u != nil && u.ID == botID {
			return true
		}
	}
	if strings.Contains(m.Content, "<@"+botID+">") || strings.Contains(m.Content, "<@!"+botID+">") {
		return true
	}
	for _, roleID := range m.MentionRoles {
		if roleID == botRoleID && roleID != "" {
			return true
		}
	}
	return botRoleID != "" && strings.Contains(m.Content, "<@&"+botRoleID+">")
}
