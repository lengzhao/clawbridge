package slack

import (
	"github.com/lengzhao/clawbridge/client"
)

func newSlackOnboardingFlow(map[string]any, *client.OnboardingHooks) (client.OnboardingFlow, error) {
	desc := client.OnboardingDescriptor{
		Driver:      "slack",
		Kind:        client.OnboardingInstructionsOnly,
		DisplayName: "Slack (Socket Mode)",
		HelpURL:     "https://api.slack.com/apis/connections/socket",
		Fields: []client.CredentialField{
			{Key: "bot_token", Secret: true},
			{Key: "app_token", Secret: true},
			{Key: "allow_from", Secret: false},
			{Key: "group_trigger", Secret: false},
		},
	}
	payload := client.ManualInstructionsPayload(
		[]string{
			"本驱动使用 Slack Socket Mode（无需公网入站 URL）。",
			"打开 Slack API 控制台创建（或选择）一个 App。",
			"在「OAuth & Permissions」中按需添加 Bot Token Scopes；安装到工作区，复制 Bot User OAuth Token（xoxb-）。",
			"在「Basic Information」→「App-Level Tokens」创建 Token，Scope 勾选 connections:write，复制 App-Level Token（xapp-）。",
			"在「Socket Mode」中启用 Socket Mode。",
			"将 bot_token、app_token 写入 options；可选 allow_from、group_trigger，参见 config.example.yaml。",
		},
		[]client.OnboardingLink{
			{Title: "Slack API — Your Apps", URL: "https://api.slack.com/apps"},
			{Title: "Socket Mode 概述", URL: "https://api.slack.com/apis/connections/socket"},
			{Title: "启用 Socket Mode", URL: "https://api.slack.com/apis/connections/socket-implement"},
		},
	)
	return client.NewManualInstructionsFlow(desc, payload), nil
}
