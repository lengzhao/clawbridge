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
	UpdateStatusState   = bus.UpdateStatusState

	OutboundSendNotifyInfo = client.OutboundSendNotifyInfo
	OutboundSendNotify     = client.OutboundSendNotify
	Replier                = client.Replier
)

// Message status string constants (UpdateStatusRequest.State and JSON wire values).
const (
	StatusProcessing = bus.StatusProcessing
	StatusCompleted  = bus.StatusCompleted
	StatusFailed     = bus.StatusFailed
)

// Typed message status for UpdateStatus(in, state, …); prefer these over raw strings.
const (
	UpdateStatusProcessing = bus.UpdateStatusProcessing
	UpdateStatusCompleted  = bus.UpdateStatusCompleted
	UpdateStatusFailed     = bus.UpdateStatusFailed
)
