package tdlib

import (
	"errors"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

func TestMapAuthorizationState(t *testing.T) {
	if got := mapAuthorizationState("authorizationStateReady"); got != AuthorizationStateReady {
		t.Fatalf("state = %s, want ready", got)
	}
	if got := mapAuthorizationState("authorizationStateWaitCode"); got != AuthorizationStateWaitCode {
		t.Fatalf("state = %s, want wait code", got)
	}
}

func TestMapChat(t *testing.T) {
	chat := mapChat(map[string]any{
		"id":           float64(-1001),
		"title":        "Backend",
		"type":         map[string]any{"@type": "chatTypeSupergroup"},
		"unread_count": float64(7),
		"notification_settings": map[string]any{
			"mute_for": float64(3600),
		},
		"last_message": map[string]any{
			"id": float64(55),
		},
	}, true)

	if chat.TelegramChatID != -1001 || chat.Title != "Backend" {
		t.Fatalf("chat identity = %+v", chat)
	}
	if chat.Type != domain.ChatTypeSupergroup || !chat.IsArchived || !chat.IsMuted {
		t.Fatalf("chat flags = %+v", chat)
	}
	if chat.UnreadCount != 7 || chat.LastMessageID != 55 {
		t.Fatalf("chat counters = %+v", chat)
	}
}

func TestMapChannelChat(t *testing.T) {
	chat := mapChat(map[string]any{
		"id":    float64(-1002),
		"title": "News",
		"type": map[string]any{
			"@type":      "chatTypeSupergroup",
			"is_channel": true,
		},
	}, false)

	if chat.Type != domain.ChatTypeChannel {
		t.Fatalf("chat type = %s, want channel", chat.Type)
	}
}

func TestMapChatFolders(t *testing.T) {
	folders := mapChatFolders(map[string]any{
		"@type": "updateChatFolders",
		"chat_folders": []any{
			map[string]any{
				"id": float64(1),
				"name": map[string]any{
					"@type": "chatFolderName",
					"text": map[string]any{
						"@type": "formattedText",
						"text":  "Work",
					},
				},
			},
		},
	})

	if len(folders) != 1 {
		t.Fatalf("folders len = %d, want 1", len(folders))
	}
	if folders[0].TelegramID != 1 || folders[0].Name != "Work" {
		t.Fatalf("folder = %+v", folders[0])
	}
}

func TestIsChatsFullyLoadedError(t *testing.T) {
	if !isChatsFullyLoadedError(&TDLibError{Code: 404, Message: "Not Found"}) {
		t.Fatal("404 TDLib error should mean chat list is fully loaded")
	}
	if isChatsFullyLoadedError(&TDLibError{Code: 400, Message: "Bad Request"}) {
		t.Fatal("non-404 TDLib error should not mean chat list is fully loaded")
	}
	if isChatsFullyLoadedError(errors.New("network failed")) {
		t.Fatal("generic error should not mean chat list is fully loaded")
	}
}

func TestIsUnexpectedSetTDLibParametersError(t *testing.T) {
	err := &TDLibError{Code: 400, Message: "Unexpected setTdlibParameters"}
	if !isUnexpectedSetTDLibParametersError(err) {
		t.Fatal("Unexpected setTdlibParameters should be recognized")
	}
	if isUnexpectedSetTDLibParametersError(&TDLibError{Code: 400, Message: "PHONE_NUMBER_INVALID"}) {
		t.Fatal("unrelated TDLib error should not be recognized")
	}
}

func TestMapMessageText(t *testing.T) {
	message := mapMessage(map[string]any{
		"id":        float64(123),
		"chat_id":   float64(-1001),
		"date":      float64(1783944000),
		"edit_date": float64(1783947600),
		"sender_id": map[string]any{
			"@type":   "messageSenderUser",
			"user_id": float64(42),
		},
		"reply_to": map[string]any{
			"@type":      "messageReplyToMessage",
			"message_id": float64(100),
		},
		"content": map[string]any{
			"@type": "messageText",
			"text": map[string]any{
				"text": "hello",
			},
			"web_page": map[string]any{
				"url": "https://example.com",
			},
		},
	})

	if message.MessageID != 123 || message.ChatID != -1001 {
		t.Fatalf("message identity = %+v", message)
	}
	if message.Text != "hello" || message.URL != "https://example.com" {
		t.Fatalf("message content = %+v", message)
	}
	if message.SenderID != 42 || message.SenderName != "user:42" {
		t.Fatalf("sender = %+v", message)
	}
	if message.ReplyToID == nil || *message.ReplyToID != 100 {
		t.Fatalf("reply = %+v", message.ReplyToID)
	}
	if want := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC); !message.Date.Equal(want) {
		t.Fatalf("date = %s, want %s", message.Date, want)
	}
	if message.EditDate == nil {
		t.Fatal("edit date is nil")
	}
}

func TestMapMessageMedia(t *testing.T) {
	message := mapMessage(map[string]any{
		"id":      float64(124),
		"chat_id": float64(-1001),
		"content": map[string]any{
			"@type": "messagePhoto",
			"caption": map[string]any{
				"text": "caption",
			},
		},
	})

	if !message.HasMedia || message.MediaType != "photo" {
		t.Fatalf("media fields = %+v", message)
	}
	if message.Caption != "caption" {
		t.Fatalf("caption = %q", message.Caption)
	}
}
