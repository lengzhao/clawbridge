package client

import "errors"

var (
	ErrNotRunning            = errors.New("clawbridge: client not running")
	ErrRateLimited           = errors.New("clawbridge: rate limited")
	ErrTemporary             = errors.New("clawbridge: temporary failure")
	ErrSendFailed            = errors.New("clawbridge: send failed")
	ErrUnknownDriver         = errors.New("clawbridge: unknown driver")
	ErrCapabilityUnsupported = errors.New("clawbridge: driver does not support this capability")
	ErrInvalidReply          = errors.New("clawbridge: invalid reply parameters")
)
