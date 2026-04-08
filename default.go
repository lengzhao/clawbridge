package clawbridge

import (
	"context"
	"sync"

	"github.com/lengzhao/clawbridge/config"
	"github.com/lengzhao/clawbridge/media"
)

var (
	defaultMu     sync.RWMutex
	defaultBridge *Bridge
)

// Init installs the process-default bridge (at most once).
func Init(cfg config.Config, opts ...Option) error {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultBridge != nil {
		return ErrAlreadyInitialized
	}
	b, err := New(cfg, opts...)
	if err != nil {
		return err
	}
	defaultBridge = b
	return nil
}

// SetDefault sets or clears the process-default bridge (b may be nil for tests).
func SetDefault(b *Bridge) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultBridge = b
}

// Instance returns the process-default bridge.
func Instance() (*Bridge, error) {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	if defaultBridge == nil {
		return nil, ErrNotInitialized
	}
	return defaultBridge, nil
}

// Start starts the default bridge.
func Start(ctx context.Context) error {
	b, err := Instance()
	if err != nil {
		return err
	}
	return b.Start(ctx)
}

// Stop stops the default bridge.
func Stop(ctx context.Context) error {
	b, err := Instance()
	if err != nil {
		return err
	}
	return b.Stop(ctx)
}

// PublishOutbound publishes on the default bridge bus.
func PublishOutbound(ctx context.Context, msg *OutboundMessage) error {
	b, err := Instance()
	if err != nil {
		return err
	}
	return b.Bus().PublishOutbound(ctx, msg)
}

// UpdateStatus delegates to the default bridge Manager.
func UpdateStatus(ctx context.Context, req *UpdateStatusRequest) error {
	b, err := Instance()
	if err != nil {
		return err
	}
	return b.UpdateStatus(ctx, req)
}

// EditMessage delegates to the default bridge Manager.
func EditMessage(ctx context.Context, req *EditMessageRequest) error {
	b, err := Instance()
	if err != nil {
		return err
	}
	return b.EditMessage(ctx, req)
}

// ConsumeInbound consumes from the default bridge bus.
func ConsumeInbound(ctx context.Context) (InboundMessage, error) {
	b, err := Instance()
	if err != nil {
		return InboundMessage{}, err
	}
	return b.Bus().ConsumeInbound(ctx)
}

// Reply replies via the default bridge.
func Reply(ctx context.Context, in *InboundMessage, text, mediaPath string) error {
	b, err := Instance()
	if err != nil {
		return err
	}
	return b.Reply(ctx, in, text, mediaPath)
}

// Media returns the default bridge media backend; nil if not initialized.
func Media() media.Backend {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	if defaultBridge == nil {
		return nil
	}
	return defaultBridge.Media()
}
