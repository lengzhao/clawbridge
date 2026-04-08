package bus

import (
	"context"
	"sync/atomic"
)

const defaultBuf = 64

// MessageBus decouples drivers from the host via inbound and outbound channels.
type MessageBus struct {
	inbound  chan InboundMessage
	outbound chan OutboundMessage
	done     chan struct{}
	closed   atomic.Bool
}

// NewMessageBus creates a bus with buffered channels.
func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:  make(chan InboundMessage, defaultBuf),
		outbound: make(chan OutboundMessage, defaultBuf),
		done:     make(chan struct{}),
	}
}

// PublishInbound is called by drivers when a message arrives.
func (b *MessageBus) PublishInbound(ctx context.Context, msg *InboundMessage) error {
	if b.closed.Load() {
		return ErrClosed
	}
	if msg == nil {
		return ErrNilMessage
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.done:
		return ErrClosed
	case b.inbound <- *msg:
		return nil
	}
}

// ConsumeInbound blocks until a message is available, ctx is cancelled, or the bus is closed.
func (b *MessageBus) ConsumeInbound(ctx context.Context) (InboundMessage, error) {
	select {
	case <-ctx.Done():
		return InboundMessage{}, ctx.Err()
	case <-b.done:
		return InboundMessage{}, ErrClosed
	case msg, ok := <-b.inbound:
		if !ok {
			return InboundMessage{}, ErrClosed
		}
		return msg, nil
	}
}

// PublishOutbound enqueues a message for delivery to the target client driver.
func (b *MessageBus) PublishOutbound(ctx context.Context, msg *OutboundMessage) error {
	if b.closed.Load() {
		return ErrClosed
	}
	if err := validateOutbound(msg); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.done:
		return ErrClosed
	case b.outbound <- *msg:
		return nil
	}
}

func validateOutbound(msg *OutboundMessage) error {
	if msg == nil {
		return ErrInvalidOutbound
	}
	if msg.ClientID == "" || msg.To.ChatID == "" {
		return ErrInvalidOutbound
	}
	if msg.Text == "" && len(msg.Parts) == 0 {
		return ErrInvalidOutbound
	}
	for _, p := range msg.Parts {
		if p.Path == "" {
			return ErrInvalidOutbound
		}
	}
	return nil
}

// RunOutboundDispatch invokes h for each outbound message until ctx is done or the bus is closed.
// The handler is typically client.Manager.HandleOutbound (one Driver.Send per delivery). If h returns an error,
// the error is ignored so the loop keeps running; use application logs or OutboundSendNotify for failures.
func (b *MessageBus) RunOutboundDispatch(ctx context.Context, h func(context.Context, *OutboundMessage) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-b.done:
			return ErrClosed
		case msg, ok := <-b.outbound:
			if !ok {
				return ErrClosed
			}
			m := msg
			if err := h(ctx, &m); err != nil {
				continue
			}
		}
	}
}

// Close signals shutdown. After Close, publishers return ErrClosed.
func (b *MessageBus) Close() {
	if !b.closed.CompareAndSwap(false, true) {
		return
	}
	close(b.done)
	// Do not close inbound/outbound chans to avoid races with concurrent senders; drain is optional for later.
}
