package noop

import (
	"github.com/lengzhao/clawbridge/client"
)

func newNoopOnboardingFlow(map[string]any, *client.OnboardingHooks) (client.OnboardingFlow, error) {
	desc := client.OnboardingDescriptor{
		Driver:      "noop",
		Kind:        client.OnboardingInstructionsOnly,
		DisplayName: "No-op（占位）",
		HelpURL:     "",
		Fields:      nil,
	}
	payload := client.ManualInstructionsPayload(
		[]string{
			"noop 驱动不连接任何外部 IM，用于测试或占位。",
			"仅需在配置中为该 client 设置 id、driver: noop、enabled: true；options 通常为空。",
		},
		nil,
	)
	return client.NewManualInstructionsFlow(desc, payload), nil
}
