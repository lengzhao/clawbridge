package client

import "github.com/lengzhao/clawbridge/bus"

// DefaultReplyOutbound builds the same [bus.OutboundMessage] as [Manager.Reply] when the driver
// does not implement [Replier] (ClientID, To, ReplyToID, Text, optional single Part from mediaPath).
func DefaultReplyOutbound(in *bus.InboundMessage, text, mediaPath string) *bus.OutboundMessage {
	msg := &bus.OutboundMessage{
		ClientID:  in.ClientID,
		To:        bus.Recipient{SessionID: in.SessionID, Kind: in.Peer.Kind},
		Text:      text,
		ReplyToID: in.MessageID,
	}
	if mediaPath != "" {
		msg.Parts = []bus.MediaPart{{Path: mediaPath}}
	}
	return msg
}
