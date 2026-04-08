package client

import (
	"context"
	"sync"

	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/config"
	"github.com/lengzhao/clawbridge/media"
)

// Factory constructs a driver for one client configuration.
type Factory func(ctx context.Context, cfg config.ClientConfig, deps Deps) (Driver, error)

// Deps are shared services passed to each driver.
type Deps struct {
	Bus   *bus.MessageBus
	Media media.Backend
}

var (
	factories   = make(map[string]Factory)
	factoriesMu sync.RWMutex
)

// RegisterDriver registers a driver implementation under name (e.g. "feishu").
func RegisterDriver(name string, f Factory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[name] = f
}

func getFactory(name string) (Factory, bool) {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()
	f, ok := factories[name]
	return f, ok
}
