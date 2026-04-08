package clawbridge_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lengzhao/clawbridge"
	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/client"
	_ "github.com/lengzhao/clawbridge/drivers"
)

func TestBridgeNoopOutboundAndNoEditOrStatus(t *testing.T) {
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
	defer func() {
		stopCtx, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		_ = b.Stop(stopCtx)
	}()

	if err := b.Bus().PublishOutbound(ctx, &bus.OutboundMessage{
		ClientID: "a",
		To:       bus.Recipient{ChatID: "room1"},
		Text:     "hello",
	}); err != nil {
		t.Fatal(err)
	}

	editErr := b.EditMessage(ctx, &clawbridge.EditMessageRequest{
		ClientID: "a",
		To:       bus.Recipient{ChatID: "room1"},
		Text:     "edited",
	})
	if !errors.Is(editErr, clawbridge.ErrCapabilityUnsupported) {
		t.Fatalf("EditMessage noop: want ErrCapabilityUnsupported, got %v", editErr)
	}

	stErr := b.UpdateStatus(ctx, &clawbridge.UpdateStatusRequest{
		ClientID:  "a",
		To:        bus.Recipient{ChatID: "room1"},
		MessageID: "m1",
		State:     clawbridge.StatusProcessing,
	})
	if !errors.Is(stErr, clawbridge.ErrCapabilityUnsupported) {
		t.Fatalf("UpdateStatus noop: want ErrCapabilityUnsupported, got %v", stErr)
	}
}

func TestManagerUnknownClientUpdateStatus(t *testing.T) {
	ctx := context.Background()
	mb := bus.NewMessageBus()
	m := client.NewManager(mb, nil)
	err := m.UpdateStatus(ctx, &bus.UpdateStatusRequest{
		ClientID:  "x",
		To:        bus.Recipient{ChatID: "c"},
		MessageID: "m",
		State:     bus.StatusCompleted,
	})
	if err == nil {
		t.Fatal("expected error for unknown client")
	}
}

func TestEditMessageUnknownClient(t *testing.T) {
	ctx := context.Background()
	mb := bus.NewMessageBus()
	m := client.NewManager(mb, nil)
	err := m.EditMessage(ctx, &bus.EditMessageRequest{
		ClientID: "nope",
		To:       bus.Recipient{ChatID: "c"},
		Text:     "x",
	})
	if err == nil {
		t.Fatal("expected error for unknown client")
	}
}
