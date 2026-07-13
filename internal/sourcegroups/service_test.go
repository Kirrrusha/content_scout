package sourcegroups

import (
	"context"
	"testing"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

func TestServiceCreatesGroupAndAddsChat(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserRepo()
	user, err := users.UpsertByTelegramID(ctx, 42)
	if err != nil {
		t.Fatalf("UpsertByTelegramID() error = %v", err)
	}
	groups := newMemoryGroupRepo()
	chats := newMemoryChatRepo([]domain.TelegramChat{{
		ID:             10,
		UserID:         user.ID,
		TelegramChatID: -100,
		Title:          "Backend",
		Type:           domain.ChatTypeChannel,
	}})
	service := NewService(42, users, groups, chats)

	group, err := service.Create(ctx, 42, "Golang", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := service.AddChat(ctx, 42, group.ID, 10, 5, true); err != nil {
		t.Fatalf("AddChat() error = %v", err)
	}

	withChats, err := service.ListChats(ctx, 42, group.ID)
	if err != nil {
		t.Fatalf("ListChats() error = %v", err)
	}
	if len(withChats.Chats) != 1 || withChats.Chats[0].Title != "Backend" {
		t.Fatalf("withChats = %+v", withChats)
	}
	if len(withChats.Links) != 1 || withChats.Links[0].Priority != 5 {
		t.Fatalf("links = %+v", withChats.Links)
	}
}

func TestServiceRejectsForeignOwner(t *testing.T) {
	service := NewService(42, newMemoryUserRepo(), newMemoryGroupRepo(), newMemoryChatRepo(nil))

	_, err := service.List(context.Background(), 99)
	if err == nil {
		t.Fatal("List() error = nil, want error")
	}
}

func TestServiceRequiresExistingChat(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserRepo()
	user, err := users.UpsertByTelegramID(ctx, 42)
	if err != nil {
		t.Fatalf("UpsertByTelegramID() error = %v", err)
	}
	groups := newMemoryGroupRepo()
	service := NewService(42, users, groups, newMemoryChatRepo(nil))
	group, err := groups.Create(ctx, domain.SourceGroup{UserID: user.ID, Name: "Golang"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err = service.AddChat(ctx, 42, group.ID, 999, 0, true)
	if err != ErrChatNotFound {
		t.Fatalf("AddChat() error = %v, want ErrChatNotFound", err)
	}
}

type memoryUserRepo struct {
	nextID int64
	users  map[int64]domain.User
}

func newMemoryUserRepo() *memoryUserRepo {
	return &memoryUserRepo{nextID: 1, users: make(map[int64]domain.User)}
}

func (r *memoryUserRepo) UpsertByTelegramID(_ context.Context, telegramUserID int64) (*domain.User, error) {
	if user, ok := r.users[telegramUserID]; ok {
		return &user, nil
	}
	user := domain.User{ID: r.nextID, TelegramUserID: telegramUserID}
	r.nextID++
	r.users[telegramUserID] = user
	return &user, nil
}

func (r *memoryUserRepo) FindByTelegramID(_ context.Context, telegramUserID int64) (*domain.User, error) {
	user, ok := r.users[telegramUserID]
	if !ok {
		return nil, nil
	}
	return &user, nil
}

type memoryGroupRepo struct {
	nextID int64
	groups map[int64]domain.SourceGroup
	links  []domain.SourceGroupChat
}

func newMemoryGroupRepo() *memoryGroupRepo {
	return &memoryGroupRepo{nextID: 1, groups: make(map[int64]domain.SourceGroup)}
}

func (r *memoryGroupRepo) Create(_ context.Context, group domain.SourceGroup) (*domain.SourceGroup, error) {
	group.ID = r.nextID
	r.nextID++
	r.groups[group.ID] = group
	return &group, nil
}

func (r *memoryGroupRepo) Update(_ context.Context, group domain.SourceGroup) (*domain.SourceGroup, error) {
	if _, ok := r.groups[group.ID]; !ok {
		return nil, nil
	}
	r.groups[group.ID] = group
	return &group, nil
}

func (r *memoryGroupRepo) Delete(_ context.Context, _, groupID int64) error {
	delete(r.groups, groupID)
	return nil
}

func (r *memoryGroupRepo) ListByUserID(_ context.Context, userID int64) ([]domain.SourceGroup, error) {
	var out []domain.SourceGroup
	for _, group := range r.groups {
		if group.UserID == userID {
			out = append(out, group)
		}
	}
	return out, nil
}

func (r *memoryGroupRepo) AddChat(_ context.Context, link domain.SourceGroupChat) error {
	r.links = append(r.links, link)
	return nil
}

func (r *memoryGroupRepo) RemoveChat(_ context.Context, groupID, chatID int64) error {
	var out []domain.SourceGroupChat
	for _, link := range r.links {
		if link.GroupID == groupID && link.ChatID == chatID {
			continue
		}
		out = append(out, link)
	}
	r.links = out
	return nil
}

func (r *memoryGroupRepo) ListChats(_ context.Context, groupID int64) ([]domain.SourceGroupChat, error) {
	var out []domain.SourceGroupChat
	for _, link := range r.links {
		if link.GroupID == groupID {
			out = append(out, link)
		}
	}
	return out, nil
}

type memoryChatRepo struct {
	chats []domain.TelegramChat
}

func newMemoryChatRepo(chats []domain.TelegramChat) *memoryChatRepo {
	return &memoryChatRepo{chats: chats}
}

func (r *memoryChatRepo) UpsertMany(context.Context, []domain.TelegramChat) error {
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
