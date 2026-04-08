package slack

import (
	"context"
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

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/client"
	"github.com/lengzhao/clawbridge/config"
	"github.com/lengzhao/clawbridge/media"
)

var (
	errNotRunning = errors.New("slack: driver not running")
	errTemporary  = errors.New("slack: temporary failure")
)

type slackMessageRef struct {
	ChannelID string
	Timestamp string
}

type driver struct {
	id           string
	botToken     string
	bus          *bus.MessageBus
	mediab       media.Backend
	api          *slack.Client
	socketClient *socketmode.Client
	allow        []string
	group        groupTrigger

	botUserID string
	teamID    string

	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	pendingAcks   sync.Map
	run           atomic.Bool
	receivedAtNow func() int64
}

// New builds a Slack driver (Socket Mode). Credentials: bot_token, app_token (required);
// optional allow_from, group_trigger (same shape as feishu driver).
func New(ctx context.Context, cfg config.ClientConfig, deps client.Deps) (client.Driver, error) {
	_ = ctx
	cred := cfg.Credentials
	if cred == nil {
		cred = map[string]any{}
	}
	botTok := credString(cred, "bot_token")
	appTok := credString(cred, "app_token")
	if botTok == "" || appTok == "" {
		return nil, fmt.Errorf("slack: bot_token and app_token are required")
	}

	api := slack.New(botTok, slack.OptionAppLevelToken(appTok))
	socketClient := socketmode.New(api)

	d := &driver{
		id:            cfg.ID,
		botToken:      botTok,
		bus:           deps.Bus,
		mediab:        deps.Media,
		api:           api,
		socketClient:  socketClient,
		allow:         credStringSlice(cred, "allow_from"),
		group:         credGroupTrigger(cred, "group_trigger"),
		receivedAtNow: func() int64 { return time.Now().Unix() },
	}
	if len(d.allow) == 0 {
		slog.Warn("slack driver: allow_from is empty; all senders are accepted (set allow_from or use '*' explicitly)",
			"client_id", cfg.ID)
	}
	return d, nil
}

func (d *driver) Name() string { return d.id }

func (d *driver) Start(ctx context.Context) error {
	if d.bus == nil {
		return fmt.Errorf("slack: message bus is nil")
	}

	slog.Info("slack driver starting (socket mode)", "client_id", d.id)

	runCtx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.ctx = runCtx
	d.cancel = cancel
	d.mu.Unlock()

	authResp, err := d.api.AuthTestContext(runCtx)
	if err != nil {
		cancel()
		return fmt.Errorf("slack auth test: %w", err)
	}
	d.botUserID = authResp.UserID
	d.teamID = authResp.TeamID

	slog.Info("slack bot connected", "client_id", d.id, "bot_user_id", d.botUserID, "team", authResp.Team)

	go d.eventLoop()
	go func() {
		if err := d.socketClient.RunContext(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			if runCtx.Err() == nil {
				slog.Error("slack socket mode error", "client_id", d.id, "err", err)
			}
		}
	}()

	d.run.Store(true)
	slog.Info("slack driver started", "client_id", d.id)
	return nil
}

func (d *driver) Stop(ctx context.Context) error {
	_ = ctx
	d.mu.Lock()
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	d.ctx = nil
	d.mu.Unlock()
	d.run.Store(false)
	slog.Info("slack driver stopped", "client_id", d.id)
	return nil
}

func (d *driver) Send(ctx context.Context, msg *bus.OutboundMessage) (string, error) {
	if !d.run.Load() {
		return "", errNotRunning
	}
	channelID, threadTS := parseSlackChatID(msg.To.ChatID)
	if msg.ThreadID != "" {
		threadTS = msg.ThreadID
	}
	if channelID == "" {
		return "", fmt.Errorf("slack: empty channel in chat_id: %w", errTemporary)
	}

	var lastTS string

	if strings.TrimSpace(msg.Text) != "" {
		opts := []slack.MsgOption{slack.MsgOptionText(msg.Text, false)}
		if msg.ReplyToID != "" && threadTS == "" {
			opts = append(opts, slack.MsgOptionTS(msg.ReplyToID))
		} else if threadTS != "" {
			opts = append(opts, slack.MsgOptionTS(threadTS))
		}

		_, ts, err := d.api.PostMessageContext(ctx, channelID, opts...)
		if err != nil {
			return "", fmt.Errorf("slack post message: %w: %w", err, errTemporary)
		}
		lastTS = ts

		if ref, ok := d.pendingAcks.LoadAndDelete(msg.To.ChatID); ok {
			msgRef := ref.(slackMessageRef)
			_ = d.api.AddReaction("white_check_mark", slack.ItemRef{
				Channel:   msgRef.ChannelID,
				Timestamp: msgRef.Timestamp,
			})
		}
	}

	for _, part := range msg.Parts {
		if err := d.uploadMediaPart(ctx, channelID, threadTS, part); err != nil {
			return lastTS, err
		}
	}

	slog.Debug("slack message sent", "client_id", d.id, "channel_id", channelID, "thread_ts", threadTS)
	return lastTS, nil
}

func (d *driver) uploadMediaPart(ctx context.Context, channelID, threadTS string, part bus.MediaPart) error {
	if d.mediab == nil {
		return fmt.Errorf("slack: media backend is nil: %w", client.ErrSendFailed)
	}
	rc, err := d.mediab.Open(ctx, part.Path)
	if err != nil {
		slog.Error("slack: open media locator", "client_id", d.id, "path", part.Path, "err", err)
		return fmt.Errorf("slack: open media: %w", err)
	}
	defer rc.Close()

	tmp, err := os.CreateTemp("", "slack-out-*")
	if err != nil {
		return fmt.Errorf("slack: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("slack: copy media: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	st, err := os.Stat(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer func() { _ = os.Remove(tmpPath) }()

	filename := part.Filename
	if filename == "" {
		filename = filepath.Base(part.Path)
	}
	if filename == "" || filename == "." {
		filename = "file"
	}
	title := part.Caption
	if title == "" {
		title = filename
	}

	params := slack.UploadFileV2Parameters{
		File:            tmpPath,
		FileSize:        int(st.Size()),
		Filename:        filename,
		Title:           title,
		Channel:         channelID,
		ThreadTimestamp: threadTS,
	}
	if _, err := d.api.UploadFileV2Context(ctx, params); err != nil {
		return fmt.Errorf("slack upload file: %w: %w", err, errTemporary)
	}
	return nil
}

func (d *driver) eventLoop() {
	d.mu.Lock()
	ctx := d.ctx
	d.mu.Unlock()
	if ctx == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-d.socketClient.Events:
			if !ok {
				return
			}
			switch event.Type {
			case socketmode.EventTypeEventsAPI:
				d.handleEventsAPI(ctx, event)
			case socketmode.EventTypeSlashCommand:
				d.handleSlashCommand(ctx, event)
			case socketmode.EventTypeInteractive:
				if event.Request != nil {
					d.socketClient.Ack(*event.Request)
				}
			}
		}
	}
}

func (d *driver) handleEventsAPI(ctx context.Context, event socketmode.Event) {
	if event.Request != nil {
		d.socketClient.Ack(*event.Request)
	}

	eventsAPIEvent, ok := event.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}

	switch ev := eventsAPIEvent.InnerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		d.handleMessageEvent(ctx, ev)
	case *slackevents.AppMentionEvent:
		d.handleAppMention(ctx, ev)
	}
}

func (d *driver) handleMessageEvent(ctx context.Context, ev *slackevents.MessageEvent) {
	if ev.User == d.botUserID || ev.User == "" {
		return
	}
	if ev.BotID != "" {
		return
	}
	if ev.SubType != "" && ev.SubType != "file_share" {
		return
	}

	sender := bus.SenderInfo{
		Platform:    "slack",
		PlatformID:  ev.User,
		CanonicalID: buildCanonicalID("slack", ev.User),
	}
	if !isAllowedSender(sender, d.allow) {
		slog.Debug("slack: message rejected by allowlist", "client_id", d.id, "user_id", ev.User)
		return
	}

	senderID := ev.User
	channelID := ev.Channel
	threadTS := ev.ThreadTimeStamp
	messageTS := ev.TimeStamp

	chatID := channelID
	if threadTS != "" {
		chatID = channelID + "/" + threadTS
	}

	d.pendingAcks.Store(chatID, slackMessageRef{ChannelID: channelID, Timestamp: messageTS})

	content := ev.Text
	content = d.stripBotMention(content)

	if !strings.HasPrefix(channelID, "D") {
		respond, cleaned := shouldRespondInGroup(d.group, false, content)
		if !respond {
			return
		}
		content = cleaned
	}

	var mediaPaths []string
	scope := fmt.Sprintf("%s/%s/%s", d.id, chatID, messageTS)

	if ev.Message != nil && len(ev.Message.Files) > 0 && d.mediab != nil {
		for _, file := range ev.Message.Files {
			loc := d.downloadSlackFileToMedia(ctx, scope, file)
			if loc == "" {
				continue
			}
			mediaPaths = append(mediaPaths, loc)
			content += fmt.Sprintf("\n[file: %s]", file.Name)
		}
	}

	if strings.TrimSpace(content) == "" {
		return
	}

	peerKind := "channel"
	peerID := channelID
	if strings.HasPrefix(channelID, "D") {
		peerKind = "direct"
		peerID = senderID
	}

	d.publishInbound(ctx, chatID, messageTS, senderID, content, mediaPaths, sender, bus.Peer{Kind: peerKind, ID: peerID}, map[string]string{
		"message_ts": messageTS,
		"channel_id": channelID,
		"thread_ts":  threadTS,
		"platform":   "slack",
		"team_id":    d.teamID,
	})
}

func (d *driver) handleAppMention(ctx context.Context, ev *slackevents.AppMentionEvent) {
	if ev.User == d.botUserID {
		return
	}

	senderCheck := bus.SenderInfo{
		Platform:    "slack",
		PlatformID:  ev.User,
		CanonicalID: buildCanonicalID("slack", ev.User),
	}
	if !isAllowedSender(senderCheck, d.allow) {
		slog.Debug("slack: mention rejected by allowlist", "client_id", d.id, "user_id", ev.User)
		return
	}

	senderID := ev.User
	sender := bus.SenderInfo{
		Platform:    "slack",
		PlatformID:  senderID,
		CanonicalID: buildCanonicalID("slack", senderID),
	}
	channelID := ev.Channel
	threadTS := ev.ThreadTimeStamp
	messageTS := ev.TimeStamp

	var chatID string
	if threadTS != "" {
		chatID = channelID + "/" + threadTS
	} else {
		chatID = channelID + "/" + messageTS
	}

	d.pendingAcks.Store(chatID, slackMessageRef{ChannelID: channelID, Timestamp: messageTS})

	content := d.stripBotMention(ev.Text)
	if strings.TrimSpace(content) == "" {
		return
	}

	mentionPeerKind := "channel"
	mentionPeerID := channelID
	if strings.HasPrefix(channelID, "D") {
		mentionPeerKind = "direct"
		mentionPeerID = senderID
	}

	d.publishInbound(ctx, chatID, messageTS, senderID, content, nil, sender, bus.Peer{Kind: mentionPeerKind, ID: mentionPeerID}, map[string]string{
		"message_ts": messageTS,
		"channel_id": channelID,
		"thread_ts":  threadTS,
		"platform":   "slack",
		"is_mention": "true",
		"team_id":    d.teamID,
	})
}

func (d *driver) handleSlashCommand(ctx context.Context, event socketmode.Event) {
	cmd, ok := event.Data.(slack.SlashCommand)
	if !ok {
		return
	}

	if event.Request != nil {
		d.socketClient.Ack(*event.Request)
	}

	cmdSender := bus.SenderInfo{
		Platform:    "slack",
		PlatformID:  cmd.UserID,
		CanonicalID: buildCanonicalID("slack", cmd.UserID),
	}
	if !isAllowedSender(cmdSender, d.allow) {
		slog.Debug("slack: slash command rejected by allowlist", "client_id", d.id, "user_id", cmd.UserID)
		return
	}

	senderID := cmd.UserID
	channelID := cmd.ChannelID
	chatID := channelID
	content := cmd.Text
	if strings.TrimSpace(content) == "" {
		content = "help"
	}

	d.publishInbound(ctx, chatID, "", senderID, content, nil, cmdSender, bus.Peer{Kind: "channel", ID: channelID}, map[string]string{
		"channel_id": channelID,
		"platform":   "slack",
		"is_command": "true",
		"trigger_id": cmd.TriggerID,
		"team_id":    d.teamID,
	})
}

func (d *driver) publishInbound(ctx context.Context, chatID, messageTS, senderID, content string, mediaPaths []string, sender bus.SenderInfo, peer bus.Peer, metadata map[string]string) {
	in := &bus.InboundMessage{
		Channel:    d.id,
		ChatID:     chatID,
		MessageID:  messageTS,
		Sender:     sender,
		Peer:       peer,
		Content:    content,
		MediaPaths: mediaPaths,
		ReceivedAt: d.receivedAtNow(),
		Metadata:   metadata,
	}
	if err := d.bus.PublishInbound(ctx, in); err != nil {
		slog.Error("slack: publish inbound", "client_id", d.id, "err", err)
	}
}

func (d *driver) downloadSlackFileToMedia(ctx context.Context, scope string, file slack.File) string {
	if d.mediab == nil {
		return ""
	}
	downloadURL := file.URLPrivateDownload
	if downloadURL == "" {
		downloadURL = file.URLPrivate
	}
	if downloadURL == "" {
		slog.Error("slack: no download URL for file", "client_id", d.id, "file_id", file.ID)
		return ""
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		slog.Error("slack: download request", "client_id", d.id, "err", err)
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+d.botToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("slack: download file", "client_id", d.id, "err", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Error("slack: download file status", "client_id", d.id, "status", resp.StatusCode)
		return ""
	}

	name := file.Name
	if name == "" {
		name = "file"
	}
	name = filepath.Base(strings.ReplaceAll(strings.ReplaceAll(name, "..", ""), "/", "_"))

	ct := resp.Header.Get("Content-Type")
	size := resp.ContentLength

	loc, err := d.mediab.Put(ctx, scope, name, resp.Body, size, ct)
	if err != nil {
		slog.Error("slack: media put", "client_id", d.id, "err", err)
		return ""
	}
	return loc
}

func (d *driver) stripBotMention(text string) string {
	mention := fmt.Sprintf("<@%s>", d.botUserID)
	text = strings.ReplaceAll(text, mention, "")
	return strings.TrimSpace(text)
}

func parseSlackChatID(chatID string) (channelID, threadTS string) {
	parts := strings.SplitN(chatID, "/", 2)
	channelID = parts[0]
	if len(parts) > 1 {
		threadTS = parts[1]
	}
	return channelID, threadTS
}
