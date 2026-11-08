package discord

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/lengzhao/agentkit"
)

const maxDiscordMessageRunes = 1900

func (p *Platform) sendText(ctx context.Context, session agentkit.SessionID, text string) error {
	raw, ok := p.routes.Load(session)
	if !ok {
		return fmt.Errorf("discord: no delivery for session %q", session)
	}
	d := raw.(delivery)
	p.mu.RLock()
	sess := p.session
	p.mu.RUnlock()
	if sess == nil {
		return fmt.Errorf("discord: session not connected")
	}
	for _, chunk := range splitDiscordMessage(text) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if _, err := sess.ChannelMessageSend(d.channelID, chunk); err != nil {
			return fmt.Errorf("discord: send message: %w", err)
		}
	}
	return nil
}

func splitDiscordMessage(text string) []string {
	if text == "" {
		return nil
	}
	if utf8.RuneCountInString(text) <= maxDiscordMessageRunes {
		return []string{text}
	}
	var chunks []string
	remaining := text
	for len(remaining) > 0 {
		if utf8.RuneCountInString(remaining) <= maxDiscordMessageRunes {
			chunks = append(chunks, remaining)
			break
		}
		cut := maxDiscordMessageRunes
		runes := 0
		byteIdx := 0
		for i := range remaining {
			runes++
			byteIdx = i
			if runes >= cut {
				break
			}
		}
		// include last rune
		if byteIdx < len(remaining)-1 {
			_, size := utf8.DecodeRuneInString(remaining[byteIdx:])
			byteIdx += size
		} else {
			byteIdx = len(remaining)
		}
		chunks = append(chunks, remaining[:byteIdx])
		remaining = remaining[byteIdx:]
	}
	return chunks
}

func (p *Platform) sendMediaPart(context.Context, agentkit.SessionID, agentkit.ContentPart) error {
	return fmt.Errorf("discord: outbound media not implemented yet")
}
