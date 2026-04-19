package clawbridge

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/client"
	"github.com/lengzhao/clawbridge/config"
	"github.com/lengzhao/clawbridge/media"
)

// Bridge wires the message bus, media backend, and client manager.
type Bridge struct {
	bus     *bus.MessageBus
	media   media.Backend
	mgr     *client.Manager
	cfg     config.Config
	mu      sync.Mutex
	started bool

	dispatchCancel context.CancelFunc
	dispatchWG     sync.WaitGroup
}

// New constructs a bridge without starting drivers or the outbound dispatcher.
func New(cfg config.Config, opts ...Option) (*Bridge, error) {
	var o bridgeOptions
	for _, opt := range opts {
		opt(&o)
	}

	var backend media.Backend
	var err error
	if o.media != nil {
		backend = o.media
	} else {
		backend, err = media.NewLocalBackend(cfg.Media.Root)
		if err != nil {
			return nil, err
		}
	}

	msgBus := bus.NewMessageBus()
	var mgrOpts []client.ManagerOption
	if o.outboundNotify != nil {
		mgrOpts = append(mgrOpts, client.WithOutboundSendNotify(o.outboundNotify))
	}
	mgr := client.NewManager(msgBus, backend, mgrOpts...)

	return &Bridge{
		bus:   msgBus,
		media: backend,
		mgr:   mgr,
		cfg:   cfg,
	}, nil
}

// Start launches enabled drivers and the outbound dispatch loop.
func (b *Bridge) Start(ctx context.Context) error {
	b.mu.Lock()
	if b.started {
		b.mu.Unlock()
		return fmt.Errorf("clawbridge: already started")
	}
	b.started = true
	ctxDispatch, cancel := context.WithCancel(ctx)
	b.dispatchCancel = cancel
	b.mu.Unlock()

	if err := b.mgr.StartClients(ctx, b.cfg.Clients); err != nil {
		cancel()
		b.mu.Lock()
		b.started = false
		b.dispatchCancel = nil
		b.mu.Unlock()
		return err
	}

	b.dispatchWG.Add(1)
	go func() {
		defer b.dispatchWG.Done()
		err := b.bus.RunOutboundDispatch(ctxDispatch, b.mgr.HandleOutbound)
		if err != nil && err != context.Canceled && err != bus.ErrClosed {
			slog.Error("outbound dispatch stopped", "err", err)
		}
	}()

	return nil
}

// Stop stops drivers, waits for the dispatcher to finish, and closes the bus.
func (b *Bridge) Stop(ctx context.Context) error {
	b.mu.Lock()
	if !b.started {
		b.mu.Unlock()
		return nil
	}
	b.started = false
	cancel := b.dispatchCancel
	b.dispatchCancel = nil
	b.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	waitDone := make(chan struct{})
	go func() {
		b.dispatchWG.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-ctx.Done():
		slog.Warn("bridge stop: dispatch wait interrupted", "err", ctx.Err())
	}

	stopCtx := context.Background()
	if ctx != nil {
		stopCtx = ctx
	}
	b.mgr.StopClients(stopCtx)
	b.bus.Close()
	return nil
}

// Media returns the configured media backend.
func (b *Bridge) Media() media.Backend {
	return b.media
}

// Bus returns the message bus.
func (b *Bridge) Bus() *bus.MessageBus {
	return b.bus
}

// UpdateStatus updates per-message state for the given inbound message: ClientID, To, and
// MessageID are taken from in (same routing as Reply). metadata may be nil.
func (b *Bridge) UpdateStatus(ctx context.Context, in *InboundMessage, state UpdateStatusState, metadata map[string]string) error {
	if in == nil || strings.TrimSpace(string(state)) == "" {
		return ErrInvalidMessage
	}
	req := &UpdateStatusRequest{
		ClientID:  in.ClientID,
		To:        Recipient{SessionID: in.SessionID, Kind: in.Peer.Kind},
		MessageID: in.MessageID,
		State:     string(state),
		Metadata:  metadata,
	}
	return b.mgr.UpdateStatus(ctx, req)
}

// UpdateStatusRequest updates per-message state with a fully specified request (e.g. when
// MessageID is not the same as an InboundMessage.MessageID).
func (b *Bridge) UpdateStatusRequest(ctx context.Context, req *UpdateStatusRequest) error {
	return b.mgr.UpdateStatus(ctx, req)
}

// EditMessage edits a sent message when the target driver implements MessageEditor.
// Fields are taken from out (same shape as PublishOutbound / Reply); MessageID empty means last Send for ClientID+To (§2.2.1).
func (b *Bridge) EditMessage(ctx context.Context, out *OutboundMessage) error {
	if out == nil {
		return ErrInvalidMessage
	}
	req := &EditMessageRequest{
		ClientID:  out.ClientID,
		To:        out.To,
		MessageID: out.MessageID,
		Text:      out.Text,
		Parts:     out.Parts,
		Metadata:  out.Metadata,
	}
	return b.mgr.EditMessage(ctx, req)
}

// EditMessageRequest edits using a fully specified request.
func (b *Bridge) EditMessageRequest(ctx context.Context, req *EditMessageRequest) error {
	return b.mgr.EditMessage(ctx, req)
}

// Reply sends a quick reply derived from an inbound message and returns the outbound message that was queued.
func (b *Bridge) Reply(ctx context.Context, in *InboundMessage, text, mediaPath string) (*OutboundMessage, error) {
	if in == nil || (text == "" && mediaPath == "") {
		return nil, ErrInvalidMessage
	}
	msg := &OutboundMessage{
		ClientID:  in.ClientID,
		To:        Recipient{SessionID: in.SessionID, Kind: in.Peer.Kind},
		Text:      text,
		ReplyToID: in.MessageID,
	}
	if mediaPath != "" {
		msg.Parts = []MediaPart{{Path: mediaPath}}
	}
	if err := b.bus.PublishOutbound(ctx, msg); err != nil {
		return nil, err
	}
	return msg, nil
}
