package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

func TestTelegramSyncHandler(t *testing.T) {
	sync := &fakeHTTPSync{
		result: &tdlib.SyncResult{
			UserID:       1,
			FoldersCount: 2,
			ChatsCount:   3,
			SyncedAt:     time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		},
	}
	server := NewWithControllers(":0", nil, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), nil, sync)

	req := httptest.NewRequest(http.MethodPost, "/telegram/sync", bytes.NewBufferString(`{"telegram_user_id":42}`))
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !sync.synced {
		t.Fatal("sync was not called")
	}

	var response syncResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.FoldersCount != 2 || response.ChatsCount != 3 {
		t.Fatalf("response = %+v", response)
	}
}

func TestTelegramChatsHandler(t *testing.T) {
	sync := &fakeHTTPSync{
		chats: []domain.TelegramChat{{
			ID:             1,
			TelegramChatID: -100,
			Title:          "Backend",
			Type:           domain.ChatTypeChannel,
			UnreadCount:    5,
		}},
	}
	server := NewWithControllers(":0", nil, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), nil, sync)

	req := httptest.NewRequest(http.MethodGet, "/telegram/chats?telegram_user_id=42", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	var response []chatResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response) != 1 || response[0].Title != "Backend" || response[0].UnreadCount != 5 {
		t.Fatalf("response = %+v", response)
	}
}

type fakeHTTPSync struct {
	synced  bool
	result  *tdlib.SyncResult
	folders []domain.TelegramFolder
	chats   []domain.TelegramChat
}

func (f *fakeHTTPSync) Sync(context.Context, int64) (*tdlib.SyncResult, error) {
	f.synced = true
	return f.result, nil
}

func (f *fakeHTTPSync) ListFolders(context.Context, int64) ([]domain.TelegramFolder, error) {
	return f.folders, nil
}

func (f *fakeHTTPSync) ListChats(context.Context, int64) ([]domain.TelegramChat, error) {
	return f.chats, nil
}
