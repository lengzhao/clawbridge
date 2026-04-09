// Package config holds YAML/JSON-friendly configuration structs shared by clawbridge and client.
package config

// Config is the top-level bridge configuration.
type Config struct {
	Media   MediaConfig    `json:"media" yaml:"media"`
	Clients []ClientConfig `json:"clients" yaml:"clients"`
}

// MediaConfig configures the built-in local media backend when no custom Backend is injected.
type MediaConfig struct {
	// Root 为本地媒体根目录。省略、空串或仅空白 → os.TempDir()/clawbridge（见 media.NewLocalBackend）；
	// YAML 中不写整块 media 亦为同样默认。
	Root string `json:"root,omitempty" yaml:"root,omitempty"`
}

// ClientConfig describes one IM client instance.
// Options 为各 driver 私有的键值配置（令牌、监听地址、触发规则等）；命名区别于仅含密钥的 “credentials”。
type ClientConfig struct {
	ID      string         `json:"id" yaml:"id"`
	Driver  string         `json:"driver" yaml:"driver"`
	Enabled bool           `json:"enabled" yaml:"enabled"`
	Options map[string]any `json:"options,omitempty" yaml:"options,omitempty"`
}
