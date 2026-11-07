// Package cloudweb implements platform/cloud-web: AgentKit adapter for the
// cc-connect Cloud Web wire protocol. Real IM drivers (telegram, discord, …)
// run in cc-connect (or compatible gateways); this plugin only normalizes JSON.
package cloudweb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/lengzhao/agentkit"
	"github.com/lengzhao/agentkit/cap/ingress"
	"github.com/lengzhao/agentkit/cap/workspace"
	"github.com/lengzhao/agentkit/runtime/platform/common"
	"github.com/lengzhao/pluginkit"
)

const Kind = "platform/cloud-web"

// Config wires one logical cc-connect platform through Cloud Web.
type Config struct {
	// BridgeName is the cc-connect platform id (telegram, discord, feishu, …).
	BridgeName string `json:"bridgeName"`
	Token      string `json:"token"`
	Transport  string `json:"transport"` // websocket | long_poll
	BaseURL    string `json:"baseUrl"`
	WSURL      string `json:"wsUrl"`
	EventsPath string `json:"eventsPath"`
	SendPath   string `json:"sendPath"`
	Project    string `json:"project"`
	// LongPollTimeoutMS is the long-poll wait in milliseconds.
	LongPollTimeoutMS int `json:"longPollTimeoutMs"`
	AllowFrom         string `json:"allowFrom"`
	ShareSessionInChannel bool `json:"shareSessionInChannel"`
	GroupReplyAll         bool `json:"groupReplyAll"`
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.Token) == "" {
		return errors.New("cloud-web requires token")
	}
	c.BridgeName = strings.TrimSpace(c.BridgeName)
	if c.BridgeName == "" {
		c.BridgeName = "cloud_web"
	}
	c.Transport = strings.TrimSpace(c.Transport)
	if c.Transport == "" {
		c.Transport = "websocket"
	}
	switch c.Transport {
	case "websocket", "long_poll":
	default:
		return fmt.Errorf("cloud-web transport must be websocket or long_poll, got %q", c.Transport)
	}
	if c.Transport == "websocket" {
		if resolveWSURL(c.BaseURL, c.WSURL) == "" {
			return errors.New("cloud-web websocket requires wsUrl or baseUrl")
		}
	} else if strings.TrimSpace(c.BaseURL) == "" {
		return errors.New("cloud-web long_poll requires baseUrl")
	}
	return nil
}

// Deps holds optional AgentKit capabilities.
type Deps struct {
	InboundPolicy ingress.Policy    `json:"inboundPolicy"`
	Commands      agentkit.Commands `json:"commands,omitempty"`
	Workspace     workspace.Service `json:"workspace,omitempty"`
}

// Platform adapts Cloud Web JSON to agentkit.MessageEvent.
type Platform struct {
	cfg      Config
	deps     Deps
	tp       transport
	outbound *common.Outbound
	inbox    *common.Inbox
	dedup    messageDedup

	mu           sync.Mutex
	replyCtxs    map[agentkit.SessionID]replyContext
	instance     string
	startOnce    sync.Once
	startErr     error
	runCancel    context.CancelFunc
}

var (
	_ agentkit.Platform                = (*Platform)(nil)
	_ agentkit.PlatformIdentifier      = (*Platform)(nil)
	_ agentkit.PlatformInstanceSetter  = (*Platform)(nil)
)

func init() { pluginkit.Register(Kind, New) }

func New(cfg Config, deps Deps) (agentkit.Platform, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	var tp transport
	switch cfg.Transport {
	case "websocket":
		tp = newWSTransport(resolveWSURL(cfg.BaseURL, cfg.WSURL), cfg.Token, cfg.BridgeName, cfg.Project)
	default:
		tp = newPollTransport(cfg.BaseURL, cfg.Token, cfg.BridgeName, cfg.Project, cfg.EventsPath, cfg.SendPath, cfg.LongPollTimeoutMS)
	}
	p := &Platform{
		cfg:       cfg,
		deps:      deps,
		tp:        tp,
		inbox:     common.NewInbox(64),
		replyCtxs: make(map[agentkit.SessionID]replyContext),
	}
	p.outbound = common.NewOutbound(p.sendText, p.sendUnsupportedMedia)
	return p, nil
}

func (p *Platform) PlatformID() string { return p.cfg.BridgeName }

func (p *Platform) SetPlatformInstance(instance string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.instance = instance
}

func (p *Platform) platformInstance() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.instance != "" {
		return p.instance
	}
	return p.cfg.BridgeName
}

func (p *Platform) ensureStarted() error {
	p.startOnce.Do(func() {
		runCtx, cancel := context.WithCancel(context.Background())
		p.runCancel = cancel
		if err := p.tp.Start(runCtx, p.handleWire); err != nil {
			p.startErr = err
			cancel()
		}
	})
	return p.startErr
}

func (p *Platform) Receive(ctx context.Context) (agentkit.MessageEvent, error) {
	if err := p.ensureStarted(); err != nil {
		return agentkit.MessageEvent{}, err
	}
	return p.inbox.Receive(ctx)
}

func (p *Platform) Send(ctx context.Context, event agentkit.OutboundEvent) error {
	return p.outbound.Handle(ctx, event)
}

func (p *Platform) sendUnsupportedMedia(context.Context, agentkit.SessionID, agentkit.ContentPart) error {
	return errors.New("cloud-web: outbound media not implemented yet")
}

func (p *Platform) sendText(ctx context.Context, session agentkit.SessionID, text string) error {
	rc, ok := p.lookupReply(session)
	if !ok {
		return fmt.Errorf("cloud-web: no reply context for session %q", session)
	}
	return p.tp.Send(ctx, map[string]any{
		"type":        "reply",
		"session_key": rc.SessionKey,
		"reply_ctx":   rc.ReplyCtx,
		"content":     text,
		"format":      "text",
	})
}

func (p *Platform) storeReply(session agentkit.SessionID, rc replyContext) {
	if session == "" || rc.ReplyCtx == "" {
		return
	}
	p.mu.Lock()
	p.replyCtxs[session] = rc
	p.mu.Unlock()
}

func (p *Platform) lookupReply(session agentkit.SessionID) (replyContext, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	rc, ok := p.replyCtxs[session]
	return rc, ok
}

func (p *Platform) handleWire(raw []byte) {
	var base wireMsg
	if err := json.Unmarshal(raw, &base); err != nil {
		slog.Debug("cloud_web: invalid inbound json", "error", err)
		return
	}
	switch base.Type {
	case "message":
		p.handleMessage(raw)
	case "card_action":
		p.handleCardAction(raw)
	case "ping":
		_ = p.tp.Send(context.Background(), map[string]any{"type": "pong", "ts": time.Now().UnixMilli()})
	default:
		slog.Debug("cloud_web: unknown inbound type", "type", base.Type)
	}
}

func (p *Platform) handleMessage(raw []byte) {
	var m wireInboundMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return
	}
	if m.UserID == "" {
		return
	}
	if !allowList(p.cfg.AllowFrom, m.UserID) {
		return
	}
	if isGroupChat(m.ChatType) && !p.cfg.GroupReplyAll {
		if m.Mentioned == nil || !*m.Mentioned {
			return
		}
	}
	hasContent := strings.TrimSpace(m.Content) != ""
	hasAttachments := len(m.Images) > 0 || len(m.Files) > 0 || m.Audio != nil
	if !hasContent && !hasAttachments {
		return
	}
	msgID := m.MsgID
	if msgID == "" {
		msgID = fmt.Sprintf("cw-%d", time.Now().UnixNano())
	}
	if p.dedup.isDuplicate(msgID) {
		return
	}
	sessionKey := buildSessionKey(p.cfg.BridgeName, m.ChatID, m.UserID, p.cfg.ShareSessionInChannel, m.SessionKey)
	replyCtx := m.ReplyCtx
	if replyCtx == "" {
		replyCtx = sessionKey
	}
	delivery := agentkit.SessionID(sessionKey)
	p.storeReply(delivery, replyContext{SessionKey: sessionKey, ReplyCtx: replyCtx})

	route := agentkit.SessionRouteInput{
		Platform:   p.cfg.BridgeName,
		Instance:   p.platformInstance(),
		DeliveryID: delivery,
		ChannelID:  m.ChatID,
		ScopeUserID: m.UserID,
	}
	images := decodeImages(m.Images)
	files := decodeFiles(m.Files)
	audio := decodeAudio(m.Audio)

	imgs := make([]common.ImageAttachment, 0, len(images))
	for _, img := range images {
		imgs = append(imgs, common.ImageAttachment{MimeType: img.MimeType, Data: img.Data, FileName: img.FileName})
	}
	fls := make([]common.FileAttachment, 0, len(files))
	for _, f := range files {
		fls = append(fls, common.FileAttachment{MimeType: f.MimeType, Data: f.Data, FileName: f.FileName})
	}
	var aud *common.AudioAttachment
	if audio != nil {
		aud = &common.AudioAttachment{MimeType: audio.MimeType, Data: audio.Data, Format: audio.Format, Duration: audio.Duration}
	}
	wsOpts := common.InboundOptsFor(p.deps.Workspace)
	event := common.InboundFromContent("", route, m.UserID, m.Content, "", imgs, fls, aud, nil, wsOpts)
	meta := map[string]any{
		"sender_id":   m.UserID,
		"sender_name": firstNonEmpty(m.UserName, m.UserID),
		"chat_id":     m.ChatID,
		"platform":    p.cfg.BridgeName,
		"message_id":  msgID,
	}
	if m.ChatType != "" {
		meta["chat_type"] = m.ChatType
	}
	event = common.WithMetadata(event, meta)

	ctx := context.Background()
	if p.deps.InboundPolicy != nil {
		rawPayload, _ := json.Marshal(m)
		in := ingress.Input{
			Platform: p.cfg.BridgeName, InstanceID: p.platformInstance(), Kind: ingress.Message,
			ID: msgID, ChatID: m.ChatID, UserID: m.UserID,
			ConversationKind: chatKind(m.ChatType),
			Raw: rawPayload,
			Message:          &ingress.MessageFacts{HasText: hasContent, AddressedToAgent: true},
			Default:          ingress.Plan{Agent: ingress.Submit, Handlers: true},
		}
		decision, err := p.deps.InboundPolicy.Evaluate(ctx, in)
		if err != nil {
			slog.Error("cloud_web: inbound policy", "error", err)
			return
		}
		plan, err := ingress.Resolve(in, decision)
		if err != nil || plan.Rejected || plan.Agent == ingress.Skip {
			return
		}
	}
	if err := p.inbox.Push(ctx, event); err != nil {
		slog.Debug("cloud_web: inbox push", "error", err)
	}
}

func chatKind(chatType string) string {
	if isGroupChat(chatType) {
		return "group"
	}
	return "direct"
}

func (p *Platform) handleCardAction(raw []byte) {
	var ca wireCardAction
	if err := json.Unmarshal(raw, &ca); err != nil || ca.SessionKey == "" {
		return
	}
	delivery := agentkit.SessionID(ca.SessionKey)
	replyCtx := ca.ReplyCtx
	if replyCtx == "" {
		if rc, ok := p.lookupReply(delivery); ok {
			replyCtx = rc.ReplyCtx
		}
	}
	p.storeReply(delivery, replyContext{SessionKey: ca.SessionKey, ReplyCtx: replyCtx})

	switch {
	case strings.HasPrefix(ca.Action, "perm:"):
		text := permissionText(ca.Action)
		if text == "" {
			return
		}
		p.pushSynthetic(delivery, ca.SessionKey, text, true)
	case strings.HasPrefix(ca.Action, "cmd:"):
		p.pushSynthetic(delivery, ca.SessionKey, strings.TrimPrefix(ca.Action, "cmd:"), false)
	default:
		slog.Debug("cloud_web: card_action", "action", ca.Action)
	}
}

func permissionText(action string) string {
	switch action {
	case "perm:allow":
		return "allow"
	case "perm:deny":
		return "deny"
	case "perm:allow_all":
		return "allow all"
	default:
		return ""
	}
}

func (p *Platform) pushSynthetic(delivery agentkit.SessionID, sessionKey, content string, permission bool) {
	route := agentkit.SessionRouteInput{
		Platform:   p.cfg.BridgeName,
		Instance:   p.platformInstance(),
		DeliveryID: delivery,
		ChannelID:  sessionKey,
		ScopeUserID: "system",
	}
	event := common.InboundFromContent("", route, "system", content, "", nil, nil, nil, nil, nil)
	meta := map[string]any{"sender_id": "system", "platform": p.cfg.BridgeName}
	if permission {
		meta["permission_response"] = true
	}
	event = common.WithMetadata(event, meta)
	_ = p.inbox.Push(context.Background(), event)
}
