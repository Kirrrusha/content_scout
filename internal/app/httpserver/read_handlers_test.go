package httpserver

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

func TestTelegramMessagesRead(t *testing.T) {
	controller := &fakeTelegramReadController{}
	server := NewWithOptions(":0", nil, testLogger(), DefaultOptions(), nil, nil, nil, nil, nil, nil, nil, nil)
	server.SetTelegramReadController(controller)

	req := httptest.NewRequest(http.MethodPost, "/telegram/messages/read", bytes.NewBufferString(`{
		"telegram_user_id": 42,
		"messages": [{"user_id": 1, "telegram_chat_id": -1001, "message_id": 10}]
	}`))
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if controller.telegramUserID != 42 || len(controller.messages) != 1 {
		t.Fatalf("controller call = user %d, messages %+v", controller.telegramUserID, controller.messages)
	}
	if controller.messages[0].UserID != 1 || controller.messages[0].TelegramChatID != -1001 || controller.messages[0].MessageID != 10 {
		t.Fatalf("message = %+v", controller.messages[0])
	}
}

type fakeTelegramReadController struct {
	telegramUserID int64
	messages       []domain.CollectedMessage
}

func (f *fakeTelegramReadController) MarkCollectedMessagesRead(_ context.Context, telegramUserID int64, messages []domain.CollectedMessage) error {
	f.telegramUserID = telegramUserID
	f.messages = messages
	return nil
}
