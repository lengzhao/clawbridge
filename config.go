package clawbridge

import "github.com/lengzhao/clawbridge/config"

// Config types are aliases so callers can use clawbridge.Config with JSON/YAML decoders.
type (
	Config       = config.Config
	MediaConfig  = config.MediaConfig
	ClientConfig = config.ClientConfig
)
