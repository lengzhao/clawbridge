// Package config holds YAML/JSON-friendly configuration structs shared by clawbridge and client.
package config

// Config is the top-level bridge configuration.
type Config struct {
	Media   MediaConfig    `json:"media" yaml:"media"`
	Clients []ClientConfig `json:"clients" yaml:"clients"`
}

// MediaConfig configures the built-in local media backend when no custom Backend is injected.
type MediaConfig struct {
	Root string `json:"root,omitempty" yaml:"root,omitempty"`
}

// ClientConfig describes one IM client instance.
type ClientConfig struct {
	ID          string            `json:"id" yaml:"id"`
	Driver      string            `json:"driver" yaml:"driver"`
	Enabled     bool              `json:"enabled" yaml:"enabled"`
	Credentials map[string]any    `json:"credentials,omitempty" yaml:"credentials,omitempty"`
	HTTP        *HTTPListenConfig `json:"http,omitempty" yaml:"http,omitempty"`
}

// HTTPListenConfig is optional webhook / HTTP server binding for a client.
type HTTPListenConfig struct {
	Listen string `json:"listen" yaml:"listen"`
	Path   string `json:"path" yaml:"path"`
}
