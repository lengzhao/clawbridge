package feishu

import (
	"github.com/lengzhao/clawbridge/client"
)

func newFeishuOnboardingFlow(map[string]any, *client.OnboardingHooks) (client.OnboardingFlow, error) {
	desc := client.OnboardingDescriptor{
		Driver:      "feishu",
		Kind:        client.OnboardingInstructionsOnly,
		DisplayName: "Feishu / Lark Open Platform",
		HelpURL:     "https://open.feishu.cn/document/",
		Fields: []client.CredentialField{
			{Key: "app_id", Secret: false},
			{Key: "app_secret", Secret: true},
			{Key: "verification_token", Secret: true},
			{Key: "encrypt_key", Secret: true},
			{Key: "is_lark", Secret: false},
			{Key: "allow_from", Secret: false},
			{Key: "group_trigger", Secret: false},
		},
	}
	payload := client.ManualInstructionsPayload(
		[]string{
			"在飞书开放平台（或 Lark 国际版控制台）创建企业自建应用。",
			"在应用凭证页复制 App ID、App Secret；记录 Verification Token；若启用加密消息则配置 Encrypt Key。",
			"为应用开通机器人能力与所需事件/权限（具体以飞书文档为准），完成版本发布与租户安装。",
			"国际版 Lark 将 options.is_lark 设为 true，并使用 https://open.larksuite.com 对应控制台。",
			"将 app_id、app_secret、verification_token 等写入 options；参见 config.example.yaml。",
		},
		[]client.OnboardingLink{
			{Title: "飞书开放平台 — 应用列表", URL: "https://open.feishu.cn/app"},
			{Title: "飞书开放平台 — 文档首页", URL: "https://open.feishu.cn/document/"},
			{Title: "Lark（国际版）— Developer Portal", URL: "https://open.larksuite.com/app"},
		},
	)
	return client.NewManualInstructionsFlow(desc, payload), nil
}
