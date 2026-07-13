package tdlib

import (
	"context"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

func TestSyncServiceSyncsFoldersAndChats(t *testing.T) {
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

	folders := newMemoryFolderRepo()
	chats := newMemoryChatRepo()
	client := &fakeClient{
		state: AuthorizationStateReady,
		folders: []domain.TelegramFolder{{
			TelegramID: 1,
			Name:       "Golang",
		}},
		mainChats: []domain.TelegramChat{
			{TelegramChatID: -1001, Title: "Backend", Type: domain.ChatTypeChannel, UnreadCount: 5},
			{TelegramChatID: 10, Title: "Personal", Type: domain.ChatTypePrivate, UnreadCount: 99},
		},
		archiveChats: []domain.TelegramChat{
			{TelegramChatID: -1002, Title: "Archive", Type: domain.ChatTypeGroup, UnreadCount: 1},
		},
	}
	service := NewSyncService(42, users, sessions, folders, chats, fakeFactory{client: client})
	service.now = func() time.Time { return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC) }

	result, err := service.Sync(ctx, 42)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.FoldersCount != 1 || result.ChatsCount != 2 {
		t.Fatalf("result = %+v, want 1 folder and 2 chats", result)
	}

	savedFolders, err := folders.ListByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListByUserID(folders) error = %v", err)
	}
	if len(savedFolders) != 1 || savedFolders[0].UserID != user.ID {
		t.Fatalf("folders = %+v", savedFolders)
	}

	savedChats, err := chats.ListByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListByUserID(chats) error = %v", err)
	}
	if len(savedChats) != 2 {
		t.Fatalf("chats len = %d, want 2", len(savedChats))
	}
	for _, chat := range savedChats {
		if chat.Type == domain.ChatTypePrivate {
			t.Fatalf("private chat was persisted: %+v", chat)
		}
		if chat.Title == "Archive" && !chat.IsArchived {
			t.Fatalf("archive chat was not marked archived: %+v", chat)
		}
	}
}

func TestSyncServiceRequiresReadySession(t *testing.T) {
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
		Status:      domain.SessionStatusAuthorizing,
	})
	if err != nil {
		t.Fatalf("session Upsert() error = %v", err)
	}

	service := NewSyncService(42, users, sessions, newMemoryFolderRepo(), newMemoryChatRepo(), fakeFactory{client: &fakeClient{state: AuthorizationStateWaitCode}})
	_, err = service.Sync(ctx, 42)
	if err == nil {
		t.Fatal("Sync() error = nil, want not connected error")
	}
}

type memoryFolderRepo struct {
	folders []domain.TelegramFolder
}

func newMemoryFolderRepo() *memoryFolderRepo {
	return &memoryFolderRepo{}
}

func (r *memoryFolderRepo) UpsertMany(_ context.Context, folders []domain.TelegramFolder) error {
	r.folders = append([]domain.TelegramFolder(nil), folders...)
	return nil
}

func (r *memoryFolderRepo) ListByUserID(_ context.Context, userID int64) ([]domain.TelegramFolder, error) {
	var out []domain.TelegramFolder
	for _, folder := range r.folders {
		if folder.UserID == userID {
			out = append(out, folder)
		}
	}
	return out, nil
}

type memoryChatRepo struct {
	chats []domain.TelegramChat
}

func newMemoryChatRepo() *memoryChatRepo {
	return &memoryChatRepo{}
}

func (r *memoryChatRepo) UpsertMany(_ context.Context, chats []domain.TelegramChat) error {
	r.chats = append([]domain.TelegramChat(nil), chats...)
	return nil
}

func (r *memoryChatRepo) ListByUserID(_ context.Context, userID int64) ([]domain.TelegramChat, error) {
	var out []domain.TelegramChat
	for _, chat := range r.chats {
		if chat.UserID == userID {
			out = append(out, chat)
		}
	}
	return out, nil
}

func (r *memoryChatRepo) FindByTelegramChatID(_ context.Context, userID, telegramChatID int64) (*domain.TelegramChat, error) {
	for _, chat := range r.chats {
		if chat.UserID == userID && chat.TelegramChatID == telegramChatID {
			return &chat, nil
		}
	}
	return nil, nil
}
