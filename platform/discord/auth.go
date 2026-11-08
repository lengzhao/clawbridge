package discord

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/lengzhao/clawbridge/platform"
	"github.com/lengzhao/pluginkit"
)

// Auth stores the bot token for setup flows.
type Auth struct{}

func init() {
	pluginkit.Register("platform/auth-discord", func() (platform.Authenticator, error) {
		return Auth{}, nil
	})
}

func (Auth) Login(ctx context.Context, req platform.LoginRequest) (platform.LoginResult, error) {
	_ = ctx
	out := req.Out
	if out == nil {
		out = io.Discard
	}
	if req.ReadSecret == nil {
		return platform.LoginResult{}, errors.New("discord: bot token required")
	}
	secret, err := req.ReadSecret()
	if err != nil {
		return platform.LoginResult{}, err
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return platform.LoginResult{}, errors.New("discord: token is empty")
	}
	id := strings.TrimSpace(req.ExistingID)
	if id == "" {
		id = "discord"
	}
	return platform.LoginResult{Credentials: map[string]string{"id": id, "secret": secret}}, nil
}

func (Auth) Owner(_ context.Context, _ map[string]string, _ map[string]string) (string, error) {
	return "", errors.New("discord: no owner identity; use allowFrom or access bind")
}
