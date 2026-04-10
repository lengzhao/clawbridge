package feishu

import (
	"testing"

	"github.com/lengzhao/clawbridge/bus"
)

func TestFeishuPickStatusEmoji(t *testing.T) {
	d := &driver{
		statusEmojiProcessing: "THINKING",
	}
	req := &bus.UpdateStatusRequest{State: bus.StatusProcessing, Metadata: map[string]string{
		"feishu_status_emoji_processing": "Typing",
	}}
	if g := feishuStatusEmojiType(req, d); g != "Typing" {
		t.Fatalf("metadata override: got %q", g)
	}
	req.Metadata = nil
	if g := feishuStatusEmojiType(req, d); g != "THINKING" {
		t.Fatalf("driver default: got %q", g)
	}
	d.statusEmojiProcessing = ""
	if g := feishuStatusEmojiType(req, d); g != defaultStatusEmojiProcessing {
		t.Fatalf("builtin: got %q want %q", g, defaultStatusEmojiProcessing)
	}
	if feishuStatusEmojiType(&bus.UpdateStatusRequest{State: "custom"}, d) != "" {
		t.Fatal("unknown state should return empty")
	}
}
