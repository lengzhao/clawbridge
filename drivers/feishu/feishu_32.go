//go:build !amd64 && !arm64 && !riscv64 && !mips64 && !ppc64

package feishu

import (
	"context"
	"errors"

	"github.com/lengzhao/clawbridge/client"
	"github.com/lengzhao/clawbridge/config"
)

// New returns an error on 32-bit architectures where the Feishu SDK is not supported.
func New(ctx context.Context, cfg config.ClientConfig, deps client.Deps) (client.Driver, error) {
	_ = ctx
	_ = cfg
	_ = deps
	return nil, errors.New("feishu: driver requires a 64-bit architecture (amd64, arm64, riscv64, mips64, or ppc64)")
}
