// Package noop registers a no-op driver for tests and scaffolding.
package noop

import (
	"context"

	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/client"
	"github.com/lengzhao/clawbridge/config"
)

func init() {
	client.RegisterDriver("noop", New)
}

type driver struct {
	id string
}

// New builds a driver that accepts messages and does not touch the network.
func New(ctx context.Context, cfg config.ClientConfig, deps client.Deps) (client.Driver, error) {
	_ = ctx
	_ = deps
	return &driver{id: cfg.ID}, nil
}

func (d *driver) Name() string { return d.id }

func (d *driver) Start(ctx context.Context) error {
	_ = ctx
	return nil
}

func (d *driver) Stop(ctx context.Context) error {
	_ = ctx
	return nil
}

func (d *driver) Send(ctx context.Context, msg *bus.OutboundMessage) (string, error) {
	_ = ctx
	_ = msg
	return "", nil
}

func (d *driver) Reply(ctx context.Context, in *bus.InboundMessage, text, mediaPath string) (*bus.OutboundMessage, error) {
	msg := client.DefaultReplyOutbound(in, text, mediaPath)
	id, err := d.Send(ctx, msg)
	if err != nil {
		return nil, err
	}
	msg.MessageID = id
	return msg, nil
}

var _ client.Replier = (*driver)(nil)
