package bus

import "errors"

var (
	// ErrClosed is returned when operating on a closed bus.
	ErrClosed = errors.New("bus: closed")
	// ErrInvalidOutbound is returned when an outbound message fails validation.
	ErrInvalidOutbound = errors.New("bus: invalid outbound message")
	// ErrNilMessage is returned when a nil pointer is passed to PublishInbound.
	ErrNilMessage = errors.New("bus: nil inbound message")
)
