package client

import (
	"context"

	"github.com/lengzhao/clawbridge/bus"
)

// Driver is one IM client connection (webhook, websocket, etc.).
type Driver interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	// Send delivers msg to the IM API. On success, sentMessageID is the platform message id when
	// known (may be empty). On failure, sentMessageID should be empty.
	Send(ctx context.Context, msg *bus.OutboundMessage) (sentMessageID string, err error)
}

// MessageStatusUpdater updates per-message processing state (optional).
type MessageStatusUpdater interface {
	UpdateStatus(ctx context.Context, req *bus.UpdateStatusRequest) error
}

// MessageEditor edits a previously sent message (optional).
type MessageEditor interface {
	EditMessage(ctx context.Context, req *bus.EditMessageRequest) error
}
