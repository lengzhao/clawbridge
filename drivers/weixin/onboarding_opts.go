package weixin

import (
	"time"
)

// OnboardingOpts holds typed options for [client.Onboarding.Params] when driver is weixin.
// Use [OnboardingOpts.DriverMap] to convert to the map expected by [client.RunOnboarding].
type OnboardingOpts struct {
	HTTPListen string        // http_listen: loopback listen addr for browser QR page (empty disables HTTP helper)
	BaseURL    string        // base_url: iLink API base (empty defaults in driver)
	BotType    string        // bot_type: iLink bot type (empty defaults in driver)
	Proxy      string        // proxy: optional HTTP proxy URL
	Timeout    time.Duration // timeout: wait for scan (zero defaults in driver)
}

// DriverMap returns a map suitable for [client.Onboarding.WithParams].
func (o OnboardingOpts) DriverMap() map[string]any {
	m := map[string]any{}
	if o.HTTPListen != "" {
		m["http_listen"] = o.HTTPListen
	}
	if o.BaseURL != "" {
		m["base_url"] = o.BaseURL
	}
	if o.BotType != "" {
		m["bot_type"] = o.BotType
	}
	if o.Proxy != "" {
		m["proxy"] = o.Proxy
	}
	if o.Timeout != 0 {
		m["timeout"] = o.Timeout.String()
	}
	return m
}
