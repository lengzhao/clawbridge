package weixin

import (
	"github.com/lengzhao/clawbridge/client"
	"github.com/lengzhao/clawbridge/config"
)

func init() {
	client.RegisterDriver("weixin", New)
	client.RegisterOnboarding("weixin", newWeixinOnboardingFlow, onboardingClientBuilder)
}

func onboardingClientBuilder(clientID string, cred map[string]any, spec client.Onboarding) config.ClientConfig {
	return ClientConfigFromOnboard(clientID, cred, spec.AllowFrom, spec.StateDir, spec.Proxy)
}
