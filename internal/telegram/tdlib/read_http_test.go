package tdlib

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

func TestHTTPReadMarkerDelegatesMessagesToAPI(t *testing.T) {
	var received ReadMessagesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/telegram/messages/read" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	marker := NewHTTPReadMarker(server.URL, "secret", server.Client())
	err := marker.MarkCollectedMessagesRead(context.Background(), 42, []domain.CollectedMessage{
		{UserID: 1, TelegramChatID: -1001, MessageID: 10},
		{UserID: 1, TelegramChatID: -1002, MessageID: 20},
	})
	if err != nil {
		t.Fatalf("MarkCollectedMessagesRead() error = %v", err)
	}
	if received.TelegramUserID != 42 || len(received.Messages) != 2 {
		t.Fatalf("request = %+v", received)
	}
	if received.Messages[1].TelegramChatID != -1002 || received.Messages[1].MessageID != 20 {
		t.Fatalf("second message = %+v", received.Messages[1])
	}
}

func TestHTTPReadMarkerReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, http.StatusConflict, map[string]string{"error": "tdlib session is busy"})
	}))
	defer server.Close()

	marker := NewHTTPReadMarker(server.URL, "", server.Client())
	err := marker.MarkCollectedMessagesRead(context.Background(), 42, []domain.CollectedMessage{{MessageID: 10}})
	if err == nil || err.Error() != "tdlib session is busy" {
		t.Fatalf("error = %v", err)
	}
}

func writeTestJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
