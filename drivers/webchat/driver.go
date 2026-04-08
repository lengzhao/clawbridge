// Package webchat 提供本地 HTTP 上的类 ChatGPT 网页 UI，经总线与宿主交互：
// 浏览器发送 → PublishInbound；宿主 Reply / PublishOutbound → Send 推送到页面（SSE）。
// 会话与消息历史由浏览器 localStorage 保存，服务端不持久化聊天内容。
package webchat

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/client"
	"github.com/lengzhao/clawbridge/config"
)

//go:embed static/index.html
var staticUI embed.FS

const (
	maxSSEClient  = 64
	maxChatIDLen  = 256
)

type chatEvent struct {
	Type      string `json:"type"` // message | edit | status
	ChatID    string `json:"chat_id,omitempty"`
	ID        string `json:"id,omitempty"`
	Role      string `json:"role,omitempty"` // user | assistant
	Text      string `json:"text,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	State     string `json:"state,omitempty"`
}

type driver struct {
	id         string
	bus        *bus.MessageBus
	senderName string

	basePath string
	listen   string

	mu sync.RWMutex
	// sseSub: each connection subscribes to one chat_id
	sseSubs map[chan []byte]string // ch -> chat_id

	msgSeq     atomic.Uint64
	inboundSeq atomic.Uint64

	srv    *http.Server
	srvMu  sync.Mutex
	closed atomic.Bool

	lastSentMu sync.RWMutex
	lastSent   map[string]string // bus.RecipientKey -> outbound message id
}

// New 构造 webchat driver。
//
// 必填：options.listen（建议 127.0.0.1:端口）；可选 options.path 为挂载前缀（默认 /）。
// 可选：display_name（默认 You）。
func New(ctx context.Context, cfg config.ClientConfig, deps client.Deps) (client.Driver, error) {
	_ = ctx
	opts := cfg.Options
	if opts == nil {
		opts = map[string]any{}
	}
	listen, path, err := listenPathFromOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("webchat: client %q: %w", cfg.ID, err)
	}
	base := normalizeHTTPPathPrefix(path)
	d := &driver{
		id:         cfg.ID,
		bus:        deps.Bus,
		senderName: credString(opts, "display_name", "You"),
		basePath:   base,
		listen:     listen,
		sseSubs:    make(map[chan []byte]string),
		lastSent:   make(map[string]string),
	}
	return d, nil
}

// listenPathFromOptions 读取 options.listen、options.path。
func listenPathFromOptions(opts map[string]any) (listen, path string, err error) {
	listen = strings.TrimSpace(credString(opts, "listen", ""))
	if listen == "" {
		return "", "", fmt.Errorf("options.listen is required (e.g. 127.0.0.1:8765)")
	}
	path = strings.TrimSpace(credString(opts, "path", ""))
	return listen, path, nil
}

func normalizeHTTPPathPrefix(path string) string {
	base := strings.TrimSpace(path)
	if base == "" {
		return "/"
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	base = strings.TrimSuffix(base, "/")
	if base == "" {
		return "/"
	}
	return base
}

func credString(cred map[string]any, key, def string) string {
	v, ok := cred[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return def
		}
		return s
	default:
		return fmt.Sprint(t)
	}
}

func validChatID(id string) bool {
	if id == "" || len(id) > maxChatIDLen {
		return false
	}
	if strings.ContainsAny(id, "\r\n\x00") {
		return false
	}
	return true
}

func (d *driver) Name() string { return d.id }

func (d *driver) Start(ctx context.Context) error {
	if d.closed.Load() {
		return errors.New("webchat: already stopped")
	}
	mux := http.NewServeMux()
	d.registerRoutes(mux)

	d.srvMu.Lock()
	d.srv = &http.Server{
		Addr:              d.listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	d.srvMu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("webchat listening", "client_id", d.id, "addr", d.listen, "path", d.basePath)
		err := d.srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-time.After(150 * time.Millisecond):
		return nil
	case err := <-errCh:
		if err != nil {
			_ = d.srv.Close()
			return fmt.Errorf("webchat: listen %q: %w", d.listen, err)
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = d.srv.Shutdown(shutdownCtx)
		return ctx.Err()
	}
}

func (d *driver) registerRoutes(mux *http.ServeMux) {
	p := d.basePath
	join := func(seg string) string {
		if p == "/" {
			return seg
		}
		return p + seg
	}
	if p != "/" {
		mux.HandleFunc("GET "+p, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != p {
				http.NotFound(w, r)
				return
			}
			http.Redirect(w, r, p+"/", http.StatusFound)
		})
	}
	mux.HandleFunc("GET "+join("/"), d.handleGETRoot)
	mux.HandleFunc("POST "+join("/api/send"), d.handleSend)
	mux.HandleFunc("GET "+join("/api/events"), d.handleSSE)
}

func (d *driver) Stop(ctx context.Context) error {
	d.closed.Store(true)
	d.srvMu.Lock()
	srv := d.srv
	d.srvMu.Unlock()
	if srv == nil {
		return nil
	}
	shutdownCtx := ctx
	if shutdownCtx == nil {
		shutdownCtx = context.Background()
	}
	err := srv.Shutdown(shutdownCtx)
	d.closeAllSSE()
	return err
}

func (d *driver) closeAllSSE() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for ch := range d.sseSubs {
		delete(d.sseSubs, ch)
	}
}

func (d *driver) handleGETRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != d.routePath("/") {
		http.NotFound(w, r)
		return
	}
	raw, err := staticUI.ReadFile("static/index.html")
	if err != nil {
		slog.Error("webchat embed", "err", err)
		http.Error(w, "ui missing", http.StatusInternalServerError)
		return
	}
	meta := `<meta name="base-path" content="` + html.EscapeString(d.basePath) + `" />`
	s := strings.Replace(string(raw), "<head>", "<head>\n    "+meta, 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(s))
}

func (d *driver) routePath(suffix string) string {
	if d.basePath == "/" {
		return suffix
	}
	if suffix == "/" {
		return d.basePath + "/"
	}
	return d.basePath + suffix
}

type sendBody struct {
	ChatID  string `json:"chat_id"`
	Content string `json:"content"`
}

func (d *driver) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	var body sendBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	chatID := strings.TrimSpace(body.ChatID)
	if !validChatID(chatID) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid chat_id"}`))
		return
	}
	text := strings.TrimSpace(body.Content)
	if text == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"empty content"}`))
		return
	}
	n := d.inboundSeq.Add(1)
	msgID := "in-" + strconv.FormatUint(n, 10)
	ev := chatEvent{Type: "message", Role: "user", ChatID: chatID, ID: msgID, Text: text}
	d.broadcastSSE(ev)

	in := &bus.InboundMessage{
		Channel:    d.id,
		ChatID:     chatID,
		MessageID:  msgID,
		ReceivedAt: time.Now().Unix(),
		Sender: bus.SenderInfo{
			Platform:    "webchat",
			PlatformID:  "browser",
			CanonicalID: "webchat:browser",
			DisplayName: d.senderName,
		},
		Peer:    bus.Peer{Kind: "direct", ID: "browser"},
		Content: text,
		Metadata: map[string]string{
			"webchat_session": chatID,
		},
	}
	if err := d.bus.PublishInbound(r.Context(), in); err != nil {
		slog.Warn("webchat PublishInbound", "err", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"bus unavailable"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true,"message_id":"` + msgID + `"}`))
}

func (d *driver) handleSSE(w http.ResponseWriter, r *http.Request) {
	chatID := strings.TrimSpace(r.URL.Query().Get("chat_id"))
	if !validChatID(chatID) {
		http.Error(w, "invalid chat_id", http.StatusBadRequest)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no flush", http.StatusInternalServerError)
		return
	}
	d.mu.Lock()
	if len(d.sseSubs) >= maxSSEClient {
		d.mu.Unlock()
		http.Error(w, "too many clients", http.StatusServiceUnavailable)
		return
	}
	ch := make(chan []byte, 16)
	d.sseSubs[ch] = chatID
	d.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	fl.Flush()

	done := r.Context().Done()
	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-done:
			d.mu.Lock()
			delete(d.sseSubs, ch)
			d.mu.Unlock()
			return
		case <-keepAlive.C:
			_, _ = w.Write([]byte(": ping\n\n"))
			fl.Flush()
		case line, ok := <-ch:
			if !ok {
				return
			}
			_, _ = w.Write(line)
			fl.Flush()
		}
	}
}

func (d *driver) broadcastSSE(ev chatEvent) {
	line, err := json.Marshal(ev)
	if err != nil {
		return
	}
	payload := fmt.Appendf(nil, "event: chat\ndata: %s\n\n", line)
	target := ev.ChatID
	d.mu.Lock()
	defer d.mu.Unlock()
	for ch, subChat := range d.sseSubs {
		if subChat != target {
			continue
		}
		select {
		case ch <- payload:
		default:
		}
	}
}

func (d *driver) Send(ctx context.Context, msg *bus.OutboundMessage) (string, error) {
	_ = ctx
	if msg == nil {
		return "", errors.New("webchat: nil message")
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" && len(msg.Parts) == 0 {
		return "", nil
	}
	chatID := strings.TrimSpace(msg.To.ChatID)
	if chatID == "" {
		return "", nil
	}
	n := d.msgSeq.Add(1)
	outID := "out-" + strconv.FormatUint(n, 10)
	if text == "" && len(msg.Parts) > 0 {
		text = "[附件] " + msg.Parts[0].Filename
		if text == "[附件] " {
			text = "[附件]"
		}
	}
	ev := chatEvent{Type: "message", Role: "assistant", ChatID: chatID, ID: outID, Text: text}
	d.broadcastSSE(ev)

	key := bus.RecipientKey(msg.To)
	d.lastSentMu.Lock()
	d.lastSent[key] = outID
	d.lastSentMu.Unlock()
	return outID, nil
}

// UpdateStatus 实现 client.MessageStatusUpdater：在页面上显示处理状态。
func (d *driver) UpdateStatus(ctx context.Context, req *bus.UpdateStatusRequest) error {
	_ = ctx
	if req == nil {
		return errors.New("webchat: nil UpdateStatusRequest")
	}
	chatID := strings.TrimSpace(req.To.ChatID)
	if chatID == "" {
		return nil
	}
	ev := chatEvent{
		Type:      "status",
		ChatID:    chatID,
		MessageID: req.MessageID,
		State:     req.State,
	}
	d.broadcastSSE(ev)
	return nil
}

// EditMessage 实现 client.MessageEditor：更新已展示的助手气泡。
func (d *driver) EditMessage(ctx context.Context, req *bus.EditMessageRequest) error {
	_ = ctx
	if req == nil {
		return errors.New("webchat: nil EditMessageRequest")
	}
	chatID := strings.TrimSpace(req.To.ChatID)
	if chatID == "" {
		return nil
	}
	id := strings.TrimSpace(req.MessageID)
	if id == "" {
		key := bus.RecipientKey(req.To)
		d.lastSentMu.RLock()
		id = d.lastSent[key]
		d.lastSentMu.RUnlock()
	}
	if id == "" {
		return client.ErrCapabilityUnsupported
	}
	text := strings.TrimSpace(req.Text)
	ev := chatEvent{Type: "edit", ChatID: chatID, ID: id, Text: text}
	d.broadcastSSE(ev)
	return nil
}

var (
	_ client.MessageStatusUpdater = (*driver)(nil)
	_ client.MessageEditor        = (*driver)(nil)
)
