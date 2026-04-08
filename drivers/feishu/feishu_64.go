//go:build amd64 || arm64 || riscv64 || mips64 || ppc64

package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkdispatcher "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/client"
	"github.com/lengzhao/clawbridge/config"
	"github.com/lengzhao/clawbridge/media"
)

const errCodeTenantTokenInvalid = 99991663

var (
	errNotRunning = errors.New("feishu: driver not running")
	errTemporary  = errors.New("feishu: temporary API error")
)

type driver struct {
	id       string
	bus      *bus.MessageBus
	mediab   media.Backend
	lastSent *boundedLastSent
	appID    string
	secret   string
	encKey   string
	verify   string
	isLark   bool
	allow    []string
	group    groupTrigger

	client     *lark.Client
	wsClient   *larkws.Client
	tokenCache *tokenCache
	botOpenID  atomic.Value

	mu     sync.Mutex
	cancel context.CancelFunc
	run    atomic.Bool
}

// New builds a Feishu driver from client options (websocket long connection).
//
// Expected options keys: app_id, app_secret, encrypt_key, verification_token (strings);
// optional is_lark (bool), allow_from ([]string or comma string), group_trigger ({ mention_only, prefixes }).
// 完整 YAML 示例见仓库 docs/public-api.md（「飞书 driver 配置示例」小节）。
func New(ctx context.Context, cfg config.ClientConfig, deps client.Deps) (client.Driver, error) {
	_ = ctx
	cred := cfg.Options
	if cred == nil {
		cred = map[string]any{}
	}
	d := &driver{
		id:         cfg.ID,
		bus:        deps.Bus,
		mediab:     deps.Media,
		lastSent:   newBoundedLastSent(),
		appID:      credString(cred, "app_id"),
		secret:     credString(cred, "app_secret"),
		encKey:     credString(cred, "encrypt_key"),
		verify:     credString(cred, "verification_token"),
		isLark:     credBool(cred, "is_lark"),
		allow:      credStringSlice(cred, "allow_from"),
		group:      credGroupTrigger(cred, "group_trigger"),
		tokenCache: newTokenCache(),
	}
	opts := []lark.ClientOptionFunc{lark.WithTokenCache(d.tokenCache)}
	if d.isLark {
		opts = append(opts, lark.WithOpenBaseUrl(lark.LarkBaseUrl))
	}
	d.client = lark.NewClient(d.appID, d.secret, opts...)

	if len(d.allow) == 0 {
		slog.Warn("feishu driver: allow_from is empty; all senders are accepted (set allow_from or use '*' explicitly)",
			"client_id", cfg.ID)
	}
	return d, nil
}

func (d *driver) Name() string { return d.id }

func (d *driver) Start(ctx context.Context) error {
	if d.appID == "" || d.secret == "" {
		return fmt.Errorf("feishu: app_id or app_secret is empty")
	}
	if d.bus == nil {
		return fmt.Errorf("feishu: message bus is nil")
	}

	if err := d.fetchBotOpenID(ctx); err != nil {
		slog.Warn("feishu: bot open_id fetch failed; @mention detection in groups may not work",
			"client_id", d.id, "err", err)
	}

	dispatcher := larkdispatcher.NewEventDispatcher(d.verify, d.encKey).
		OnP2MessageReceiveV1(d.handleMessageReceive)

	runCtx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.cancel = cancel
	domain := lark.FeishuBaseUrl
	if d.isLark {
		domain = lark.LarkBaseUrl
	}
	d.wsClient = larkws.NewClient(
		d.appID,
		d.secret,
		larkws.WithEventHandler(dispatcher),
		larkws.WithDomain(domain),
	)
	ws := d.wsClient
	d.mu.Unlock()

	d.run.Store(true)
	slog.Info("feishu driver started (websocket)", "client_id", d.id)

	go func() {
		if err := ws.Start(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("feishu websocket stopped", "client_id", d.id, "err", err)
		}
	}()
	return nil
}

func (d *driver) Stop(ctx context.Context) error {
	_ = ctx
	d.mu.Lock()
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	d.wsClient = nil
	d.mu.Unlock()
	d.run.Store(false)
	slog.Info("feishu driver stopped", "client_id", d.id)
	return nil
}

func (d *driver) Send(ctx context.Context, msg *bus.OutboundMessage) (string, error) {
	if !d.run.Load() {
		return "", errNotRunning
	}
	chatID := msg.To.ChatID
	if chatID == "" {
		return "", fmt.Errorf("feishu: empty chat_id: %w", errTemporary)
	}

	if msg.Text != "" {
		cardContent, err := buildMarkdownCard(msg.Text)
		if err != nil {
			if err2 := d.sendText(ctx, msg.To, msg.Text); err2 != nil {
				return "", err2
			}
		} else if err := d.sendCard(ctx, msg.To, cardContent); err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "11310") {
				slog.Warn("feishu: card send failed (table limit), falling back to text",
					"client_id", d.id, "chat_id", chatID)
				if err2 := d.sendText(ctx, msg.To, msg.Text); err2 != nil {
					return "", err2
				}
			} else {
				return "", err
			}
		}
	}

	for _, part := range msg.Parts {
		if err := d.sendMediaPart(ctx, msg.To, part); err != nil {
			return "", err
		}
	}
	mid, _ := d.lastSent.get(msg.To)
	return mid, nil
}

func (d *driver) UpdateStatus(ctx context.Context, req *bus.UpdateStatusRequest) error {
	_ = ctx
	_ = req
	return client.ErrCapabilityUnsupported
}

func (d *driver) EditMessage(ctx context.Context, req *bus.EditMessageRequest) error {
	if !d.run.Load() {
		return errNotRunning
	}
	mid := req.MessageID
	if mid == "" {
		var ok bool
		mid, ok = d.lastSent.get(req.To)
		if !ok || mid == "" {
			return bus.ErrInvalidOutbound
		}
	}
	if len(req.Parts) > 0 {
		return fmt.Errorf("feishu: edit Parts: %w", client.ErrCapabilityUnsupported)
	}
	if req.Text == "" {
		return bus.ErrInvalidOutbound
	}
	content, _ := json.Marshal(map[string]string{"text": req.Text})
	up := larkim.NewUpdateMessageReqBuilder().
		MessageId(mid).
		Body(larkim.NewUpdateMessageReqBodyBuilder().
			MsgType(larkim.MsgTypeText).
			Content(string(content)).
			Build()).
		Build()
	resp, err := d.client.Im.V1.Message.Update(ctx, up)
	if err != nil {
		return fmt.Errorf("feishu edit message: %w: %w", err, errTemporary)
	}
	if !resp.Success() {
		d.invalidateTokenOnAuthError(resp.Code)
		return fmt.Errorf("feishu edit api error (code=%d msg=%s): %w", resp.Code, resp.Msg, errTemporary)
	}
	return nil
}

func (d *driver) sendMediaPart(ctx context.Context, to bus.Recipient, part bus.MediaPart) error {
	rc, err := d.mediab.Open(ctx, part.Path)
	if err != nil {
		slog.Error("feishu: open media locator", "client_id", d.id, "path", part.Path, "err", err)
		return fmt.Errorf("feishu: open media: %w", err)
	}
	defer rc.Close()

	tmp, err := os.CreateTemp("", "feishu-out-*")
	if err != nil {
		return fmt.Errorf("feishu: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("feishu: copy media: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	f, err := os.Open(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer func() {
		f.Close()
		_ = os.Remove(tmpPath)
	}()

	filename := part.Filename
	if filename == "" {
		filename = filepath.Base(part.Path)
	}
	if filename == "" || filename == "." {
		filename = "file"
	}

	if isImagePart(part) {
		return d.sendImage(ctx, to, f)
	}
	ft := partContentTypeToFeishuFileType(part.ContentType, filename)
	return d.sendFile(ctx, to, f, filename, ft)
}

func partContentTypeToFeishuFileType(ct, filename string) string {
	ct = strings.ToLower(ct)
	switch {
	case strings.HasPrefix(ct, "audio/"):
		return "audio"
	case strings.HasPrefix(ct, "video/"):
		return "video"
	default:
		ext := strings.ToLower(filepath.Ext(filename))
		switch ext {
		case ".opus", ".ogg", ".mp3", ".wav", ".m4a":
			return "audio"
		case ".mp4", ".webm", ".mov":
			return "video"
		default:
			return "file"
		}
	}
}

func isImagePart(p bus.MediaPart) bool {
	if strings.HasPrefix(strings.ToLower(p.ContentType), "image/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(p.Path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return true
	default:
		return false
	}
}

func (d *driver) handleMessageReceive(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return nil
	}
	message := event.Event.Message
	sender := event.Event.Sender

	chatID := stringValue(message.ChatId)
	if chatID == "" {
		return nil
	}
	senderID := extractFeishuSenderID(sender)
	if senderID == "" {
		senderID = "unknown"
	}
	messageType := stringValue(message.MessageType)
	messageID := stringValue(message.MessageId)
	rawContent := stringValue(message.Content)

	senderInfo := bus.SenderInfo{
		Platform:    "feishu",
		PlatformID:  senderID,
		CanonicalID: buildCanonicalID("feishu", senderID),
	}
	if !isAllowedSender(senderInfo, d.allow) {
		return nil
	}

	content := extractContent(messageType, rawContent)

	var mediaPaths []string
	if d.mediab != nil && messageID != "" {
		mediaPaths = d.downloadInboundMedia(ctx, chatID, messageID, messageType, rawContent)
	}

	if messageType == larkim.MsgTypeInteractive {
		_, externalURLs := extractCardImageKeys(rawContent)
		mediaPaths = append(mediaPaths, externalURLs...)
	}

	content = appendMediaTags(content, messageType, mediaPaths)
	if content == "" {
		content = "[empty message]"
	}

	metadata := map[string]string{}
	if messageID != "" {
		metadata["message_id"] = messageID
	}
	if messageType != "" {
		metadata["message_type"] = messageType
	}
	chatType := stringValue(message.ChatType)
	if chatType != "" {
		metadata["chat_type"] = chatType
	}
	if sender != nil && sender.TenantKey != nil {
		metadata["tenant_key"] = *sender.TenantKey
	}

	var peer bus.Peer
	if chatType == "p2p" {
		peer = bus.Peer{Kind: "direct", ID: senderID}
	} else {
		peer = bus.Peer{Kind: "group", ID: chatID}
		isMentioned := d.isBotMentioned(message)
		if len(message.Mentions) > 0 {
			content = stripMentionPlaceholders(content, message.Mentions)
		}
		respond, cleaned := shouldRespondInGroup(d.group, isMentioned, content)
		if !respond {
			return nil
		}
		content = cleaned
	}

	slog.Info("feishu inbound", "client_id", d.id, "sender_id", senderID, "chat_id", chatID, "message_id", messageID)

	in := bus.InboundMessage{
		Channel:    d.id,
		ChatID:     chatID,
		MessageID:  messageID,
		Sender:     senderInfo,
		Peer:       peer,
		Content:    content,
		MediaPaths: mediaPaths,
		ReceivedAt: time.Now().Unix(),
		Metadata:   metadata,
	}
	if err := d.bus.PublishInbound(ctx, &in); err != nil {
		slog.Error("feishu: publish inbound failed", "client_id", d.id, "err", err)
	}
	return nil
}

func (d *driver) fetchBotOpenID(ctx context.Context) error {
	resp, err := d.client.Do(ctx, &larkcore.ApiReq{
		HttpMethod:                http.MethodGet,
		ApiPath:                   "/open-apis/bot/v3/info",
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant},
	})
	if err != nil {
		return fmt.Errorf("bot info request: %w", err)
	}
	var result struct {
		Code int `json:"code"`
		Bot  struct {
			OpenID string `json:"open_id"`
		} `json:"bot"`
	}
	if err := json.Unmarshal(resp.RawBody, &result); err != nil {
		return fmt.Errorf("bot info parse: %w", err)
	}
	if result.Code != 0 {
		d.invalidateTokenOnAuthError(result.Code)
		return fmt.Errorf("bot info api error (code=%d)", result.Code)
	}
	if result.Bot.OpenID == "" {
		return fmt.Errorf("bot info: empty open_id")
	}
	d.botOpenID.Store(result.Bot.OpenID)
	slog.Debug("feishu bot open_id", "client_id", d.id, "open_id", result.Bot.OpenID)
	return nil
}

func (d *driver) isBotMentioned(message *larkim.EventMessage) bool {
	if message.Mentions == nil {
		return false
	}
	knownID, _ := d.botOpenID.Load().(string)
	if knownID == "" {
		return false
	}
	for _, m := range message.Mentions {
		if m.Id == nil {
			continue
		}
		if m.Id.OpenId != nil && *m.Id.OpenId == knownID {
			return true
		}
	}
	return false
}

func extractContent(messageType, rawContent string) string {
	if rawContent == "" {
		return ""
	}
	switch messageType {
	case larkim.MsgTypeText:
		var textPayload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(rawContent), &textPayload); err == nil {
			return textPayload.Text
		}
		return rawContent
	case larkim.MsgTypePost, larkim.MsgTypeInteractive:
		return rawContent
	case larkim.MsgTypeImage:
		return ""
	case larkim.MsgTypeFile, larkim.MsgTypeAudio, larkim.MsgTypeMedia:
		name := extractFileName(rawContent)
		if name != "" {
			return name
		}
		return ""
	default:
		return rawContent
	}
}

func (d *driver) downloadInboundMedia(ctx context.Context, chatID, messageID, messageType, rawContent string) []string {
	scope := d.id + "/" + chatID + "/" + messageID
	var refs []string
	switch messageType {
	case larkim.MsgTypeImage:
		imageKey := extractImageKey(rawContent)
		if imageKey == "" {
			return nil
		}
		if ref := d.downloadResource(ctx, messageID, imageKey, "image", ".jpg", scope); ref != "" {
			refs = append(refs, ref)
		}
	case larkim.MsgTypeInteractive:
		feishuKeys, _ := extractCardImageKeys(rawContent)
		for _, imageKey := range feishuKeys {
			if ref := d.downloadResource(ctx, messageID, imageKey, "image", ".jpg", scope); ref != "" {
				refs = append(refs, ref)
			}
		}
	case larkim.MsgTypeFile, larkim.MsgTypeAudio, larkim.MsgTypeMedia:
		fileKey := extractFileKey(rawContent)
		if fileKey == "" {
			return nil
		}
		var ext string
		switch messageType {
		case larkim.MsgTypeAudio:
			ext = ".ogg"
		case larkim.MsgTypeMedia:
			ext = ".mp4"
		default:
			ext = ""
		}
		if ref := d.downloadResource(ctx, messageID, fileKey, "file", ext, scope); ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

func (d *driver) downloadResource(ctx context.Context, messageID, fileKey, resourceType, fallbackExt, scope string) string {
	req := larkim.NewGetMessageResourceReqBuilder().
		MessageId(messageID).
		FileKey(fileKey).
		Type(resourceType).
		Build()
	resp, err := d.client.Im.V1.MessageResource.Get(ctx, req)
	if err != nil {
		slog.Error("feishu: download resource", "client_id", d.id, "file_key", fileKey, "err", err)
		return ""
	}
	if !resp.Success() {
		d.invalidateTokenOnAuthError(resp.Code)
		slog.Error("feishu: resource API", "client_id", d.id, "code", resp.Code, "msg", resp.Msg)
		return ""
	}
	if resp.File == nil {
		return ""
	}
	if closer, ok := resp.File.(io.Closer); ok {
		defer closer.Close()
	}
	filename := resp.FileName
	if filename == "" {
		filename = fileKey
	}
	if filepath.Ext(filename) == "" && fallbackExt != "" {
		filename += fallbackExt
	}
	safeName := sanitizeFilename(messageID + "-" + fileKey + filepath.Ext(filename))
	loc, err := d.mediab.Put(ctx, scope, safeName, resp.File, -1, "")
	if err != nil {
		slog.Error("feishu: media put", "client_id", d.id, "err", err)
		return ""
	}
	return loc
}

func sanitizeFilename(name string) string {
	b := make([]rune, 0, len(name))
	for _, r := range name {
		switch r {
		case '/', '\\', ':', 0:
			b = append(b, '_')
		default:
			b = append(b, r)
		}
	}
	return string(b)
}

func appendMediaTags(content, messageType string, mediaRefs []string) string {
	if len(mediaRefs) == 0 {
		return content
	}
	if messageType == larkim.MsgTypeInteractive {
		return content
	}
	var tag string
	switch messageType {
	case larkim.MsgTypeImage:
		tag = "[image: photo]"
	case larkim.MsgTypeAudio:
		tag = "[audio]"
	case larkim.MsgTypeMedia:
		tag = "[video]"
	case larkim.MsgTypeFile:
		tag = "[file]"
	default:
		tag = "[attachment]"
	}
	if content == "" {
		return tag
	}
	return content + " " + tag
}

func (d *driver) sendCard(ctx context.Context, to bus.Recipient, cardContent string) error {
	chatID := to.ChatID
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeInteractive).
			Content(cardContent).
			Build()).
		Build()
	resp, err := d.client.Im.V1.Message.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("feishu send card: %w: %w", err, errTemporary)
	}
	if !resp.Success() {
		d.invalidateTokenOnAuthError(resp.Code)
		return fmt.Errorf("feishu api error (code=%d msg=%s): %w", resp.Code, resp.Msg, errTemporary)
	}
	d.lastSent.set(to, feishuCreateMessageID(resp))
	return nil
}

func (d *driver) sendText(ctx context.Context, to bus.Recipient, text string) error {
	chatID := to.ChatID
	content, _ := json.Marshal(map[string]string{"text": text})
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeText).
			Content(string(content)).
			Build()).
		Build()
	resp, err := d.client.Im.V1.Message.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("feishu send text: %w: %w", err, errTemporary)
	}
	if !resp.Success() {
		return fmt.Errorf("feishu text api error (code=%d msg=%s): %w", resp.Code, resp.Msg, errTemporary)
	}
	d.lastSent.set(to, feishuCreateMessageID(resp))
	return nil
}

func (d *driver) sendImage(ctx context.Context, to bus.Recipient, file *os.File) error {
	chatID := to.ChatID
	uploadReq := larkim.NewCreateImageReqBuilder().
		Body(larkim.NewCreateImageReqBodyBuilder().
			ImageType("message").
			Image(file).
			Build()).
		Build()
	uploadResp, err := d.client.Im.V1.Image.Create(ctx, uploadReq)
	if err != nil {
		return fmt.Errorf("feishu image upload: %w", err)
	}
	if !uploadResp.Success() {
		d.invalidateTokenOnAuthError(uploadResp.Code)
		return fmt.Errorf("feishu image upload api error (code=%d msg=%s)", uploadResp.Code, uploadResp.Msg)
	}
	if uploadResp.Data == nil || uploadResp.Data.ImageKey == nil {
		return fmt.Errorf("feishu image upload: no image_key")
	}
	imageKey := *uploadResp.Data.ImageKey
	payload, _ := json.Marshal(map[string]string{"image_key": imageKey})
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeImage).
			Content(string(payload)).
			Build()).
		Build()
	resp, err := d.client.Im.V1.Message.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("feishu image send: %w", err)
	}
	if !resp.Success() {
		d.invalidateTokenOnAuthError(resp.Code)
		return fmt.Errorf("feishu image send api error (code=%d msg=%s)", resp.Code, resp.Msg)
	}
	d.lastSent.set(to, feishuCreateMessageID(resp))
	return nil
}

func (d *driver) sendFile(ctx context.Context, to bus.Recipient, file *os.File, filename, fileType string) error {
	chatID := to.ChatID
	feishuFileType := "stream"
	switch fileType {
	case "audio":
		feishuFileType = "opus"
	case "video":
		feishuFileType = "mp4"
	}
	uploadReq := larkim.NewCreateFileReqBuilder().
		Body(larkim.NewCreateFileReqBodyBuilder().
			FileType(feishuFileType).
			FileName(filename).
			File(file).
			Build()).
		Build()
	uploadResp, err := d.client.Im.V1.File.Create(ctx, uploadReq)
	if err != nil {
		return fmt.Errorf("feishu file upload: %w", err)
	}
	if !uploadResp.Success() {
		d.invalidateTokenOnAuthError(uploadResp.Code)
		return fmt.Errorf("feishu file upload api error (code=%d msg=%s)", uploadResp.Code, uploadResp.Msg)
	}
	if uploadResp.Data == nil || uploadResp.Data.FileKey == nil {
		return fmt.Errorf("feishu file upload: no file_key")
	}
	fileKey := *uploadResp.Data.FileKey
	payload, _ := json.Marshal(map[string]string{"file_key": fileKey})
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeFile).
			Content(string(payload)).
			Build()).
		Build()
	resp, err := d.client.Im.V1.Message.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("feishu file send: %w", err)
	}
	if !resp.Success() {
		d.invalidateTokenOnAuthError(resp.Code)
		return fmt.Errorf("feishu file send api error (code=%d msg=%s)", resp.Code, resp.Msg)
	}
	d.lastSent.set(to, feishuCreateMessageID(resp))
	return nil
}

func feishuCreateMessageID(resp *larkim.CreateMessageResp) string {
	if resp == nil || resp.Data == nil || resp.Data.MessageId == nil {
		return ""
	}
	return *resp.Data.MessageId
}

func extractFeishuSenderID(sender *larkim.EventSender) string {
	if sender == nil || sender.SenderId == nil {
		return ""
	}
	if sender.SenderId.UserId != nil && *sender.SenderId.UserId != "" {
		return *sender.SenderId.UserId
	}
	if sender.SenderId.OpenId != nil && *sender.SenderId.OpenId != "" {
		return *sender.SenderId.OpenId
	}
	if sender.SenderId.UnionId != nil && *sender.SenderId.UnionId != "" {
		return *sender.SenderId.UnionId
	}
	return ""
}

func (d *driver) invalidateTokenOnAuthError(code int) {
	if code == errCodeTenantTokenInvalid {
		d.tokenCache.InvalidateAll()
		slog.Warn("feishu: invalidated cached tenant token after auth error", "client_id", d.id)
	}
}
