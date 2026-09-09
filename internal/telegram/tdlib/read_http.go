package tdlib

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

// HTTPReadMarker delegates Telegram mutations to the API process that owns the
// native TDLib session. This avoids opening the same TDLib database from a
// background worker process.
type HTTPReadMarker struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type ReadMessagesRequest struct {
	TelegramUserID int64                  `json:"telegram_user_id"`
	Messages       []ReadMessageReference `json:"messages"`
}

type ReadMessageReference struct {
	UserID         int64 `json:"user_id"`
	TelegramChatID int64 `json:"telegram_chat_id"`
	MessageID      int64 `json:"message_id"`
}

func NewHTTPReadMarker(baseURL, token string, httpClient *http.Client) *HTTPReadMarker {
	return &HTTPReadMarker{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: httpClient,
	}
}

func (m *HTTPReadMarker) MarkCollectedMessagesRead(ctx context.Context, telegramUserID int64, messages []domain.CollectedMessage) error {
	if len(messages) == 0 {
		return nil
	}
	if m.baseURL == "" {
		return errors.New("internal API URL is not configured")
	}
	if m.httpClient == nil {
		return errors.New("HTTP client is not configured")
	}

	references := make([]ReadMessageReference, 0, len(messages))
	for _, message := range messages {
		references = append(references, ReadMessageReference{
			UserID:         message.UserID,
			TelegramChatID: message.TelegramChatID,
			MessageID:      message.MessageID,
		})
	}
	payload, err := json.Marshal(ReadMessagesRequest{TelegramUserID: telegramUserID, Messages: references})
	if err != nil {
		return fmt.Errorf("encode read request: %w", err)
	}
	endpoint, err := url.JoinPath(m.baseURL, "telegram/messages/read")
	if err != nil {
		return fmt.Errorf("build read endpoint: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create read request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if m.token != "" {
		req.Header.Set("Authorization", "Bearer "+m.token)
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send read request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var apiError struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&apiError); err == nil && apiError.Error != "" {
			return errors.New(apiError.Error)
		}
		return fmt.Errorf("read request failed: %s", resp.Status)
	}
	return nil
}

func CollectedMessagesFromReadRequest(req ReadMessagesRequest) []domain.CollectedMessage {
	messages := make([]domain.CollectedMessage, 0, len(req.Messages))
	for _, message := range req.Messages {
		messages = append(messages, domain.CollectedMessage{
			UserID:         message.UserID,
			TelegramChatID: message.TelegramChatID,
			MessageID:      message.MessageID,
		})
	}
	return messages
}
