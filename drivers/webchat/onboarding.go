package webchat

import (
	"github.com/lengzhao/clawbridge/client"
)

func newWebchatOnboardingFlow(map[string]any, *client.OnboardingHooks) (client.OnboardingFlow, error) {
	desc := client.OnboardingDescriptor{
		Driver:      "webchat",
		Kind:        client.OnboardingInstructionsOnly,
		DisplayName: "Webchat（本地浏览器 UI）",
		HelpURL:     "",
		Fields: []client.CredentialField{
			{Key: "listen", Secret: false},
			{Key: "path", Secret: false},
			{Key: "display_name", Secret: false},
		},
	}
	payload := client.ManualInstructionsPayload(
		[]string{
			"Webchat 在本机开启 HTTP 服务，浏览器访问即可聊天；无需第三方账号 OAuth。",
			"建议 listen 绑定 127.0.0.1 端口（如 127.0.0.1:8765），path 一般为 /。",
			"将 listen、path、display_name 写入 options；与 Bridge 共用 media 配置以支持附件。",
		},
		nil,
	)
	return client.NewManualInstructionsFlow(desc, payload), nil
}
