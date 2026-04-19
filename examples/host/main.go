// 示例：宿主进程如何加载配置、启动 clawbridge、消费入站并回复/出站。
// 运行（在仓库根目录）: go run ./examples/host -config examples/host/config.yaml
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/lengzhao/clawbridge"
	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/config"
	_ "github.com/lengzhao/clawbridge/drivers"
	"gopkg.in/yaml.v3"
)

func main() {
	cfgPath := flag.String("config", "examples/host/config.yaml", "clawbridge YAML 配置路径")
	duration := flag.Duration("duration", 0, "运行多久后退出（0 表示仅收到中断信号时退出，便于本地试跑加 -duration=3s）")
	flag.Parse()

	raw, err := os.ReadFile(*cfgPath)
	if err != nil {
		slog.Error("read config", "path", *cfgPath, "err", err)
		os.Exit(1)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		slog.Error("parse yaml", "err", err)
		os.Exit(1)
	}

	b, err := clawbridge.New(cfg,
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

	base := context.Background()
	var ctx context.Context
	var stop context.CancelFunc
	if *duration > 0 {
		ctx, stop = context.WithTimeout(base, *duration)
	} else {
		ctx, stop = signal.NotifyContext(base, os.Interrupt)
	}
	defer stop()

	go runHost(ctx, b)

	if err := b.Start(ctx); err != nil {
		slog.Error("Start", "err", err)
		os.Exit(1)
	}
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = b.Stop(shutdown)
	}()

	// 演示入站：真实环境由 feishu/slack 等 driver 调用 PublishInbound；此处模拟一条消息。
	demoIn := &clawbridge.InboundMessage{
		ClientID:  firstEnabledClientID(cfg),
		SessionID: "demo-chat",
		MessageID: "demo-msg-1",
		Sender: clawbridge.SenderInfo{
			CanonicalID: "noop:demo-user",
			DisplayName: "Demo",
		},
		Peer:    clawbridge.Peer{Kind: "direct", ID: "demo-user"},
		Content: "ping",
	}
	if demoIn.ClientID == "" {
		slog.Error("no enabled client in config")
		os.Exit(1)
	}
	if err := b.Bus().PublishInbound(ctx, demoIn); err != nil {
		slog.Error("PublishInbound", "err", err)
		os.Exit(1)
	}

	// 等宿主处理完入站与 Reply，再演示一条主动出站，避免与 Reply 在队列里顺序打乱日志阅读。
	time.Sleep(100 * time.Millisecond)

	// 演示主动出站（非 Reply）
	_ = b.Bus().PublishOutbound(ctx, &clawbridge.OutboundMessage{
		ClientID: demoIn.ClientID,
		To:       clawbridge.Recipient{SessionID: demoIn.SessionID, Kind: demoIn.Peer.Kind},
		Text:     "主动一条（noop 会吞掉，不连外网）",
	})

	<-ctx.Done()
	slog.Info("shutdown", "err", ctx.Err())
}

func firstEnabledClientID(cfg config.Config) string {
	for _, c := range cfg.Clients {
		if c.Enabled {
			return c.ID
		}
	}
	return ""
}

func runHost(ctx context.Context, b *clawbridge.Bridge) {
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
		reply := "pong: " + in.Content
		mediaPath := ""
		if len(in.MediaPaths) > 0 {
			mediaPath = in.MediaPaths[0]
		}
		if _, err := b.Reply(ctx, &in, reply, mediaPath); err != nil {
			slog.Error("Reply", "err", err)
		}
	}
}
