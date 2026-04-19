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
// Use OutboundMessage fields (ClientID, To, MessageID, Text, Parts, Metadata, ThreadID, ReplyToID as needed);
// Send ignores MessageID but EditMessage uses it to target the platform message.
type MessageEditor interface {
	EditMessage(ctx context.Context, msg *bus.OutboundMessage) error
}

// Replier handles Reply without going through the outbound queue (optional).
// Implementations may set Metadata, ThreadID, or custom fields; return the outbound message that was sent
// (MessageID set when the platform id is known). If not implemented, the Manager falls back to
// PublishOutbound with [DefaultReplyOutbound]. Built-in drivers typically delegate to [Driver.Send].
type Replier interface {
	Reply(ctx context.Context, in *bus.InboundMessage, text, mediaPath string) (*bus.OutboundMessage, error)
}
