package client

import (
	"fmt"

	"github.com/lengzhao/clawbridge/config"
)

// DefaultOnboardingClientConfig copies credentialOpts into Options and applies AllowFrom / StateDir / Proxy fallback.
func DefaultOnboardingClientConfig(driver, clientID string, credentialOpts map[string]any, spec Onboarding) config.ClientConfig {
	opts := cloneAnyMap(credentialOpts)
	if opts == nil {
		opts = map[string]any{}
	}
	if len(spec.AllowFrom) > 0 {
		opts["allow_from"] = spec.AllowFrom
	}
	if spec.StateDir != "" {
		opts["state_dir"] = spec.StateDir
	}
	if credString(opts, "proxy") == "" && spec.Proxy != "" {
		opts["proxy"] = spec.Proxy
	}
	return config.ClientConfig{
		ID:      clientID,
		Driver:  driver,
		Enabled: true,
		Options: opts,
	}
}

func cloneAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func credString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}
