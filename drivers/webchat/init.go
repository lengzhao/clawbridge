package webchat

import "github.com/lengzhao/clawbridge/client"

func init() {
	client.RegisterDriver("webchat", New)
	client.RegisterOnboarding("webchat", newWebchatOnboardingFlow, nil)
}
