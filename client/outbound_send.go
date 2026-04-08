package client

import (
	"context"
	"log/slog"

	"github.com/lengzhao/clawbridge/bus"
)

// OutboundSendNotifyInfo is passed to [OutboundSendNotify] after each [Driver.Send] completes.
// When Err is nil, MessageID is the id returned by the driver (may be empty if the platform does not provide one).
// When Err is non-nil, MessageID is empty.
type OutboundSendNotifyInfo struct {
	Message   *bus.OutboundMessage
	Err       error
	MessageID string
}

// OutboundSendNotify is called after every Send attempt (success or failure).
type OutboundSendNotify func(ctx context.Context, info OutboundSendNotifyInfo)

func deliverOutboundOnce(ctx context.Context, notify OutboundSendNotify, msg *bus.OutboundMessage, send func() (string, error)) error {
	mid, err := send()
	if notify != nil {
		info := OutboundSendNotifyInfo{Message: msg, Err: err}
		if err == nil {
			info.MessageID = mid
		}
		notify(ctx, info)
	}
	if err != nil {
		slog.Error("outbound send", "client_id", msg.ClientID, "err", err)
	}
	return err
}
