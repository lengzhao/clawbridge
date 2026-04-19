// Package telegram 提供 Telegram Bot（长轮询）驱动，自 PicoClaw channels/telegram 迁移。
package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/client"
	"github.com/lengzhao/clawbridge/config"
	"github.com/lengzhao/clawbridge/media"
)

var (
	reHeading    = regexp.MustCompile(`(?m)^#{1,6}\s+([^\n]+)`)
	reBlockquote = regexp.MustCompile(`^>\s*(.*)$`)
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reBoldStar   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reBoldUnder  = regexp.MustCompile(`__(.+?)__`)
	reItalic     = regexp.MustCompile(`_([^_]+)_`)
	reStrike     = regexp.MustCompile(`~~(.+?)~~`)
	reListItem   = regexp.MustCompile(`^[-*]\s+`)
	reCodeBlock  = regexp.MustCompile("```[\\w]*\\n?([\\s\\S]*?)```")
	reInlineCode = regexp.MustCompile("`([^`]+)`")
)

type driver struct {
	id            string
	bot           *telego.Bot
	bh            *th.BotHandler
	bus           *bus.MessageBus
	mediab        media.Backend
	allow         []string
	group         groupTrigger
	useMarkdownV2 bool
	chatIDs       map[string]int64
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	run           atomic.Bool
	receivedAt    func() int64
}

// New 构造 Telegram 驱动。必填 options.bot_token；可选 proxy、base_url、allow_from、group_trigger、use_markdown_v2。
func New(ctx context.Context, cfg config.ClientConfig, deps client.Deps) (client.Driver, error) {
	_ = ctx
	cred := cfg.Options
	if cred == nil {
		cred = map[string]any{}
	}
	token := credString(cred, "bot_token")
	if token == "" {
		return nil, fmt.Errorf("telegram: bot_token is required")
	}

	var opts []telego.BotOption
	if p := credString(cred, "proxy"); p != "" {
		proxyURL, err := url.Parse(p)
		if err != nil {
			return nil, fmt.Errorf("telegram: invalid proxy URL %q: %w", p, err)
		}
		opts = append(opts, telego.WithHTTPClient(&http.Client{
			Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		}))
	} else if os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" {
		opts = append(opts, telego.WithHTTPClient(&http.Client{
			Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
		}))
	}
	if baseURL := strings.TrimRight(credString(cred, "base_url"), "/"); baseURL != "" {
		opts = append(opts, telego.WithAPIServer(baseURL))
	}

	bot, err := telego.NewBot(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("telegram: create bot: %w", err)
	}

	d := &driver{
		id:            cfg.ID,
		bot:           bot,
		bus:           deps.Bus,
		mediab:        deps.Media,
		allow:         credStringSlice(cred, "allow_from"),
		group:         credGroupTrigger(cred, "group_trigger"),
		useMarkdownV2: credBool(cred, "use_markdown_v2"),
		chatIDs:       make(map[string]int64),
		receivedAt:    func() int64 { return time.Now().Unix() },
	}
	if len(d.allow) == 0 {
		slog.Warn("telegram driver: allow_from is empty; all senders are accepted (set allow_from or use '*' explicitly)",
			"client_id", cfg.ID)
	}
	return d, nil
}

func (d *driver) Name() string { return d.id }

func (d *driver) Start(ctx context.Context) error {
	if d.bus == nil {
		return fmt.Errorf("telegram: message bus is nil")
	}
	slog.Info("telegram driver starting (long polling)", "client_id", d.id)

	d.ctx, d.cancel = context.WithCancel(ctx)

	updates, err := d.bot.UpdatesViaLongPolling(d.ctx, &telego.GetUpdatesParams{Timeout: 30})
	if err != nil {
		d.cancel()
		return fmt.Errorf("telegram: long polling: %w", err)
	}

	bh, err := th.NewBotHandler(d.bot, updates)
	if err != nil {
		d.cancel()
		return fmt.Errorf("telegram: bot handler: %w", err)
	}
	d.bh = bh

	bh.HandleMessage(func(tc *th.Context, message telego.Message) error {
		return d.handleMessage(tc.Context(), &message)
	}, th.AnyMessage())

	d.run.Store(true)
	slog.Info("telegram bot connected", "client_id", d.id, "username", d.bot.Username())

	go func() {
		if err := bh.Start(); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("telegram bot handler stopped", "client_id", d.id, "err", err)
		}
	}()

	return nil
}

func (d *driver) Stop(ctx context.Context) error {
	d.run.Store(false)
	if d.bh != nil {
		_ = d.bh.StopWithContext(ctx)
	}
	if d.cancel != nil {
		d.cancel()
	}
	slog.Info("telegram driver stopped", "client_id", d.id)
	return nil
}

func (d *driver) Send(ctx context.Context, msg *bus.OutboundMessage) (string, error) {
	if !d.run.Load() {
		return "", client.ErrNotRunning
	}
	if msg == nil {
		return "", errors.New("telegram: nil message")
	}

	chatID, threadID, err := parseTelegramSessionID(msg.To.SessionID)
	if err != nil {
		return "", fmt.Errorf("telegram: invalid session_id: %w", client.ErrSendFailed)
	}

	replyToID := msg.ReplyToID
	var lastID string
	text := strings.TrimSpace(msg.Text)

	if text != "" {
		queue := []string{text}
		for len(queue) > 0 {
			chunk := queue[0]
			queue = queue[1:]

			content := parseContent(chunk, d.useMarkdownV2)

			if len([]rune(content)) > 4096 {
				runeChunk := []rune(chunk)
				ratio := float64(len(runeChunk)) / float64(max(len([]rune(content)), 1))
				smallerLen := int(float64(4096) * ratio * 0.95)
				if smallerLen >= len(runeChunk) {
					smallerLen = len(runeChunk) - 1
				}
				if smallerLen <= 0 {
					msgID, err := d.sendChunk(ctx, sendChunkParams{
						chatID: chatID, threadID: threadID, content: content,
						replyToID: replyToID, mdFallback: chunk, useMarkdownV2: d.useMarkdownV2,
					})
					if err != nil {
						return lastID, err
					}
					lastID = msgID
					replyToID = ""
					continue
				}
				subChunks := SplitMessage(chunk, smallerLen)
				if len(subChunks) == 1 && subChunks[0] == chunk {
					part1 := string(runeChunk[:smallerLen])
					part2 := string(runeChunk[smallerLen:])
					subChunks = []string{part1, part2}
				}
				var nonEmpty []string
				for _, s := range subChunks {
					if s != "" {
						nonEmpty = append(nonEmpty, s)
					}
				}
				queue = append(nonEmpty, queue...)
				continue
			}

			msgID, err := d.sendChunk(ctx, sendChunkParams{
				chatID: chatID, threadID: threadID, content: content,
				replyToID: replyToID, mdFallback: chunk, useMarkdownV2: d.useMarkdownV2,
			})
			if err != nil {
				return lastID, err
			}
			lastID = msgID
			replyToID = ""
		}
	}

	for _, part := range msg.Parts {
		id, err := d.sendOutboundMediaPart(ctx, chatID, threadID, part)
		if err != nil {
			return lastID, err
		}
		if id != "" {
			lastID = id
		}
	}

	return lastID, nil
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

type sendChunkParams struct {
	chatID        int64
	threadID      int
	content       string
	replyToID     string
	mdFallback    string
	useMarkdownV2 bool
}

func (d *driver) sendChunk(ctx context.Context, params sendChunkParams) (string, error) {
	tgMsg := tu.Message(tu.ID(params.chatID), params.content)
	tgMsg.MessageThreadID = params.threadID
	if params.useMarkdownV2 {
		tgMsg.WithParseMode(telego.ModeMarkdownV2)
	} else {
		tgMsg.WithParseMode(telego.ModeHTML)
	}

	if params.replyToID != "" {
		if mid, parseErr := strconv.Atoi(params.replyToID); parseErr == nil {
			tgMsg.ReplyParameters = &telego.ReplyParameters{MessageID: mid}
		}
	}

	pMsg, err := d.bot.SendMessage(ctx, tgMsg)
	if err != nil {
		logParseFailed(err, params.useMarkdownV2)
		tgMsg.Text = params.mdFallback
		tgMsg.ParseMode = ""
		pMsg, err = d.bot.SendMessage(ctx, tgMsg)
		if err != nil {
			return "", fmt.Errorf("telegram send: %w: %w", err, client.ErrTemporary)
		}
	}

	return strconv.Itoa(pMsg.MessageID), nil
}

func (d *driver) EditMessage(ctx context.Context, msg *bus.OutboundMessage) error {
	if msg == nil {
		return errors.New("telegram: nil OutboundMessage")
	}
	sessionID := strings.TrimSpace(msg.To.SessionID)
	if sessionID == "" {
		return nil
	}
	midStr := strings.TrimSpace(msg.MessageID)
	if midStr == "" {
		return client.ErrCapabilityUnsupported
	}
	cid, _, err := parseTelegramSessionID(sessionID)
	if err != nil {
		return err
	}
	mid, err := strconv.Atoi(midStr)
	if err != nil {
		return err
	}
	content := strings.TrimSpace(msg.Text)
	parsedContent := parseContent(content, d.useMarkdownV2)
	editMsg := tu.EditMessageText(tu.ID(cid), mid, parsedContent)
	if d.useMarkdownV2 {
		editMsg.WithParseMode(telego.ModeMarkdownV2)
	} else {
		editMsg.WithParseMode(telego.ModeHTML)
	}
	_, err = d.bot.EditMessageText(ctx, editMsg)
	if err != nil {
		if strings.Contains(err.Error(), "message is not modified") {
			return nil
		}
		if strings.Contains(err.Error(), "Bad Request") {
			logParseFailed(err, d.useMarkdownV2)
			_, err = d.bot.EditMessageText(ctx, tu.EditMessageText(tu.ID(cid), mid, content))
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "message is not modified") {
			return nil
		}
		if isPostConnectError(err) {
			slog.Warn("telegram EditMessage: swallowing post-connect error", "session_id", sessionID, "mid", mid, "err", err)
			return nil
		}
	}
	return err
}

func (d *driver) publishInbound(ctx context.Context, sessionID, messageID, content string, mediaPaths []string, sender bus.SenderInfo, peer bus.Peer, metadata map[string]string) {
	in := &bus.InboundMessage{
		ClientID:   d.id,
		SessionID:  sessionID,
		MessageID:  messageID,
		Sender:     sender,
		Peer:       peer,
		Content:    content,
		MediaPaths: mediaPaths,
		ReceivedAt: d.receivedAt(),
		Metadata:   metadata,
	}
	if err := d.bus.PublishInbound(ctx, in); err != nil {
		slog.Error("telegram: publish inbound", "client_id", d.id, "err", err)
	}
}

func (d *driver) handleMessage(ctx context.Context, message *telego.Message) error {
	if message == nil {
		return fmt.Errorf("message is nil")
	}
	user := message.From
	if user == nil {
		return fmt.Errorf("message sender (user) is nil")
	}

	platformID := fmt.Sprintf("%d", user.ID)
	sender := bus.SenderInfo{
		Platform:    "telegram",
		PlatformID:  platformID,
		CanonicalID: buildCanonicalID("telegram", platformID),
		Username:    user.Username,
		DisplayName: user.FirstName,
	}

	if !isAllowedSender(sender, d.allow) {
		slog.Debug("telegram: message rejected by allowlist", "user_id", platformID)
		return nil
	}

	chatID := message.Chat.ID
	d.chatIDs[platformID] = chatID

	content := ""
	var mediaPaths []string
	chatIDStr := fmt.Sprintf("%d", chatID)
	messageIDStr := fmt.Sprintf("%d", message.MessageID)
	scope := fmt.Sprintf("%s/%s/%s", d.id, chatIDStr, messageIDStr)

	storeMedia := func(localPath, filename string) string {
		if d.mediab == nil || localPath == "" {
			return localPath
		}
		f, err := os.Open(localPath)
		if err != nil {
			return localPath
		}
		st, err := f.Stat()
		if err != nil {
			_ = f.Close()
			return localPath
		}
		ct := mime.TypeByExtension(filepath.Ext(filename))
		if ct == "" {
			ct = "application/octet-stream"
		}
		loc, err := d.mediab.Put(ctx, scope, filepath.Base(filename), f, st.Size(), ct)
		_ = f.Close()
		if err != nil {
			slog.Error("telegram: media put", "path", localPath, "err", err)
			return localPath
		}
		_ = os.Remove(localPath)
		return loc
	}

	if message.Text != "" {
		content += message.Text
	}
	if message.Caption != "" {
		if content != "" {
			content += "\n"
		}
		content += message.Caption
	}

	if len(message.Photo) > 0 {
		photo := message.Photo[len(message.Photo)-1]
		photoPath := d.downloadPhoto(ctx, photo.FileID)
		if photoPath != "" {
			mediaPaths = append(mediaPaths, storeMedia(photoPath, "photo.jpg"))
			if content != "" {
				content += "\n"
			}
			content += "[image: photo]"
		}
	}
	if message.Voice != nil {
		voicePath := d.downloadFile(ctx, message.Voice.FileID, ".ogg")
		if voicePath != "" {
			mediaPaths = append(mediaPaths, storeMedia(voicePath, "voice.ogg"))
			if content != "" {
				content += "\n"
			}
			content += "[voice]"
		}
	}
	if message.Audio != nil {
		audioPath := d.downloadFile(ctx, message.Audio.FileID, ".mp3")
		if audioPath != "" {
			mediaPaths = append(mediaPaths, storeMedia(audioPath, "audio.mp3"))
			if content != "" {
				content += "\n"
			}
			content += "[audio]"
		}
	}
	if message.Document != nil {
		docPath := d.downloadFile(ctx, message.Document.FileID, "")
		if docPath != "" {
			mediaPaths = append(mediaPaths, storeMedia(docPath, "document"))
			if content != "" {
				content += "\n"
			}
			content += "[file]"
		}
	}

	if content == "" && len(mediaPaths) == 0 {
		return nil
	}
	if content == "" {
		content = "[media only]"
	}

	if message.Chat.Type != "private" {
		isMentioned := d.isBotMentioned(message)
		if isMentioned {
			content = d.stripBotMention(content)
		}
		respond, cleaned := shouldRespondInGroup(d.group, isMentioned, content)
		if !respond {
			return nil
		}
		content = cleaned
	}

	if message.ReplyToMessage != nil {
		quotedMedia := quotedTelegramMediaRefs(message.ReplyToMessage, func(fileID, ext, filename string) string {
			localPath := d.downloadFile(ctx, fileID, ext)
			if localPath == "" {
				return ""
			}
			return storeMedia(localPath, filename)
		})
		if len(quotedMedia) > 0 {
			mediaPaths = append(quotedMedia, mediaPaths...)
		}
		content = d.prependTelegramQuotedReply(content, message.ReplyToMessage)
	}

	compositeChatID := fmt.Sprintf("%d", chatID)
	threadID := message.MessageThreadID
	if message.Chat.IsForum && threadID != 0 {
		compositeChatID = fmt.Sprintf("%d/%d", chatID, threadID)
	}

	slog.Debug("telegram inbound", "client_id", d.id, "sender", sender.CanonicalID, "session_id", compositeChatID,
		"preview", truncateRunes(content, 50))

	peerKind := "direct"
	peerID := fmt.Sprintf("%d", user.ID)
	if message.Chat.Type != "private" {
		peerKind = "group"
		peerID = compositeChatID
	}

	metadata := map[string]string{
		"user_id":    fmt.Sprintf("%d", user.ID),
		"username":   user.Username,
		"first_name": user.FirstName,
		"is_group":   fmt.Sprintf("%t", message.Chat.Type != "private"),
	}
	if message.ReplyToMessage != nil {
		metadata["reply_to_message_id"] = fmt.Sprintf("%d", message.ReplyToMessage.MessageID)
	}
	if message.Chat.IsForum && threadID != 0 {
		metadata["parent_peer_kind"] = "topic"
		metadata["parent_peer_id"] = fmt.Sprintf("%d", threadID)
	}

	d.publishInbound(ctx, compositeChatID, messageIDStr, content, mediaPaths, sender,
		bus.Peer{Kind: peerKind, ID: peerID}, metadata)
	return nil
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func (d *driver) prependTelegramQuotedReply(content string, reply *telego.Message) string {
	quoted := strings.TrimSpace(telegramQuotedContent(reply))
	if quoted == "" {
		return content
	}
	author := telegramQuotedAuthor(reply)
	role := d.telegramQuotedRole(reply)
	if strings.TrimSpace(content) == "" {
		return fmt.Sprintf("[quoted %s message from %s]: %s", role, author, quoted)
	}
	return fmt.Sprintf("[quoted %s message from %s]: %s\n\n%s", role, author, quoted, content)
}

func (d *driver) telegramQuotedRole(message *telego.Message) string {
	if message == nil {
		return "unknown"
	}
	if message.From != nil {
		if !message.From.IsBot {
			return "user"
		}
		if d.isOwnBotUser(message.From) {
			return "assistant"
		}
		return "bot"
	}
	if message.SenderChat != nil {
		return "chat"
	}
	return "unknown"
}

func (d *driver) isOwnBotUser(user *telego.User) bool {
	if d == nil || d.bot == nil || user == nil || !user.IsBot {
		return false
	}
	if botID := d.bot.ID(); botID != 0 && user.ID == botID {
		return true
	}
	botUsername := strings.TrimPrefix(strings.TrimSpace(d.bot.Username()), "@")
	if botUsername == "" {
		return false
	}
	return strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(user.Username), "@"), botUsername)
}

func telegramQuotedAuthor(message *telego.Message) string {
	if message == nil || message.From == nil {
		return "unknown"
	}
	if username := strings.TrimSpace(message.From.Username); username != "" {
		return username
	}
	if firstName := strings.TrimSpace(message.From.FirstName); firstName != "" {
		return firstName
	}
	return "unknown"
}

func telegramQuotedContent(message *telego.Message) string {
	if message == nil {
		return ""
	}
	var parts []string
	if text := strings.TrimSpace(message.Text); text != "" {
		parts = append(parts, text)
	}
	if caption := strings.TrimSpace(message.Caption); caption != "" {
		parts = append(parts, caption)
	}
	if len(message.Photo) > 0 {
		parts = append(parts, "[image: photo]")
	}
	switch {
	case message.Voice != nil:
		parts = append(parts, "[voice]")
	case message.Audio != nil:
		parts = append(parts, "[audio]")
	}
	if message.Document != nil {
		parts = append(parts, "[file]")
	}
	return strings.Join(parts, "\n")
}

func quotedTelegramMediaRefs(message *telego.Message, resolve func(fileID, ext, filename string) string) []string {
	if message == nil || resolve == nil {
		return nil
	}
	var refs []string
	if message.Voice != nil {
		if ref := resolve(message.Voice.FileID, ".ogg", "voice.ogg"); ref != "" {
			refs = append(refs, ref)
		}
	}
	if message.Audio != nil {
		if ref := resolve(message.Audio.FileID, ".mp3", "audio.mp3"); ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

func (d *driver) downloadPhoto(ctx context.Context, fileID string) string {
	file, err := d.bot.GetFile(ctx, &telego.GetFileParams{FileID: fileID})
	if err != nil {
		slog.Error("telegram: get photo file", "err", err)
		return ""
	}
	return d.downloadFileWithInfo(ctx, file, ".jpg")
}

func (d *driver) downloadFile(ctx context.Context, fileID, ext string) string {
	file, err := d.bot.GetFile(ctx, &telego.GetFileParams{FileID: fileID})
	if err != nil {
		slog.Error("telegram: get file", "err", err)
		return ""
	}
	return d.downloadFileWithInfo(ctx, file, ext)
}

func (d *driver) downloadFileWithInfo(ctx context.Context, file *telego.File, ext string) string {
	if file.FilePath == "" {
		return ""
	}
	u := d.bot.FileDownloadURL(file.FilePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("telegram: download file", "err", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	pattern := "tg-in-*"
	if ext != "" {
		pattern = "tg-in-*" + ext
	}
	tmp, err := os.CreateTemp("", pattern)
	if err != nil {
		return ""
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return ""
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return ""
	}
	return tmpPath
}

func inferOutboundMediaKind(part bus.MediaPart) string {
	fn := strings.ToLower(part.Filename)
	ct := strings.ToLower(part.ContentType)
	if strings.HasPrefix(ct, "image/") || strings.HasSuffix(fn, ".jpg") || strings.HasSuffix(fn, ".jpeg") ||
		strings.HasSuffix(fn, ".png") || strings.HasSuffix(fn, ".gif") || strings.HasSuffix(fn, ".webp") {
		return "image"
	}
	if strings.HasPrefix(ct, "video/") || strings.HasSuffix(fn, ".mp4") || strings.HasSuffix(fn, ".mov") {
		return "video"
	}
	if strings.HasPrefix(ct, "audio/") {
		if strings.Contains(fn, "voice") && (strings.HasSuffix(fn, ".ogg") || strings.HasSuffix(fn, ".oga")) {
			return "voice"
		}
		return "audio"
	}
	return "file"
}

func (d *driver) sendOutboundMediaPart(ctx context.Context, chatID int64, threadID int, part bus.MediaPart) (string, error) {
	if d.mediab == nil {
		return "", fmt.Errorf("telegram: media backend is nil: %w", client.ErrSendFailed)
	}
	rc, err := d.mediab.Open(ctx, part.Path)
	if err != nil {
		return "", fmt.Errorf("telegram: open media: %w", err)
	}
	defer rc.Close()

	tmp, err := os.CreateTemp("", "tg-out-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	defer func() { _ = os.Remove(tmpPath) }()

	file, err := os.Open(tmpPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	kind := inferOutboundMediaKind(part)
	var tgResult *telego.Message
	switch kind {
	case "image":
		params := &telego.SendPhotoParams{
			ChatID: tu.ID(chatID), MessageThreadID: threadID,
			Photo: telego.InputFile{File: file}, Caption: part.Caption,
		}
		tgResult, err = d.bot.SendPhoto(ctx, params)
		if err != nil && strings.Contains(err.Error(), "PHOTO_INVALID_DIMENSIONS") {
			if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
				return "", fmt.Errorf("telegram: rewind photo: %w", client.ErrTemporary)
			}
			tgResult, err = d.bot.SendDocument(ctx, &telego.SendDocumentParams{
				ChatID: tu.ID(chatID), MessageThreadID: threadID,
				Document: telego.InputFile{File: file}, Caption: part.Caption,
			})
		}
	case "voice":
		tgResult, err = d.bot.SendVoice(ctx, &telego.SendVoiceParams{
			ChatID: tu.ID(chatID), MessageThreadID: threadID,
			Voice: telego.InputFile{File: file}, Caption: part.Caption,
		})
	case "audio":
		tgResult, err = d.bot.SendAudio(ctx, &telego.SendAudioParams{
			ChatID: tu.ID(chatID), MessageThreadID: threadID,
			Audio: telego.InputFile{File: file}, Caption: part.Caption,
		})
	case "video":
		tgResult, err = d.bot.SendVideo(ctx, &telego.SendVideoParams{
			ChatID: tu.ID(chatID), MessageThreadID: threadID,
			Video: telego.InputFile{File: file}, Caption: part.Caption,
		})
	default:
		tgResult, err = d.bot.SendDocument(ctx, &telego.SendDocumentParams{
			ChatID: tu.ID(chatID), MessageThreadID: threadID,
			Document: telego.InputFile{File: file}, Caption: part.Caption,
		})
	}
	if err != nil {
		return "", fmt.Errorf("telegram send media: %w: %w", err, client.ErrTemporary)
	}
	if tgResult == nil {
		return "", nil
	}
	return strconv.Itoa(tgResult.MessageID), nil
}

func parseContent(text string, useMarkdownV2 bool) string {
	if useMarkdownV2 {
		return markdownToTelegramMarkdownV2(text)
	}
	return markdownToTelegramHTML(text)
}

func parseTelegramSessionID(sessionID string) (int64, int, error) {
	idx := strings.Index(sessionID, "/")
	if idx == -1 {
		cid, err := strconv.ParseInt(sessionID, 10, 64)
		return cid, 0, err
	}
	cid, err := strconv.ParseInt(sessionID[:idx], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	tid, err := strconv.Atoi(sessionID[idx+1:])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid thread ID in session_id %q: %w", sessionID, err)
	}
	return cid, tid, nil
}

func logParseFailed(err error, useMarkdownV2 bool) {
	name := "HTML"
	if useMarkdownV2 {
		name = "MarkdownV2"
	}
	slog.Error("telegram parse mode failed, falling back to plain text", "mode", name, "err", err)
}

func (d *driver) isBotMentioned(message *telego.Message) bool {
	text, entities := telegramEntityTextAndList(message)
	if text == "" || len(entities) == 0 {
		return false
	}
	botUsername := ""
	if d.bot != nil {
		botUsername = d.bot.Username()
	}
	runes := []rune(text)

	for _, entity := range entities {
		entityText, ok := telegramEntityText(runes, entity)
		if !ok {
			continue
		}
		switch entity.Type {
		case telego.EntityTypeMention:
			if botUsername != "" && strings.EqualFold(entityText, "@"+botUsername) {
				return true
			}
		case telego.EntityTypeTextMention:
			if botUsername != "" && entity.User != nil && strings.EqualFold(entity.User.Username, botUsername) {
				return true
			}
		case telego.EntityTypeBotCommand:
			if isBotCommandEntityForThisBot(entityText, botUsername) {
				return true
			}
		}
	}
	return false
}

func telegramEntityTextAndList(message *telego.Message) (string, []telego.MessageEntity) {
	if message.Text != "" {
		return message.Text, message.Entities
	}
	return message.Caption, message.CaptionEntities
}

func telegramEntityText(runes []rune, entity telego.MessageEntity) (string, bool) {
	if entity.Offset < 0 || entity.Length <= 0 {
		return "", false
	}
	end := entity.Offset + entity.Length
	if entity.Offset >= len(runes) || end > len(runes) {
		return "", false
	}
	return string(runes[entity.Offset:end]), true
}

func isBotCommandEntityForThisBot(entityText, botUsername string) bool {
	if !strings.HasPrefix(entityText, "/") {
		return false
	}
	command := strings.TrimPrefix(entityText, "/")
	if command == "" {
		return false
	}
	at := strings.IndexRune(command, '@')
	if at == -1 {
		return true
	}
	mentionUsername := command[at+1:]
	if mentionUsername == "" || botUsername == "" {
		return false
	}
	return strings.EqualFold(mentionUsername, botUsername)
}

func (d *driver) stripBotMention(content string) string {
	botUsername := d.bot.Username()
	if botUsername == "" {
		return content
	}
	re := regexp.MustCompile(`(?i)@` + regexp.QuoteMeta(botUsername))
	content = re.ReplaceAllString(content, "")
	return strings.TrimSpace(content)
}

func isPostConnectError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "connection closed by foreign host") ||
		strings.Contains(msg, "broken pipe")
}

var (
	_ client.MessageEditor = (*driver)(nil)
	_ client.Replier       = (*driver)(nil)
)
