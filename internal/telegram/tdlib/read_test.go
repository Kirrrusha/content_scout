package tdlib

import (
	"context"
	"reflect"
	"testing"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

func TestReadServiceMarksCollectedMessagesRead(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserRepo()
	sessions := newMemorySessionRepo()
	user, err := users.UpsertByTelegramID(ctx, 42)
	if err != nil {
		t.Fatalf("UpsertByTelegramID() error = %v", err)
	}
	_, err = sessions.Upsert(ctx, domain.TelegramSession{
		UserID:      user.ID,
		StoragePath: "/tmp/tdlib",
		Status:      domain.SessionStatusConnected,
	})
	if err != nil {
		t.Fatalf("session Upsert() error = %v", err)
	}

	client := &fakeClient{state: AuthorizationStateReady}
	service := NewReadService(42, users, sessions, fakeFactory{client: client})

	err = service.MarkCollectedMessagesRead(ctx, 42, []domain.CollectedMessage{
		{UserID: user.ID, TelegramChatID: -100, MessageID: 3},
		{UserID: user.ID, TelegramChatID: -100, MessageID: 1},
		{UserID: user.ID, TelegramChatID: -100, MessageID: 3},
		{UserID: user.ID, TelegramChatID: -200, MessageID: 2},
		{UserID: user.ID + 1, TelegramChatID: -300, MessageID: 9},
	})
	if err != nil {
		t.Fatalf("MarkCollectedMessagesRead() error = %v", err)
	}
	if !reflect.DeepEqual(client.readMessages[-100], []int64{1, 3}) {
		t.Fatalf("chat -100 read messages = %+v", client.readMessages[-100])
	}
	if !reflect.DeepEqual(client.readMessages[-200], []int64{2}) {
		t.Fatalf("chat -200 read messages = %+v", client.readMessages[-200])
	}
	if _, ok := client.readMessages[-300]; ok {
		t.Fatalf("unexpected other-user messages marked read: %+v", client.readMessages[-300])
	}
}
