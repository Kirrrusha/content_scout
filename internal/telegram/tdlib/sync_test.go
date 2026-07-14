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
	groups := newMemorySourceGroupRepo()
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
		folderChats: map[int32][]domain.TelegramChat{
			1: {{TelegramChatID: -1003, Title: "Folder Backend", Type: domain.ChatTypeChannel, UnreadCount: 2}},
		},
	}
	service := NewSyncService(42, users, sessions, folders, chats, groups, fakeFactory{client: client})
	service.now = func() time.Time { return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC) }

	result, err := service.Sync(ctx, 42)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.FoldersCount != 1 || result.ChatsCount != 3 {
		t.Fatalf("result = %+v, want 1 folder and 3 chats", result)
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
	if len(savedChats) != 3 {
		t.Fatalf("chats len = %d, want 3", len(savedChats))
	}
	for _, chat := range savedChats {
		if chat.Type == domain.ChatTypePrivate {
			t.Fatalf("private chat was persisted: %+v", chat)
		}
		if chat.Title == "Archive" && !chat.IsArchived {
			t.Fatalf("archive chat was not marked archived: %+v", chat)
		}
	}
	sourceGroups, err := groups.ListByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListByUserID(groups) error = %v", err)
	}
	if len(sourceGroups) != 1 || sourceGroups[0].Name != "Golang" {
		t.Fatalf("source groups = %+v", sourceGroups)
	}
	links, err := groups.ListChats(ctx, sourceGroups[0].ID)
	if err != nil {
		t.Fatalf("ListChats(groups) error = %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("source group links = %+v, want 1", links)
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

	service := NewSyncService(42, users, sessions, newMemoryFolderRepo(), newMemoryChatRepo(), nil, fakeFactory{client: &fakeClient{state: AuthorizationStateWaitCode}})
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
	nextID int64
	chats  []domain.TelegramChat
}

func newMemoryChatRepo() *memoryChatRepo {
	return &memoryChatRepo{nextID: 1}
}

func (r *memoryChatRepo) UpsertMany(_ context.Context, chats []domain.TelegramChat) error {
	byTelegramID := make(map[int64]domain.TelegramChat, len(r.chats))
	for _, chat := range r.chats {
		byTelegramID[chat.TelegramChatID] = chat
	}
	for _, chat := range chats {
		if existing, ok := byTelegramID[chat.TelegramChatID]; ok {
			chat.ID = existing.ID
		} else {
			chat.ID = r.nextID
			r.nextID++
		}
		byTelegramID[chat.TelegramChatID] = chat
	}
	r.chats = r.chats[:0]
	for _, chat := range byTelegramID {
		r.chats = append(r.chats, chat)
	}
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

type memorySourceGroupRepo struct {
	nextID int64
	groups map[int64]domain.SourceGroup
	links  []domain.SourceGroupChat
}

func newMemorySourceGroupRepo() *memorySourceGroupRepo {
	return &memorySourceGroupRepo{nextID: 1, groups: make(map[int64]domain.SourceGroup)}
}

func (r *memorySourceGroupRepo) Create(_ context.Context, group domain.SourceGroup) (*domain.SourceGroup, error) {
	group.ID = r.nextID
	r.nextID++
	r.groups[group.ID] = group
	return &group, nil
}

func (r *memorySourceGroupRepo) Update(_ context.Context, group domain.SourceGroup) (*domain.SourceGroup, error) {
	if _, ok := r.groups[group.ID]; !ok {
		return nil, nil
	}
	r.groups[group.ID] = group
	return &group, nil
}

func (r *memorySourceGroupRepo) Delete(_ context.Context, _ int64, groupID int64) error {
	delete(r.groups, groupID)
	return nil
}

func (r *memorySourceGroupRepo) ListByUserID(_ context.Context, userID int64) ([]domain.SourceGroup, error) {
	var out []domain.SourceGroup
	for _, group := range r.groups {
		if group.UserID == userID {
			out = append(out, group)
		}
	}
	return out, nil
}

func (r *memorySourceGroupRepo) AddChat(_ context.Context, link domain.SourceGroupChat) error {
	for i, existing := range r.links {
		if existing.GroupID == link.GroupID && existing.ChatID == link.ChatID {
			r.links[i] = link
			return nil
		}
	}
	r.links = append(r.links, link)
	return nil
}

func (r *memorySourceGroupRepo) RemoveChat(_ context.Context, groupID, chatID int64) error {
	for i, link := range r.links {
		if link.GroupID == groupID && link.ChatID == chatID {
			r.links = append(r.links[:i], r.links[i+1:]...)
			return nil
		}
	}
	return nil
}

func (r *memorySourceGroupRepo) ListChats(_ context.Context, groupID int64) ([]domain.SourceGroupChat, error) {
	var out []domain.SourceGroupChat
	for _, link := range r.links {
		if link.GroupID == groupID {
			out = append(out, link)
		}
	}
	return out, nil
}
