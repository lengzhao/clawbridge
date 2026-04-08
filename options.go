package clawbridge

import (
	"github.com/lengzhao/clawbridge/client"
	"github.com/lengzhao/clawbridge/media"
)

type bridgeOptions struct {
	media          media.Backend
	outboundNotify client.OutboundSendNotify
}

// Option configures Bridge construction.
type Option func(*bridgeOptions)

// WithMediaBackend sets a custom media backend; when set, Config.Media is not used to build storage.
func WithMediaBackend(b media.Backend) Option {
	return func(o *bridgeOptions) {
		o.media = b
	}
}

// WithOutboundSendNotify registers a callback after each Driver.Send (success or failure).
// On failure, use [Bridge.Bus].PublishOutbound to republish when appropriate (e.g. errors.Is(err, ErrTemporary)).
func WithOutboundSendNotify(n client.OutboundSendNotify) Option {
	return func(o *bridgeOptions) {
		o.outboundNotify = n
	}
}
