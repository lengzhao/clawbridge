package clawbridge

import (
	"errors"

	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/client"
)

var (
	ErrAlreadyInitialized = errors.New("clawbridge: already initialized")
	ErrNotInitialized     = errors.New("clawbridge: not initialized")

	// ErrInvalidMessage is returned by Reply / UpdateStatus and matches outbound validation failures from the bus.
	ErrInvalidMessage = bus.ErrInvalidOutbound

	ErrNotRunning            = client.ErrNotRunning
	ErrRateLimited           = client.ErrRateLimited
	ErrTemporary             = client.ErrTemporary
	ErrSendFailed            = client.ErrSendFailed
	ErrUnknownDriver         = client.ErrUnknownDriver
	ErrCapabilityUnsupported = client.ErrCapabilityUnsupported
)
