package telegram

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/lengzhao/agentkit"
	"github.com/lengzhao/agentkit/cap/ingress"
	"github.com/lengzhao/agentkit/runtime/platform/common"
)

func (p *Platform) handleMessage(ctx context.Context, msg *models.Message) {
	msgTime := time.Unix(int64(msg.Date), 0)
	if msgTime.Before(common.ProcessStartTime.Add(-2 * time.Second)) {
		return
	}
	if msg.From == nil {
		return
	}

	isGroup := msg.Chat.Type == models.ChatTypeGroup || msg.Chat.Type == models.ChatTypeSupergroup
	threadID := 0
	if msg.Chat.IsForum || !isGroup {
		threadID = msg.MessageThreadID
	}
	userID := strconv.FormatInt(msg.From.ID, 10)
	if !common.AllowList(p.cfg.AllowFrom, userID) {
		return
	}
	if isGroup && !p.cfg.GroupReplyAll && !p.isDirectedAtBot(msg) {
		return
	}

	msgID := strconv.Itoa(msg.ID)
	if p.dedup.IsDuplicate("telegram:" + msgID) {
		return
	}

	deliveryID := buildSessionKey(p.cfg.ShareSessionInChannel, msg.Chat.ID, threadID, msg.From.ID)
	p.routes.Store(deliveryID, delivery{chatID: msg.Chat.ID, threadID: threadID})

	botName := p.botUsername()
	kind := "direct"
	if isGroup {
		kind = "group"
	}
	userName := msg.From.Username
	if userName == "" {
		userName = strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
	}
	route := agentkit.SessionRouteInput{
		Platform:    "telegram",
		Instance:    p.platformInstance(),
		DeliveryID:  deliveryID,
		ChannelID:   channelID(msg.Chat.ID, threadID),
		ScopeUserID: userID,
		ThreadID:    strconv.Itoa(threadID),
	}
	wsOpts := common.InboundOptsFor(p.deps.Workspace)

	if len(msg.Photo) > 0 {
		best := msg.Photo[len(msg.Photo)-1]
		imgData, err := p.downloadFile(ctx, best.FileID)
		if err != nil {
			slog.Error("telegram: download photo", "error", err)
			return
		}
		caption := stripBotMention(msg.Caption, botName)
		images := []common.ImageAttachment{{MimeType: "image/jpeg", Data: imgData}}
		p.emitInbound(ctx, msg, route, userID, userName, kind, msgID, caption, images, nil, nil, wsOpts)
		return
	}

	if msg.Voice != nil {
		audioData, err := p.downloadFile(ctx, msg.Voice.FileID)
		if err != nil {
			slog.Error("telegram: download voice", "error", err)
			return
		}
		audio := &common.AudioAttachment{
			MimeType: msg.Voice.MimeType,
			Data:     audioData,
			Format:   "ogg",
			Duration: msg.Voice.Duration,
		}
		p.emitInbound(ctx, msg, route, userID, userName, kind, msgID, "", nil, nil, audio, wsOpts)
		return
	}

	if msg.Audio != nil {
		audioData, err := p.downloadFile(ctx, msg.Audio.FileID)
		if err != nil {
			slog.Error("telegram: download audio", "error", err)
			return
		}
		format := "mp3"
		if parts := strings.SplitN(msg.Audio.MimeType, "/", 2); len(parts) == 2 && parts[1] != "" {
			format = parts[1]
		}
		audio := &common.AudioAttachment{
			MimeType: msg.Audio.MimeType,
			Data:     audioData,
			Format:   format,
			Duration: msg.Audio.Duration,
		}
		p.emitInbound(ctx, msg, route, userID, userName, kind, msgID, "", nil, nil, audio, wsOpts)
		return
	}

	if msg.Document != nil {
		fileData, err := p.downloadFile(ctx, msg.Document.FileID)
		if err != nil {
			slog.Error("telegram: download document", "error", err)
			return
		}
		caption := stripBotMention(msg.Caption, botName)
		files := []common.FileAttachment{{
			MimeType: msg.Document.MimeType,
			Data:     fileData,
			FileName: msg.Document.FileName,
		}}
		p.emitInbound(ctx, msg, route, userID, userName, kind, msgID, caption, nil, files, nil, wsOpts)
		return
	}

	if strings.TrimSpace(msg.Text) == "" {
		return
	}
	text := stripBotMention(msg.Text, botName)
	p.emitInbound(ctx, msg, route, userID, userName, kind, msgID, text, nil, nil, nil, wsOpts)
}

func (p *Platform) emitInbound(
	ctx context.Context,
	msg *models.Message,
	route agentkit.SessionRouteInput,
	userID, userName, kind, msgID, text string,
	images []common.ImageAttachment,
	files []common.FileAttachment,
	audio *common.AudioAttachment,
	wsOpts *common.InboundOpts,
) {
	event := common.InboundFromContent("", route, userID, text, "", images, files, audio, nil, wsOpts)
	meta := map[string]any{
		"sender_id": userID, "sender_name": userName, "chat_id": route.ChannelID,
		"platform": "telegram", "message_id": msgID, "chat_type": kind,
	}
	event = common.WithMetadata(event, meta)

	if p.deps.InboundPolicy != nil {
		raw, _ := json.Marshal(msg)
		hasText := strings.TrimSpace(text) != ""
		hasAttach := len(images) > 0 || len(files) > 0 || audio != nil
		in := ingress.Input{
			Platform: "telegram", InstanceID: p.platformInstance(), Kind: ingress.Message,
			ID: msgID, ChatID: route.ChannelID, UserID: userID, ConversationKind: kind,
			Raw: raw,
			Message: &ingress.MessageFacts{
				HasText:          hasText,
				AttachmentOnly:   hasAttach && !hasText,
				AddressedToAgent: true,
			},
			Default: ingress.Plan{Agent: ingress.Submit, Handlers: true},
		}
		decision, err := p.deps.InboundPolicy.Evaluate(ctx, in)
		if err != nil {
			slog.Error("telegram: inbound policy", "error", err)
			return
		}
		plan, err := ingress.Resolve(in, decision)
		if err != nil || plan.Rejected || plan.Agent == ingress.Skip {
			return
		}
	}
	if err := p.inbox.Push(ctx, event); err != nil {
		slog.Debug("telegram: inbox push", "error", err)
	}
}

func (p *Platform) botUsername() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.selfUser != nil {
		return p.selfUser.Username
	}
	return ""
}
