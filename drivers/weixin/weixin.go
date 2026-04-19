// Package weixin 提供微信 iLink Bot（长轮询）驱动，自 PicoClaw channels/weixin 迁移。
package weixin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/client"
	"github.com/lengzhao/clawbridge/config"
	"github.com/lengzhao/clawbridge/media"
)

type driver struct {
	id                string
	api               *ApiClient
	bus               *bus.MessageBus
	mediab            media.Backend
	allow             []string
	cdnBase           string
	ctx               context.Context
	cancel            context.CancelFunc
	run               atomic.Bool
	contextTokens     sync.Map
	typingMu          sync.Mutex
	typingCache       map[string]typingTicketCacheEntry
	pauseMu           sync.Mutex
	pauseUntil        time.Time
	syncBufPath       string
	contextTokensPath string
}

// New 构造 weixin 驱动。必填 options.token 或 bot_token；可选 base_url、proxy、allow_from、state_dir、cdn_base_url。
func New(ctx context.Context, cfg config.ClientConfig, deps client.Deps) (client.Driver, error) {
	_ = ctx
	cred := cfg.Options
	if cred == nil {
		cred = map[string]any{}
	}
	token := credString(cred, "token")
	if token == "" {
		token = credString(cred, "bot_token")
	}
	if token == "" {
		return nil, fmt.Errorf("weixin: token or bot_token is required")
	}
	baseURL := credString(cred, "base_url")
	if baseURL == "" {
		baseURL = "https://ilinkai.weixin.qq.com/"
	}
	api, err := NewApiClient(baseURL, token, credString(cred, "proxy"))
	if err != nil {
		return nil, fmt.Errorf("weixin: api client: %w", err)
	}

	stateRoot := credString(cred, "state_dir")
	if stateRoot == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			cache = os.TempDir()
		}
		stateRoot = filepath.Join(cache, "clawbridge", "weixin", cfg.ID)
	}
	accountKey := genWeixinAccountKey(baseURL, token)

	d := &driver{
		id:                cfg.ID,
		api:               api,
		bus:               deps.Bus,
		mediab:            deps.Media,
		allow:             credStringSlice(cred, "allow_from"),
		cdnBase:           credString(cred, "cdn_base_url"),
		typingCache:       make(map[string]typingTicketCacheEntry),
		syncBufPath:       buildWeixinSyncBufPath(stateRoot, accountKey),
		contextTokensPath: buildWeixinContextTokensPath(stateRoot, accountKey),
	}
	if len(d.allow) == 0 {
		slog.Warn("weixin driver: allow_from is empty; all senders accepted (set allow_from or use '*' explicitly)",
			"client_id", cfg.ID)
	}
	return d, nil
}

func (d *driver) Name() string { return d.id }

func (d *driver) Start(ctx context.Context) error {
	if d.bus == nil {
		return fmt.Errorf("weixin: message bus is nil")
	}
	slog.Info("weixin driver starting", "client_id", d.id)
	d.ctx, d.cancel = context.WithCancel(ctx)
	d.run.Store(true)
	d.restoreContextTokens()
	go d.pollLoop(d.ctx)
	return nil
}

func (d *driver) Stop(ctx context.Context) error {
	_ = ctx
	d.run.Store(false)
	if d.cancel != nil {
		d.cancel()
	}
	slog.Info("weixin driver stopped", "client_id", d.id)
	return nil
}

func (d *driver) restoreContextTokens() {
	tokens, err := loadContextTokens(d.contextTokensPath)
	if err != nil {
		slog.Warn("weixin: load context tokens", "path", d.contextTokensPath, "err", err)
		return
	}
	for userID, token := range tokens {
		d.contextTokens.Store(userID, token)
	}
	if len(tokens) > 0 {
		slog.Info("weixin: restored context tokens", "client_id", d.id, "count", len(tokens))
	}
}

func (d *driver) persistContextTokens() {
	tokens := make(map[string]string)
	d.contextTokens.Range(func(k, v any) bool {
		if userID, ok := k.(string); ok {
			if token, ok := v.(string); ok {
				tokens[userID] = token
			}
		}
		return true
	})
	if err := saveContextTokens(d.contextTokensPath, tokens); err != nil {
		slog.Warn("weixin: save context tokens", "path", d.contextTokensPath, "err", err)
	}
}

func (d *driver) pollLoop(ctx context.Context) {
	const (
		defaultPollTimeoutMs = 35_000
		retryDelay           = 2 * time.Second
		backoffDelay         = 30 * time.Second
		maxConsecutiveFails  = 3
	)

	consecutiveFails := 0
	getUpdatesBuf, err := loadGetUpdatesBuf(d.syncBufPath)
	if err != nil {
		slog.Warn("weixin: load get_updates_buf", "path", d.syncBufPath, "err", err)
		getUpdatesBuf = ""
	} else if getUpdatesBuf != "" {
		slog.Info("weixin: resumed get_updates_buf", "client_id", d.id, "bytes", len(getUpdatesBuf))
	}
	nextTimeoutMs := defaultPollTimeoutMs

	for {
		select {
		case <-ctx.Done():
			slog.Info("weixin poll loop stopped", "client_id", d.id)
			return
		default:
		}

		if err := d.waitWhileSessionPaused(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}

		pollCtx, pollCancel := context.WithTimeout(ctx, time.Duration(nextTimeoutMs+5000)*time.Millisecond)
		resp, err := d.api.GetUpdates(pollCtx, GetUpdatesReq{GetUpdatesBuf: getUpdatesBuf})
		pollCancel()

		if err != nil {
			if ctx.Err() != nil {
				return
			}
			consecutiveFails++
			slog.Warn("weixin getUpdates failed", "client_id", d.id, "attempt", consecutiveFails, "err", err)
			if consecutiveFails >= maxConsecutiveFails {
				consecutiveFails = 0
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoffDelay):
				}
			} else {
				select {
				case <-ctx.Done():
					return
				case <-time.After(retryDelay):
				}
			}
			continue
		}

		if isSessionExpiredStatus(resp.Ret, resp.Errcode) {
			remaining := d.pauseSession("getupdates", resp.Ret, resp.Errcode, resp.Errmsg)
			select {
			case <-ctx.Done():
				return
			case <-time.After(remaining):
			}
			continue
		}

		if resp.Errcode != 0 || resp.Ret != 0 {
			consecutiveFails++
			slog.Error("weixin getUpdates API error", "client_id", d.id,
				"ret", resp.Ret, "errcode", resp.Errcode, "errmsg", resp.Errmsg)
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryDelay):
			}
			continue
		}

		consecutiveFails = 0
		if resp.LongpollingTimeoutMs > 0 {
			nextTimeoutMs = resp.LongpollingTimeoutMs
		}
		if resp.GetUpdatesBuf != "" {
			getUpdatesBuf = resp.GetUpdatesBuf
			if err := saveGetUpdatesBuf(d.syncBufPath, getUpdatesBuf); err != nil {
				slog.Warn("weixin: persist get_updates_buf", "err", err)
			}
		}
		for _, msg := range resp.Msgs {
			d.handleInboundMessage(ctx, msg)
		}
	}
}

func (d *driver) handleInboundMessage(ctx context.Context, msg WeixinMessage) {
	fromUserID := msg.FromUserID
	if fromUserID == "" {
		return
	}

	messageID := msg.ClientID
	if messageID == "" {
		messageID = uuid.New().String()
	}

	var parts []string
	for _, item := range msg.ItemList {
		switch item.Type {
		case MessageItemTypeText:
			if item.TextItem != nil && item.TextItem.Text != "" {
				parts = append(parts, item.TextItem.Text)
			}
		case MessageItemTypeVoice:
			if item.VoiceItem != nil && item.VoiceItem.Text != "" {
				parts = append(parts, item.VoiceItem.Text)
			} else {
				parts = append(parts, "[audio]")
			}
		case MessageItemTypeImage:
			parts = append(parts, "[image]")
		case MessageItemTypeFile:
			if item.FileItem != nil && item.FileItem.FileName != "" {
				parts = append(parts, fmt.Sprintf("[file: %s]", item.FileItem.FileName))
			} else {
				parts = append(parts, "[file]")
			}
		case MessageItemTypeVideo:
			parts = append(parts, "[video]")
		}
	}

	var mediaRefs []string
	if mediaItem := selectInboundMediaItem(msg); mediaItem != nil {
		ref, err := d.downloadMediaFromItem(ctx, fromUserID, messageID, mediaItem)
		if err != nil {
			slog.Error("weixin: download inbound media", "from", fromUserID, "msg", messageID,
				"type", mediaItem.Type, "err", err)
		} else if ref != "" {
			mediaRefs = append(mediaRefs, ref)
		}
	}

	content := strings.Join(parts, "\n")
	if content == "" && len(mediaRefs) == 0 {
		return
	}

	sender := bus.SenderInfo{
		Platform:    "weixin",
		PlatformID:  fromUserID,
		CanonicalID: buildCanonicalID("weixin", fromUserID),
		Username:    fromUserID,
		DisplayName: fromUserID,
	}

	if !isAllowedSender(sender, d.allow) {
		slog.Debug("weixin: rejected by allowlist", "from_user_id", fromUserID)
		return
	}

	metadata := map[string]string{
		"from_user_id":  fromUserID,
		"context_token": msg.ContextToken,
		"session_id":    msg.SessionID,
	}

	if msg.ContextToken != "" {
		d.contextTokens.Store(fromUserID, msg.ContextToken)
		d.persistContextTokens()
	}

	d.publishInbound(ctx, fromUserID, messageID, content, mediaRefs, sender, metadata)
}

func (d *driver) publishInbound(ctx context.Context, chatID, messageID, content string, mediaPaths []string, sender bus.SenderInfo, metadata map[string]string) {
	in := &bus.InboundMessage{
		ClientID:   d.id,
		SessionID:  chatID,
		MessageID:  messageID,
		Sender:     sender,
		Peer:       bus.Peer{Kind: "direct", ID: chatID},
		Content:    content,
		MediaPaths: mediaPaths,
		ReceivedAt: time.Now().Unix(),
		Metadata:   metadata,
	}
	if err := d.bus.PublishInbound(ctx, in); err != nil {
		slog.Error("weixin: publish inbound", "client_id", d.id, "err", err)
	}
}

func (d *driver) Send(ctx context.Context, msg *bus.OutboundMessage) (string, error) {
	if !d.run.Load() {
		return "", client.ErrNotRunning
	}
	if msg == nil {
		return "", fmt.Errorf("weixin: nil message")
	}
	if err := d.ensureSessionActive(); err != nil {
		return "", err
	}

	toUserID := strings.TrimSpace(msg.To.SessionID)
	if toUserID == "" {
		return "", nil
	}

	contextToken := ""
	if ct, ok := d.contextTokens.Load(toUserID); ok {
		contextToken, _ = ct.(string)
	}
	if contextToken == "" {
		return "", fmt.Errorf("weixin: missing context token for chat %s: %w", toUserID, client.ErrSendFailed)
	}

	if strings.TrimSpace(msg.Text) != "" {
		if err := d.sendTextMessage(ctx, toUserID, contextToken, msg.Text); err != nil {
			slog.Error("weixin: send text", "to", toUserID, "err", err)
			if d.remainingPause() > 0 {
				return "", fmt.Errorf("weixin send: %w", client.ErrSendFailed)
			}
			return "", fmt.Errorf("weixin send: %w: %w", err, client.ErrTemporary)
		}
	}

	if len(msg.Parts) > 0 {
		if err := d.sendMediaParts(ctx, toUserID, contextToken, msg.Parts); err != nil {
			return "", err
		}
	}

	return "", nil
}

func (d *driver) Reply(ctx context.Context, in *bus.InboundMessage, text, mediaPath string) (*bus.OutboundMessage, error) {
	msg := client.DefaultReplyOutbound(in, text, mediaPath)
	id, err := d.Send(ctx, msg)
	if err != nil {
		return nil, err
	}
	msg.MessageID = id
	return msg, nil
}

var _ client.Replier = (*driver)(nil)
