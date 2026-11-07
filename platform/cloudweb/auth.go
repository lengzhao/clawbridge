package cloudweb

import (
	"context"
	"errors"
	"io"

	"github.com/lengzhao/clawbridge/platform"
	"github.com/lengzhao/pluginkit"
)

// Auth stores a shared Cloud Web bearer token used to reach the gateway.
type Auth struct{}

func init() {
	pluginkit.Register("platform/auth-cloud-web", func() (platform.Authenticator, error) {
		return Auth{}, nil
	})
}

func (Auth) Login(ctx context.Context, req platform.LoginRequest) (platform.LoginResult, error) {
	_ = ctx
	out := req.Out
	if out == nil {
		out = io.Discard
	}
	id := req.ExistingID
	if id == "" {
		id = "cloud_web"
	}
	if req.ReadSecret == nil {
		return platform.LoginResult{}, errors.New("cloud-web login requires a token (secret)")
	}
	secret, err := req.ReadSecret()
	if err != nil {
		return platform.LoginResult{}, err
	}
	if secret == "" {
		return platform.LoginResult{}, errors.New("cloud-web token is empty")
	}
	_, _ = out, req.NonInteractive
	return platform.LoginResult{
		Credentials: map[string]string{"id": id, "secret": secret},
	}, nil
}

func (Auth) Owner(_ context.Context, _ map[string]string, _ map[string]string) (string, error) {
	return "", errors.New("cloud-web has no platform owner; use IM-specific auth or access bind")
}
