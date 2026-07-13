package sourcegroups

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/storage"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

var (
	ErrGroupNotFound = errors.New("source group not found")
	ErrChatNotFound  = errors.New("telegram chat not found")
)

type Service struct {
	ownerTelegramID int64
	users           storage.UserRepository
	groups          storage.SourceGroupRepository
	chats           storage.TelegramChatRepository
}

type GroupWithChats struct {
	Group domain.SourceGroup
	Links []domain.SourceGroupChat
	Chats []domain.TelegramChat
}

func NewService(ownerTelegramID int64, users storage.UserRepository, groups storage.SourceGroupRepository, chats storage.TelegramChatRepository) *Service {
	return &Service{
		ownerTelegramID: ownerTelegramID,
		users:           users,
		groups:          groups,
		chats:           chats,
	}
}

func (s *Service) Create(ctx context.Context, telegramUserID int64, name, description string) (*domain.SourceGroup, error) {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("source group name is required")
	}
	return s.groups.Create(ctx, domain.SourceGroup{
		UserID:      user.ID,
		Name:        name,
		Description: strings.TrimSpace(description),
	})
}

func (s *Service) Update(ctx context.Context, telegramUserID, groupID int64, name, description string) (*domain.SourceGroup, error) {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("source group name is required")
	}
	updated, err := s.groups.Update(ctx, domain.SourceGroup{
		ID:          groupID,
		UserID:      user.ID,
		Name:        name,
		Description: strings.TrimSpace(description),
	})
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, ErrGroupNotFound
	}
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, telegramUserID, groupID int64) error {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return err
	}
	if _, err := s.requireGroup(ctx, user.ID, groupID); err != nil {
		return err
	}
	return s.groups.Delete(ctx, user.ID, groupID)
}

func (s *Service) List(ctx context.Context, telegramUserID int64) ([]domain.SourceGroup, error) {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	return s.groups.ListByUserID(ctx, user.ID)
}

func (s *Service) AddChat(ctx context.Context, telegramUserID, groupID, chatID int64, priority int, enabled bool) error {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return err
	}
	if _, err := s.requireGroup(ctx, user.ID, groupID); err != nil {
		return err
	}
	if _, err := s.requireChat(ctx, user.ID, chatID); err != nil {
		return err
	}
	return s.groups.AddChat(ctx, domain.SourceGroupChat{
		GroupID:  groupID,
		ChatID:   chatID,
		Priority: priority,
		Enabled:  enabled,
	})
}

func (s *Service) RemoveChat(ctx context.Context, telegramUserID, groupID, chatID int64) error {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return err
	}
	if _, err := s.requireGroup(ctx, user.ID, groupID); err != nil {
		return err
	}
	return s.groups.RemoveChat(ctx, groupID, chatID)
}

func (s *Service) ListChats(ctx context.Context, telegramUserID, groupID int64) (*GroupWithChats, error) {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	group, err := s.requireGroup(ctx, user.ID, groupID)
	if err != nil {
		return nil, err
	}
	links, err := s.groups.ListChats(ctx, groupID)
	if err != nil {
		return nil, err
	}
	userChats, err := s.chats.ListByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	chatByID := make(map[int64]domain.TelegramChat, len(userChats))
	for _, chat := range userChats {
		chatByID[chat.ID] = chat
	}

	details := make([]domain.TelegramChat, 0, len(links))
	for _, link := range links {
		if chat, ok := chatByID[link.ChatID]; ok {
			details = append(details, chat)
		}
	}
	sort.SliceStable(details, func(i, j int) bool {
		return strings.ToLower(details[i].Title) < strings.ToLower(details[j].Title)
	})
	return &GroupWithChats{Group: *group, Links: links, Chats: details}, nil
}

func (s *Service) ownerUser(ctx context.Context, telegramUserID int64) (*domain.User, error) {
	if s.ownerTelegramID == 0 || telegramUserID != s.ownerTelegramID {
		return nil, tdlib.ErrUnauthorizedOwner
	}
	user, err := s.users.FindByTelegramID(ctx, telegramUserID)
	if err != nil {
		return nil, fmt.Errorf("find owner user: %w", err)
	}
	if user == nil {
		return nil, errors.New("owner user is not initialized")
	}
	return user, nil
}

func (s *Service) requireGroup(ctx context.Context, userID, groupID int64) (*domain.SourceGroup, error) {
	groups, err := s.groups.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if group.ID == groupID {
			return &group, nil
		}
	}
	return nil, ErrGroupNotFound
}

func (s *Service) requireChat(ctx context.Context, userID, chatID int64) (*domain.TelegramChat, error) {
	chats, err := s.chats.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, chat := range chats {
		if chat.ID == chatID {
			return &chat, nil
		}
	}
	return nil, ErrChatNotFound
}
