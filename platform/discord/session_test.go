package discord

import (
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestBuildSessionKey(t *testing.T) {
	if got := buildSessionKey("ch1", "u1", false); got != "discord:ch1:u1" {
		t.Fatalf("per-user: %q", got)
	}
	if got := buildSessionKey("ch1", "u1", true); got != "discord:ch1" {
		t.Fatalf("shared: %q", got)
	}
}

func TestNewRequiresToken(t *testing.T) {
	_, err := New(Config{}, Deps{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSplitDiscordMessage(t *testing.T) {
	long := stringsRepeat("a", maxDiscordMessageRunes+100)
	chunks := splitDiscordMessage(long)
	if len(chunks) < 2 {
		t.Fatalf("expected split, got %d chunks", len(chunks))
	}
	for _, c := range chunks {
		if len([]rune(c)) > maxDiscordMessageRunes {
			t.Fatalf("chunk too long: %d runes", len([]rune(c)))
		}
	}
}

func TestIsDiscordBotMention(t *testing.T) {
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			Content: "<@123>",
			Mentions: []*discordgo.User{{ID: "123"}},
		},
	}
	if !isDiscordBotMention(m, "123", "", false) {
		t.Fatal("expected user mention")
	}
}

func stringsRepeat(s string, n int) string {
	return strings.Repeat(s, n)
}
