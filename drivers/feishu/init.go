package feishu

import "github.com/lengzhao/clawbridge/client"

func init() {
	client.RegisterDriver("feishu", New)
	client.RegisterOnboarding("feishu", newFeishuOnboardingFlow, nil)
}
