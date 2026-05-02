package slack

import "github.com/lengzhao/clawbridge/client"

func init() {
	client.RegisterDriver("slack", New)
	client.RegisterOnboarding("slack", newSlackOnboardingFlow, nil)
}
