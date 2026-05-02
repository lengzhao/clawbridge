package weixin

import (
	"context"
	"fmt"
	"time"

	"github.com/lengzhao/clawbridge/client"
)

type onboardingFlow struct {
	auth       AuthFlowOpts
	httpListen string
	hooks      *client.OnboardingHooks

	api          *ApiClient
	qrResp       *QRCodeResponse
	shutdownHTTP func()
	started      bool
}

func newWeixinOnboardingFlow(opts map[string]any, hooks *client.OnboardingHooks) (client.OnboardingFlow, error) {
	auth := AuthFlowOpts{
		BaseURL: credString(opts, "base_url"),
		BotType: credString(opts, "bot_type"),
		Proxy:   credString(opts, "proxy"),
	}
	if s := credString(opts, "timeout"); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("weixin onboarding: timeout: %w", err)
		}
		auth.Timeout = d
	}
	httpListen := credString(opts, "http_listen")
	return &onboardingFlow{auth: auth, httpListen: httpListen, hooks: hooks}, nil
}

func (f *onboardingFlow) Descriptor() client.OnboardingDescriptor {
	return client.OnboardingDescriptor{
		Driver:      "weixin",
		Kind:        client.OnboardingQRPoll,
		DisplayName: "Weixin iLink Bot",
		HelpURL:     "https://ilinkai.weixin.qq.com/",
		Fields: []client.CredentialField{
			{Key: "token", Secret: true},
			{Key: "base_url", Secret: false},
			{Key: "ilink_user_id", Secret: false},
			{Key: "ilink_bot_id", Secret: false},
			{Key: "proxy", Secret: false},
		},
		ParamsHelp: []client.DriverOptField{
			{Key: "http_listen", Type: "string", DefaultValue: "（空则仅终端二维码）", Description: "本机 HTTP 监听地址，用于浏览器打开二维码页；如 127.0.0.1:0 表示随机端口"},
			{Key: "base_url", Type: "string", DefaultValue: "https://ilinkai.weixin.qq.com/", Description: "iLink OpenAPI 基址"},
			{Key: "bot_type", Type: "string", DefaultValue: "3", Description: "iLink 机器人类型"},
			{Key: "proxy", Type: "string", Description: "可选 HTTP/HTTPS 代理 URL"},
			{Key: "timeout", Type: "duration", DefaultValue: "5m", Description: "等待扫码确认的最长时间（Go duration 字符串）"},
		},
	}
}

func (f *onboardingFlow) Start(ctx context.Context) (*client.OnboardingSession, error) {
	if f.started {
		return nil, fmt.Errorf("weixin onboarding: Start already called")
	}
	normalizeAuthFlowOpts(&f.auth)

	api, err := NewApiClient(f.auth.BaseURL, "", f.auth.Proxy)
	if err != nil {
		return nil, fmt.Errorf("weixin onboarding: api client: %w", err)
	}

	f.hooks.LogInfo("weixin: requesting QR code for interactive login")
	qrResp, err := api.GetQRCode(ctx, f.auth.BotType)
	if err != nil {
		return nil, fmt.Errorf("weixin onboarding: get qrcode: %w", err)
	}

	f.api = api
	f.qrResp = qrResp
	f.started = true

	payload := qrResp.QrcodeImgContent
	sess := &client.OnboardingSession{
		Driver: "weixin",
		Payload: map[string]any{
			"qr_link": payload,
		},
	}

	if f.httpListen != "" {
		listen := f.httpListen
		pageURL, shutdown, httpErr := startWeixinQRHTTPServer(payload, listen)
		if httpErr != nil {
			f.started = false
			f.api = nil
			f.qrResp = nil
			return nil, httpErr
		}
		f.shutdownHTTP = shutdown
		sess.Payload["page_url"] = pageURL
		f.hooks.LogInfo("weixin: local QR page", "url", pageURL)
		f.hooks.BannerLocalPage(pageURL)
	}

	f.hooks.DisplayQR(payload)
	f.hooks.PrintNotify("Waiting for scan...")

	return sess, nil
}

func (f *onboardingFlow) Wait(ctx context.Context, sess *client.OnboardingSession) (map[string]any, error) {
	if !f.started || f.api == nil || f.qrResp == nil {
		return nil, fmt.Errorf("weixin onboarding: Start did not succeed")
	}
	if sess != nil && sess.Driver != "" && sess.Driver != "weixin" {
		return nil, fmt.Errorf("weixin onboarding: session driver mismatch")
	}

	defer func() {
		if f.shutdownHTTP != nil {
			f.shutdownHTTP()
			f.shutdownHTTP = nil
		}
	}()

	timeoutCtx, cancel := context.WithTimeout(ctx, f.auth.Timeout)
	defer cancel()

	token, userID, botID, base, err := pollQRCodeLogin(timeoutCtx, f.auth, f.api, f.qrResp.Qrcode, f.hooks)
	if err != nil {
		return nil, err
	}

	out := map[string]any{
		"token":          token,
		"ilink_user_id":  userID,
		"ilink_bot_id":   botID,
	}
	if base != "" {
		out["base_url"] = base
	}
	if f.auth.Proxy != "" {
		out["proxy"] = f.auth.Proxy
	}
	return out, nil
}
