package slack

import (
	"context"
	"testing"

	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/client"
	"github.com/lengzhao/clawbridge/config"
)

func TestParseSlackChatID(t *testing.T) {
	tests := []struct {
		name       string
		chatID     string
		wantChanID string
		wantThread string
	}{
		{name: "channel only", chatID: "C123456", wantChanID: "C123456", wantThread: ""},
		{name: "channel with thread", chatID: "C123456/1234567890.123456", wantChanID: "C123456", wantThread: "1234567890.123456"},
		{name: "DM channel", chatID: "D987654", wantChanID: "D987654", wantThread: ""},
		{name: "empty string", chatID: "", wantChanID: "", wantThread: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chanID, threadTS := parseSlackChatID(tt.chatID)
			if chanID != tt.wantChanID {
				t.Errorf("parseSlackChatID(%q) channelID = %q, want %q", tt.chatID, chanID, tt.wantChanID)
			}
			if threadTS != tt.wantThread {
				t.Errorf("parseSlackChatID(%q) threadTS = %q, want %q", tt.chatID, threadTS, tt.wantThread)
			}
		})
	}
}

func TestStripBotMention(t *testing.T) {
	d := &driver{botUserID: "U12345BOT"}
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "mention at start", input: "<@U12345BOT> hello there", want: "hello there"},
		{name: "mention in middle", input: "hey <@U12345BOT> can you help", want: "hey  can you help"},
		{name: "no mention", input: "hello world", want: "hello world"},
		{name: "empty string", input: "", want: ""},
		{name: "only mention", input: "<@U12345BOT>", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.stripBotMention(tt.input)
			if got != tt.want {
				t.Errorf("stripBotMention(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewSlackDriver(t *testing.T) {
	ctx := context.Background()
	deps := client.Deps{Bus: bus.NewMessageBus()}

	t.Run("missing bot token", func(t *testing.T) {
		_, err := New(ctx, config.ClientConfig{
			ID: "s1",
			Credentials: map[string]any{
				"app_token": "xapp-test",
			},
		}, deps)
		if err == nil {
			t.Fatal("expected error for missing bot_token")
		}
	})

	t.Run("missing app token", func(t *testing.T) {
		_, err := New(ctx, config.ClientConfig{
			ID: "s1",
			Credentials: map[string]any{
				"bot_token": "xoxb-test",
			},
		}, deps)
		if err == nil {
			t.Fatal("expected error for missing app_token")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		drv, err := New(ctx, config.ClientConfig{
			ID: "slack-1",
			Credentials: map[string]any{
				"bot_token": "xoxb-test",
				"app_token": "xapp-test",
				"allow_from": []string{"U123"},
			},
		}, deps)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if drv.Name() != "slack-1" {
			t.Errorf("Name() = %q, want %q", drv.Name(), "slack-1")
		}
	})
}

func TestIsAllowedSenderSlack(t *testing.T) {
	sender := bus.SenderInfo{
		Platform:    "slack",
		PlatformID:  "U_ALLOWED",
		CanonicalID: "slack:U_ALLOWED",
	}
	if !isAllowedSender(sender, []string{}) {
		t.Error("empty allowlist should allow all")
	}
	if !isAllowedSender(sender, []string{"U_ALLOWED"}) {
		t.Error("allowed user should pass")
	}
	if isAllowedSender(sender, []string{"U_OTHER"}) {
		t.Error("non-listed user should be blocked")
	}
}
