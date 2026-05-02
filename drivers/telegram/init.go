package telegram

import "github.com/lengzhao/clawbridge/client"

func init() {
	client.RegisterDriver("telegram", New)
	client.RegisterOnboarding("telegram", newTelegramOnboardingFlow, nil)
}
