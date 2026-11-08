// Package discord implements platform/discord for AgentKit (cc-connect semantics, text MVP).
package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/websocket"
	"github.com/lengzhao/agentkit"
	"github.com/lengzhao/agentkit/cap/ingress"
	"github.com/lengzhao/agentkit/cap/workspace"
	"github.com/lengzhao/agentkit/runtime/platform/common"
	"github.com/lengzhao/pluginkit"
)

const Kind = "platform/discord"

// Config 对齐 cc-connect discord options（JSON 字段供 pluginkit 使用）。
type Config struct {
	Token                      string `json:"token"`
	AllowFrom                  string `json:"allowFrom"`
	ShareSessionInChannel      bool   `json:"shareSessionInChannel"`
	GroupReplyAll              bool   `json:"groupReplyAll"`
	GroupReplyAllGuilds        string `json:"groupReplyAllGuilds"`
	ProxyURL                   string `json:"proxyUrl"`
	RespondToAtEveryoneAndHere bool   `json:"respondToAtEveryoneAndHere"`
}

// Deps 为 AgentKit 注入能力。
type Deps struct {
	InboundPolicy ingress.Policy    `json:"inboundPolicy,omitempty"`
	Commands      agentkit.Commands `json:"commands,omitempty"`
	Workspace     workspace.Service `json:"workspace,omitempty"`
}

// Platform Gateway 长连接，文本入站/出站。
type Platform struct {
	cfg                 Config
	deps                Deps
	inbox               *common.Inbox
	dedup               common.MessageDedup
	outbound            *common.Outbound
	routes              sync.Map
	botRoleIDs          sync.Map
	groupReplyAllGuilds []string

	mu         sync.RWMutex
	session    *discordgo.Session
	botID      string
	instance   string
	proxyURL   *url.URL
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
		return nil, errors.New("discord: token is required")
	}
	common.WarnAllowFromEmpty("discord", cfg.AllowFrom)
	p := &Platform{
		cfg:                 cfg,
		deps:                deps,
		inbox:               common.NewInbox(64),
		groupReplyAllGuilds: parseGroupReplyAllGuilds(cfg.GroupReplyAll, cfg.GroupReplyAllGuilds),
	}
	if u := strings.TrimSpace(cfg.ProxyURL); u != "" {
		parsed, err := url.Parse(u)
		if err != nil {
			return nil, fmt.Errorf("discord: invalid proxy URL: %w", err)
		}
		p.proxyURL = parsed
		slog.Info("discord: using proxy", "host", parsed.Host)
	}
	p.outbound = common.NewOutbound(p.sendText, p.sendMediaPart)
	return p, nil
}

func (*Platform) PlatformID() string { return "discord" }

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
	return "discord"
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
	session, err := p.buildSession()
	if err != nil {
		slog.Error("discord: create session", "error", err)
		return
	}
	p.mu.Lock()
	p.session = session
	p.mu.Unlock()

	if err := session.Open(); err != nil {
		slog.Error("discord: gateway open", "error", err)
		return
	}
	slog.Info("discord: gateway connected")
	<-ctx.Done()
	_ = session.Close()
}

func (p *Platform) buildSession() (*discordgo.Session, error) {
	session, err := discordgo.New("Bot " + p.cfg.Token)
	if err != nil {
		return nil, err
	}
	if p.proxyURL != nil {
		transport := &http.Transport{Proxy: http.ProxyURL(p.proxyURL)}
		session.Client = &http.Client{Transport: transport, Timeout: 60 * time.Second}
		session.Dialer = &websocket.Dialer{Proxy: http.ProxyURL(p.proxyURL)}
	}
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentMessageContent

	session.AddHandler(func(_ *discordgo.Session, r *discordgo.Ready) {
		p.mu.Lock()
		p.botID = r.User.ID
		p.mu.Unlock()
		slog.Info("discord: ready", "user", r.User.Username)
	})
	session.AddHandler(func(s *discordgo.Session, g *discordgo.GuildCreate) {
		if g == nil || g.Unavailable {
			return
		}
		p.cacheBotRoleIDForGuild(s, g.ID, g.Roles)
	})
	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		p.handleMessageCreate(s, m)
	})
	return session, nil
}
