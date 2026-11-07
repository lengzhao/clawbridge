package telegram

import (
	"log/slog"
	"strings"

	"github.com/go-telegram/bot/models"
)

func stripBotMention(text, botName string) string {
	if botName == "" {
		return text
	}
	text = strings.ReplaceAll(text, "@"+botName, "")
	return strings.TrimSpace(text)
}

func isCommand(msg *models.Message) bool {
	for _, e := range msg.Entities {
		if e.Type == models.MessageEntityTypeBotCommand && e.Offset == 0 {
			return true
		}
	}
	return false
}

func extractEntityText(text string, offset, length int) string {
	runes := []rune(text)
	if offset < 0 || length <= 0 || offset >= len(runes) {
		return ""
	}
	end := offset + length
	if end > len(runes) {
		end = len(runes)
	}
	return string(runes[offset:end])
}

func (p *Platform) isDirectedAtBot(msg *models.Message) bool {
	p.mu.RLock()
	self := p.selfUser
	p.mu.RUnlock()
	if self == nil {
		slog.Debug("telegram: group routing skipped, bot identity unknown")
		return false
	}
	botName := self.Username
	if isCommand(msg) {
		atIdx := strings.Index(msg.Text, "@")
		spaceIdx := strings.Index(msg.Text, " ")
		cmdEnd := len(msg.Text)
		if spaceIdx > 0 {
			cmdEnd = spaceIdx
		}
		if atIdx > 0 && atIdx < cmdEnd {
			target := msg.Text[atIdx+1 : cmdEnd]
			return strings.EqualFold(target, botName)
		}
		return true
	}
	for _, e := range msg.Entities {
		if e.Type == models.MessageEntityTypeMention {
			mention := extractEntityText(msg.Text, e.Offset, e.Length)
			if strings.EqualFold(mention, "@"+botName) {
				return true
			}
		}
	}
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil && msg.ReplyToMessage.From.ID == self.ID {
		return true
	}
	for _, e := range msg.CaptionEntities {
		if e.Type == models.MessageEntityTypeMention {
			mention := extractEntityText(msg.Caption, e.Offset, e.Length)
			if strings.EqualFold(mention, "@"+botName) {
				return true
			}
		}
	}
	return false
}
