package weixin

import (
	"github.com/lengzhao/clawbridge/config"
)

// ClientConfigFromOnboard builds one enabled weixin [config.ClientConfig] from credential options
// produced after interactive onboarding ([client.RunOnboarding]). proxyFallback applies when credentialOpts has no proxy.
func ClientConfigFromOnboard(clientID string, credentialOpts map[string]any, allowFrom []string, stateDir, proxyFallback string) config.ClientConfig {
	token := credString(credentialOpts, "token")
	baseURL := credString(credentialOpts, "base_url")
	px := credString(credentialOpts, "proxy")
	if px == "" {
		px = proxyFallback
	}
	opt := map[string]any{
		"token":      token,
		"allow_from": allowFrom,
	}
	if baseURL != "" {
		opt["base_url"] = baseURL
	}
	if px != "" {
		opt["proxy"] = px
	}
	if stateDir != "" {
		opt["state_dir"] = stateDir
	}
	if v := credString(credentialOpts, "ilink_user_id"); v != "" {
		opt["ilink_user_id"] = v
	}
	if v := credString(credentialOpts, "ilink_bot_id"); v != "" {
		opt["ilink_bot_id"] = v
	}
	return config.ClientConfig{
		ID:      clientID,
		Driver:  "weixin",
		Enabled: true,
		Options: opt,
	}
}
