package telegram

import (
	"github.com/lengzhao/clawbridge/client"
)

func newTelegramOnboardingFlow(map[string]any, *client.OnboardingHooks) (client.OnboardingFlow, error) {
	desc := client.OnboardingDescriptor{
		Driver:      "telegram",
		Kind:        client.OnboardingInstructionsOnly,
		DisplayName: "Telegram Bot",
		HelpURL:     "https://core.telegram.org/bots/tutorial",
		Fields: []client.CredentialField{
			{Key: "bot_token", Secret: true},
			{Key: "proxy", Secret: false},
			{Key: "base_url", Secret: false},
			{Key: "use_markdown_v2", Secret: false},
			{Key: "allow_from", Secret: false},
			{Key: "group_trigger", Secret: false},
		},
	}
	payload := client.ManualInstructionsPayload(
		[]string{
			"在 Telegram 中打开 @BotFather。",
			"发送 /newbot（或选择现有机器人的 API Token）。",
			"按提示创建机器人后，复制 BotFather 提供的 HTTP API token（形如 123456:ABC-DEF…）。",
			"将 token 写入该 client 的 options.bot_token；可选 proxy、base_url（自建 Bot API）、allow_from、group_trigger 等，参见 config.example.yaml。",
		},
		[]client.OnboardingLink{
			{Title: "Telegram — BotFather(@BotFather)", URL: "https://t.me/BotFather"},
			{Title: "Telegram Bots — 入门教程", URL: "https://core.telegram.org/bots/tutorial"},
			{Title: "Bot API 文档", URL: "https://core.telegram.org/bots/api"},
		},
	)
	return client.NewManualInstructionsFlow(desc, payload), nil
}
