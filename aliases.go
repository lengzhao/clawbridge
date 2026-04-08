package clawbridge

import (
	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/client"
)

// Root type aliases for package-level API users (single import).
type (
	InboundMessage      = bus.InboundMessage
	OutboundMessage     = bus.OutboundMessage
	Peer                = bus.Peer
	SenderInfo          = bus.SenderInfo
	Recipient           = bus.Recipient
	MediaPart           = bus.MediaPart
	UpdateStatusRequest = bus.UpdateStatusRequest
	EditMessageRequest  = bus.EditMessageRequest

	OutboundSendNotifyInfo = client.OutboundSendNotifyInfo
	OutboundSendNotify     = client.OutboundSendNotify
)

// Message status constants (UpdateStatusRequest.State).
const (
	StatusProcessing = bus.StatusProcessing
	StatusCompleted  = bus.StatusCompleted
	StatusFailed     = bus.StatusFailed
)
