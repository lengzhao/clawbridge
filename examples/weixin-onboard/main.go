// 微信 iLink：扫码登录绑定 + 可选 echo（回复与消息相同文本）。
// 默认无需配置任何连接参数（终端展示二维码，超时等由驱动内置）。
// 进阶 flag（-listen、-proxy 等）的说明写在 main() 里，-h 里不重复长文案。
//
//	go run ./examples/weixin-onboard
//	go run ./examples/weixin-onboard -listen 127.0.0.1:8769   // 可选：浏览器打开二维码页
//	go run ./examples/weixin-onboard -onboard-only -print yaml
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/lengzhao/clawbridge"
	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/client"
	_ "github.com/lengzhao/clawbridge/drivers"
	"gopkg.in/yaml.v3"
)

func main() {

	onboardOnly := flag.Bool("onboard-only", false, "仅完成扫码后退出（不启动 echo）")
	clientID := flag.String("client-id", "weixin-onboard", "clawbridge client id")
	allowFrom := flag.String("allow-from", "*", "允许的发送方，逗号分隔；* 表示不限制")
	stateDir := flag.String("state-dir", "", "可选，持久化 weixin 同步状态目录")
	writeConfig := flag.String("write-config", "", "可选，写入完整 clawbridge YAML（含明文密钥）")
	printMode := flag.String("print", "human", "扫码结束后摘要输出: none | yaml | json | human（走 [client.ReportOnboarding]）")
	printSecrets := flag.Bool("print-secrets", false, "摘要中显示完整密钥（默认脱敏）；写入 -write-config 始终为明文")

	flag.Parse()

	pm, err := client.ParseOnboardingPrintMode(*printMode)
	if err != nil {
		slog.Error("print mode", "err", err)
		os.Exit(2)
	}

	spec := client.NewOnboarding("weixin", *clientID).
		WithStateDir(*stateDir).
		WithAllowFrom(splitAllowFrom(*allowFrom)...)

	res, err := client.RunOnboarding(context.Background(), spec)
	if err != nil {
		slog.Error("onboarding", "err", err)
		os.Exit(1)
	}
	if !res.Ready() {
		slog.Error("onboarding did not produce a runnable client", "phase", res.Phase)
		os.Exit(1)
	}

	client.ReportOnboarding(os.Stdout, pm, res, client.ReportOptions{
		MaskSecrets: !*printSecrets,
		ErrWriter:   os.Stderr,
	})

	if *writeConfig != "" {
		raw, mErr := yaml.Marshal(&res.Config)
		if mErr != nil {
			slog.Error("marshal config", "err", mErr)
			os.Exit(1)
		}
		if wErr := os.WriteFile(*writeConfig, raw, 0o600); wErr != nil {
			slog.Error("write config", "path", *writeConfig, "err", wErr)
			os.Exit(1)
		}
		slog.Info("wrote config", "path", *writeConfig)
	}

	if *onboardOnly {
		return
	}

	slog.Info("echo 模式：向微信机器人发消息将收到原文回复；Ctrl+C 退出")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	b, err := clawbridge.New(res.Config,
		clawbridge.WithOutboundSendNotify(func(ctx context.Context, info clawbridge.OutboundSendNotifyInfo) {
			if info.Err != nil {
				slog.Warn("outbound send failed", "client", info.Message.ClientID, "err", info.Err)
				return
			}
			slog.Info("outbound send ok", "client", info.Message.ClientID, "message_id", info.MessageID)
		}),
	)
	if err != nil {
		slog.Error("clawbridge.New", "err", err)
		os.Exit(1)
	}

	go runEcho(ctx, b)

	if err := b.Start(ctx); err != nil {
		slog.Error("Start", "err", err)
		os.Exit(1)
	}
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_ = b.Stop(shutdown)
	}()

	<-ctx.Done()
	slog.Info("shutdown", "err", ctx.Err())
}

func splitAllowFrom(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"*"}
	}
	return out
}

func runEcho(ctx context.Context, b *clawbridge.Bridge) {
	for {
		in, err := b.Bus().ConsumeInbound(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, bus.ErrClosed) {
				return
			}
			slog.Error("ConsumeInbound", "err", err)
			return
		}
		slog.Info("inbound",
			"client_id", in.ClientID,
			"session_id", in.SessionID,
			"from", in.Sender.DisplayName,
			"text", in.Content,
		)
		reply := in.Content
		mediaPath := ""
		if len(in.MediaPaths) > 0 {
			mediaPath = in.MediaPaths[0]
		}
		if _, err := b.Reply(ctx, &in, reply, mediaPath); err != nil {
			slog.Error("Reply", "err", err)
		}
	}
}
