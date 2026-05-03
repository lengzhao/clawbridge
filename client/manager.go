package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/config"
	"github.com/lengzhao/clawbridge/media"
)

// Manager owns driver instances and routes outbound messages.
type Manager struct {
	bus   *bus.MessageBus
	media media.Backend

	mu             sync.RWMutex
	drivers        map[string]Driver
	outboundNotify OutboundSendNotify
}

// ManagerOption configures [NewManager].
type ManagerOption func(*Manager)

// WithOutboundSendNotify registers a callback after each [Driver.Send] (success or failure).
// On failure you may republish with [bus.MessageBus.PublishOutbound] when info.Err is a transient error.
func WithOutboundSendNotify(n OutboundSendNotify) ManagerOption {
	return func(m *Manager) {
		m.outboundNotify = n
	}
}

// NewManager creates an empty manager.
func NewManager(b *bus.MessageBus, m media.Backend, opts ...ManagerOption) *Manager {
	mgr := &Manager{
		bus:     b,
		media:   m,
		drivers: make(map[string]Driver),
	}
	for _, opt := range opts {
		opt(mgr)
	}
	return mgr
}

// StartClients builds and starts drivers for enabled client configs.
func (m *Manager) StartClients(ctx context.Context, clients []config.ClientConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, cc := range clients {
		if !cc.Enabled {
			continue
		}
		if cc.ID == "" {
			return fmt.Errorf("client: empty id")
		}
		f, ok := getFactory(cc.Driver)
		if !ok {
			return fmt.Errorf("%w: %q", ErrUnknownDriver, cc.Driver)
		}
		deps := Deps{Bus: m.bus, Media: m.media}
		d, err := f(ctx, cc, deps)
		if err != nil {
			return fmt.Errorf("client %q: %w", cc.ID, err)
		}
		if err := d.Start(ctx); err != nil {
			return fmt.Errorf("client %q start: %w", cc.ID, err)
		}
		m.drivers[cc.ID] = d
	}
	return nil
}

// StopClients stops all drivers.
func (m *Manager) StopClients(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, d := range m.drivers {
		if err := d.Stop(ctx); err != nil {
			slog.Error("driver stop", "client_id", id, "err", err)
		}
		delete(m.drivers, id)
	}
}

// HandleOutbound delivers one message to the driver for msg.ClientID.
func (m *Manager) HandleOutbound(ctx context.Context, msg *bus.OutboundMessage) error {
	m.mu.RLock()
	d, ok := m.drivers[msg.ClientID]
	m.mu.RUnlock()
	if !ok {
		slog.Error("outbound: unknown client_id", "client_id", msg.ClientID)
		return fmt.Errorf("unknown client_id %q", msg.ClientID)
	}

	return deliverOutboundOnce(ctx, m.outboundNotify, msg, func() (string, error) {
		return d.Send(ctx, msg)
	})
}

// UpdateStatus dispatches to the driver if it implements MessageStatusUpdater.
func (m *Manager) UpdateStatus(ctx context.Context, req *bus.UpdateStatusRequest) error {
	if req == nil {
		return errors.New("client: nil UpdateStatusRequest")
	}
	m.mu.RLock()
	d, ok := m.drivers[req.ClientID]
	m.mu.RUnlock()
	if !ok {
		slog.Error("update status: unknown client_id", "client_id", req.ClientID)
		return fmt.Errorf("unknown client_id %q", req.ClientID)
	}
	u, ok := d.(MessageStatusUpdater)
	if !ok {
		return ErrCapabilityUnsupported
	}
	if err := u.UpdateStatus(ctx, req); err != nil {
		slog.Error("update status", "client_id", req.ClientID, "err", err)
		return err
	}
	return nil
}

// RequiredOutboundMetadataKeysForSend returns keys that should be present on OutboundMessage.Metadata
// for Send to succeed when the driver declares extra requirements. The second result is false if
// clientID is unknown or the driver does not implement [OutboundSendMetadataRequirements].
func (m *Manager) RequiredOutboundMetadataKeysForSend(clientID string) ([]string, bool) {
	m.mu.RLock()
	d, ok := m.drivers[clientID]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}
	r, ok := d.(OutboundSendMetadataRequirements)
	if !ok {
		return nil, false
	}
	return r.RequiredOutboundMetadataKeysForSend(), true
}

// EditMessage dispatches to the driver if it implements MessageEditor.
func (m *Manager) EditMessage(ctx context.Context, msg *bus.OutboundMessage) error {
	if msg == nil {
		return errors.New("client: nil OutboundMessage")
	}
	m.mu.RLock()
	d, ok := m.drivers[msg.ClientID]
	m.mu.RUnlock()
	if !ok {
		slog.Error("edit message: unknown client_id", "client_id", msg.ClientID)
		return fmt.Errorf("unknown client_id %q", msg.ClientID)
	}
	e, ok := d.(MessageEditor)
	if !ok {
		return ErrCapabilityUnsupported
	}
	if err := e.EditMessage(ctx, msg); err != nil {
		slog.Error("edit message", "client_id", msg.ClientID, "err", err)
		return err
	}
	return nil
}

// Reply sends a reply for the inbound message. If the driver implements [Replier], that path is used
// and [OutboundSendNotify] is invoked synchronously on success. Otherwise the default outbound
// message is published and delivery follows [HandleOutbound] (notify after Send).
func (m *Manager) Reply(ctx context.Context, in *bus.InboundMessage, text, mediaPath string) (*bus.OutboundMessage, error) {
	if in == nil || (strings.TrimSpace(text) == "" && mediaPath == "") {
		return nil, ErrInvalidReply
	}
	m.mu.RLock()
	d, ok := m.drivers[in.ClientID]
	m.mu.RUnlock()
	if !ok {
		slog.Error("reply: unknown client_id", "client_id", in.ClientID)
		return nil, fmt.Errorf("unknown client_id %q", in.ClientID)
	}
	if r, ok := d.(Replier); ok {
		msg, err := r.Reply(ctx, in, text, mediaPath)
		if err != nil {
			if m.outboundNotify != nil {
				m.outboundNotify(ctx, OutboundSendNotifyInfo{Message: msg, Err: err})
			}
			return nil, err
		}
		if msg == nil {
			err := errors.New("client: Replier returned nil message")
			if m.outboundNotify != nil {
				m.outboundNotify(ctx, OutboundSendNotifyInfo{Message: nil, Err: err})
			}
			return nil, err
		}
		if m.outboundNotify != nil {
			m.outboundNotify(ctx, OutboundSendNotifyInfo{Message: msg, Err: nil, MessageID: msg.MessageID})
		}
		return msg, nil
	}
	msg := DefaultReplyOutbound(in, text, mediaPath)
	if err := m.bus.PublishOutbound(ctx, msg); err != nil {
		return nil, err
	}
	return msg, nil
}
