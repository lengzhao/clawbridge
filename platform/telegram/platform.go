// Package telegram implements platform/telegram for AgentKit (migrated from cc-connect semantics).
package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/lengzhao/agentkit"
	"github.com/lengzhao/agentkit/cap/ingress"
	"github.com/lengzhao/agentkit/cap/workspace"
	"github.com/lengzhao/agentkit/runtime/platform/common"
	"github.com/lengzhao/pluginkit"
)

const Kind = "platform/telegram"

// Config 对齐 cc-connect telegram options（JSON 字段供 pluginkit 使用）。
type Config struct {
	Token                 string `json:"token"`
	AllowFrom             string `json:"allowFrom"`
	GroupReplyAll         bool   `json:"groupReplyAll"`
	ShareSessionInChannel bool   `json:"shareSessionInChannel"`
	ProxyURL              string `json:"proxyUrl"`
}

// Deps 为 AgentKit 注入能力。
type Deps struct {
	InboundPolicy ingress.Policy    `json:"inboundPolicy,omitempty"`
	Commands      agentkit.Commands `json:"commands,omitempty"`
	Workspace     workspace.Service `json:"workspace,omitempty"`
}

type delivery struct {
	chatID   int64
	threadID int
}

// Platform 长轮询 Telegram Bot API（文本与常见媒体类型）。
type Platform struct {
	cfg      Config
	deps     Deps
	inbox    *common.Inbox
	dedup    common.MessageDedup
	outbound *common.Outbound
	routes   sync.Map // agentkit.SessionID -> delivery

	mu         sync.RWMutex
	bot        *tgbot.Bot
	selfUser   *models.User
	instance   string
	httpClient *http.Client
	startOnce  sync.Once
	runCancel  context.CancelFunc
}

var (
	_ agentkit.Platform               = (*Platform)(nil)
	_ agentkit.PlatformIdentifier     = (*Platform)(nil)
	_ agentkit.PlatformInstanceSetter = (*Platform)(nil)
)

func init() { pluginkit.Register(Kind, New) }

func New(cfg Config, deps Deps) (agentkit.Platform, error) {
	cfg.Token = strings.TrimSpace(cfg.Token)
	if cfg.Token == "" {
		return nil, errors.New("telegram: token is required")
	}
	common.WarnAllowFromEmpty("telegram", cfg.AllowFrom)
	httpClient := &http.Client{Timeout: 90 * time.Second}
	if u := strings.TrimSpace(cfg.ProxyURL); u != "" {
		parsed, err := url.Parse(u)
		if err != nil {
			return nil, fmt.Errorf("telegram: invalid proxy URL: %w", err)
		}
		httpClient.Transport = &http.Transport{Proxy: http.ProxyURL(parsed)}
		slog.Info("telegram: using proxy", "host", parsed.Host)
	}
	p := &Platform{
		cfg:        cfg,
		deps:       deps,
		inbox:      common.NewInbox(64),
		httpClient: httpClient,
	}
	p.outbound = common.NewOutbound(p.sendText, p.sendMediaPart)
	return p, nil
}

func (*Platform) PlatformID() string { return "telegram" }

func (p *Platform) SetPlatformInstance(instance string) {
	p.mu.Lock()
	p.instance = instance
	p.mu.Unlock()
}

func (p *Platform) platformInstance() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.instance != "" {
		return p.instance
	}
	return "telegram"
}

func (p *Platform) Receive(ctx context.Context) (agentkit.MessageEvent, error) {
	p.startOnce.Do(func() {
		runCtx, cancel := context.WithCancel(context.Background())
		p.runCancel = cancel
		go p.run(runCtx)
	})
	return p.inbox.Receive(ctx)
}

func (p *Platform) Send(ctx context.Context, event agentkit.OutboundEvent) error {
	return p.outbound.Handle(ctx, event)
}

func (p *Platform) run(ctx context.Context) {
	handler := func(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
		if update == nil || update.Message == nil {
			return
		}
		p.handleMessage(ctx, update.Message)
	}
	opts := []tgbot.Option{
		tgbot.WithDefaultHandler(handler),
		tgbot.WithNotAsyncHandlers(),
		tgbot.WithHTTPClient(60*time.Second, p.httpClient),
	}
	b, err := tgbot.New(p.cfg.Token, opts...)
	if err != nil {
		slog.Error("telegram: bot init", "error", err)
		return
	}
	meCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	me, err := b.GetMe(meCtx)
	cancel()
	if err != nil {
		slog.Error("telegram: getMe", "error", err)
		return
	}
	p.mu.Lock()
	p.bot = b
	p.selfUser = me
	p.mu.Unlock()
	drainPendingUpdates(p.cfg.Token, p.httpClient)
	slog.Info("telegram: polling", "username", me.Username)
	b.Start(ctx)
}

func (p *Platform) sendText(ctx context.Context, session agentkit.SessionID, text string) error {
	raw, ok := p.routes.Load(session)
	if !ok {
		return fmt.Errorf("telegram: no delivery for session %q", session)
	}
	d := raw.(delivery)
	p.mu.RLock()
	bot := p.bot
	p.mu.RUnlock()
	if bot == nil {
		return errors.New("telegram: bot not connected")
	}
	params := &tgbot.SendMessageParams{ChatID: d.chatID, Text: text}
	if d.threadID != 0 {
		params.MessageThreadID = d.threadID
	}
	_, err := bot.SendMessage(ctx, params)
	return err
}

func drainPendingUpdates(token string, client *http.Client) {
	if client == nil {
		client = http.DefaultClient
	}
	apiURL := "https://api.telegram.org/bot" + token + "/getUpdates?offset=-1&timeout=0"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
}
