package telegram

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/lengzhao/agentkit"
	"github.com/lengzhao/agentkit/runtime/platform/common"
)

func (p *Platform) downloadFile(ctx context.Context, fileID string) ([]byte, error) {
	p.mu.RLock()
	bot := p.bot
	client := p.httpClient
	p.mu.RUnlock()
	if bot == nil {
		return nil, fmt.Errorf("telegram: bot not connected")
	}
	if client == nil {
		client = http.DefaultClient
	}
	f, err := bot.GetFile(ctx, &tgbot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, fmt.Errorf("telegram: get file: %w", err)
	}
	if f.FilePath == "" {
		return nil, fmt.Errorf("telegram: empty file_path for %s", fileID)
	}
	link := bot.FileDownloadLink(f)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram: download %s: %w", fileID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram: download %s: status %d", fileID, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (p *Platform) sendMediaPart(ctx context.Context, session agentkit.SessionID, part agentkit.ContentPart) error {
	raw, ok := p.routes.Load(session)
	if !ok {
		return fmt.Errorf("telegram: no delivery for session %q", session)
	}
	d := raw.(delivery)
	data, name, err := common.ReadMediaPart(part)
	if err != nil {
		return fmt.Errorf("telegram: %w", err)
	}
	if name == "" {
		name = "attachment"
	}
	p.mu.RLock()
	bot := p.bot
	p.mu.RUnlock()
	if bot == nil {
		return fmt.Errorf("telegram: bot not connected")
	}
	upload := &models.InputFileUpload{Filename: name, Data: bytes.NewReader(data)}
	switch part.Type {
	case "image":
		_, err = bot.SendPhoto(ctx, &tgbot.SendPhotoParams{
			ChatID:          d.chatID,
			MessageThreadID: d.threadID,
			Photo:           upload,
		})
	case "document":
		_, err = bot.SendDocument(ctx, &tgbot.SendDocumentParams{
			ChatID:          d.chatID,
			MessageThreadID: d.threadID,
			Document:        upload,
		})
	default:
		return fmt.Errorf("telegram: unsupported outbound media type %q", part.Type)
	}
	if err != nil {
		return fmt.Errorf("telegram: send %s: %w", part.Type, err)
	}
	return nil
}
