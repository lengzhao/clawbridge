// Package plugin registers clawbridge platform and auth plugins with pluginkit.
//
// Import this package for side-effect registration, or import individual
// platform/* packages when you only need one implementation.
package plugin

import (
	"fmt"

	"github.com/lengzhao/pluginkit"

	"github.com/lengzhao/clawbridge/credentials"
)

const (
	// KindCredentialsKeyring is the OS credential store slot.
	KindCredentialsKeyring = "clawbridge/credentials-keyring"
	// KindCredentialsMemory is the non-persistent test store.
	KindCredentialsMemory = "clawbridge/credentials-memory"
)

// KeyringConfig configures the keyring credential slot.
type KeyringConfig struct {
	ServicePrefix string `json:"servicePrefix"`
}

// MemoryConfig configures the in-memory credential slot.
type MemoryConfig struct{}

func init() {
	pluginkit.Register(KindCredentialsKeyring, func(cfg KeyringConfig) (credentials.Store, error) {
		if cfg.ServicePrefix == "" {
			return nil, fmt.Errorf("%s requires servicePrefix", KindCredentialsKeyring)
		}
		return credentials.NewKeyringStore(cfg.ServicePrefix), nil
	})
	pluginkit.Register(KindCredentialsMemory, func(_ MemoryConfig) (credentials.Store, error) {
		return credentials.NewMemoryStore(), nil
	})
}
