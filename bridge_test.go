package clawbridge_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lengzhao/clawbridge"
	"github.com/lengzhao/clawbridge/bus"
	_ "github.com/lengzhao/clawbridge/drivers"
)

func TestNewStartStopNoop(t *testing.T) {
	cfg := clawbridge.Config{
		Clients: []clawbridge.ClientConfig{
			{ID: "a", Driver: "noop", Enabled: true},
		},
	}
	b, err := clawbridge.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := b.Start(ctx); err != nil {
		t.Fatal(err)
	}

	msg := &bus.OutboundMessage{
		ClientID: "a",
		To:       bus.Recipient{SessionID: "room1"},
		Text:     "hi",
	}
	if err := b.Bus().PublishOutbound(ctx, msg); err != nil {
		t.Fatal(err)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := b.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
}

func TestInitReply(t *testing.T) {
	clawbridge.SetDefault(nil)
	cfg := clawbridge.Config{
		Clients: []clawbridge.ClientConfig{
			{ID: "a", Driver: "noop", Enabled: true},
		},
	}
	if err := clawbridge.Init(cfg); err != nil {
		t.Fatal(err)
	}
	defer clawbridge.SetDefault(nil)

	ctx := context.Background()
	if err := clawbridge.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = clawbridge.Stop(context.Background()) }()

	in := &clawbridge.InboundMessage{
		ClientID:  "a",
		SessionID: "room1",
		Sender:  clawbridge.SenderInfo{PlatformID: "u1"},
		Peer:    clawbridge.Peer{Kind: "group"},
	}
	if _, err := clawbridge.Reply(ctx, in, "ok", ""); err != nil {
		t.Fatal(err)
	}

	if err := clawbridge.UpdateStatus(ctx, nil, clawbridge.UpdateStatusProcessing, nil); !errors.Is(err, clawbridge.ErrInvalidMessage) {
		t.Fatalf("UpdateStatus nil in: want ErrInvalidMessage, got %v", err)
	}
	if err := clawbridge.UpdateStatus(ctx, in, "", nil); !errors.Is(err, clawbridge.ErrInvalidMessage) {
		t.Fatalf("UpdateStatus empty state: want ErrInvalidMessage, got %v", err)
	}
}
