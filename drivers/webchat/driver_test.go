package webchat

import (
	"context"
	"testing"

	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/client"
	"github.com/lengzhao/clawbridge/config"
)

func TestNewRequiresHTTPListen(t *testing.T) {
	_, err := New(context.Background(), config.ClientConfig{
		ID:     "x",
		Driver: "webchat",
	}, client.Deps{Bus: bus.NewMessageBus()})
	if err == nil {
		t.Fatal("expected error when options.listen missing")
	}
}

func TestNewWithOptionsHTTP(t *testing.T) {
	d, err := New(context.Background(), config.ClientConfig{
		ID:     "wc",
		Driver: "webchat",
		Options: map[string]any{
			"listen": "127.0.0.1:0",
			"path":   "/chat",
		},
	}, client.Deps{Bus: bus.NewMessageBus()})
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "wc" {
		t.Fatalf("name: %q", d.Name())
	}
}

func TestCredString(t *testing.T) {
	if got := credString(nil, "k", "d"); got != "d" {
		t.Fatalf("nil cred: %q", got)
	}
	if got := credString(map[string]any{"k": "  "}, "k", "d"); got != "d" {
		t.Fatalf("empty trim: %q", got)
	}
	if got := credString(map[string]any{"k": "hi"}, "k", "d"); got != "hi" {
		t.Fatalf("string: %q", got)
	}
}
