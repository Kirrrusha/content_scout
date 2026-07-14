package tdlib

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/storage"
)

type SyncResult struct {
	UserID       int64
	FoldersCount int
	ChatsCount   int
	SyncedAt     time.Time
}

type SyncService struct {
	ownerTelegramID int64
	users           storage.UserRepository
	sessions        storage.TelegramSessionRepository
	folders         storage.TelegramFolderRepository
	chats           storage.TelegramChatRepository
	groups          storage.SourceGroupRepository
	factory         ClientFactory
	now             func() time.Time
}

func NewSyncService(ownerTelegramID int64, users storage.UserRepository, sessions storage.TelegramSessionRepository, folders storage.TelegramFolderRepository, chats storage.TelegramChatRepository, groups storage.SourceGroupRepository, factory ClientFactory) *SyncService {
	return &SyncService{
		ownerTelegramID: ownerTelegramID,
		users:           users,
		sessions:        sessions,
		folders:         folders,
		chats:           chats,
		groups:          groups,
		factory:         factory,
		now:             time.Now,
	}
}

func (s *SyncService) Sync(ctx context.Context, telegramUserID int64) (*SyncResult, error) {
	user, session, client, err := s.readyClient(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}

	folders, err := client.ListFolders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list telegram folders: %w", err)
	}
	syncedAt := s.now()
	for i := range folders {
		folders[i].UserID = user.ID
		folders[i].SyncedAt = syncedAt
	}
	if err := s.folders.UpsertMany(ctx, folders); err != nil {
		return nil, fmt.Errorf("persist telegram folders: %w", err)
	}

	mainChats, err := client.ListChats(ctx, ChatListMain)
	if err != nil {
		return nil, fmt.Errorf("list main telegram chats: %w", err)
	}
	archiveChats, err := client.ListChats(ctx, ChatListArchive)
	if err != nil {
		return nil, fmt.Errorf("list archived telegram chats: %w", err)
	}
	allChats := normalizeChats(user.ID, mainChats, false)
	allChats = append(allChats, normalizeChats(user.ID, archiveChats, true)...)
	folderChatsByID := make(map[int32][]domain.TelegramChat, len(folders))
	for _, folder := range folders {
		chats, err := client.ListFolderChats(ctx, folder.TelegramID)
		if err != nil {
			return nil, fmt.Errorf("list telegram folder chats: %w", err)
		}
		normalized := normalizeChats(user.ID, chats, false)
		folderChatsByID[folder.TelegramID] = normalized
		allChats = append(allChats, normalized...)
	}
	allChats = filterPersonalChats(allChats)

	if err := s.chats.UpsertMany(ctx, allChats); err != nil {
		return nil, fmt.Errorf("persist telegram chats: %w", err)
	}
	if err := s.syncSourceGroups(ctx, user.ID, folders, folderChatsByID); err != nil {
		return nil, err
	}

	connectedAt := syncedAt
	_, err = s.sessions.Upsert(ctx, domain.TelegramSession{
		UserID:        user.ID,
		StoragePath:   session.StoragePath,
		Status:        domain.SessionStatusConnected,
		LastConnected: &connectedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("update telegram session sync timestamp: %w", err)
	}

	return &SyncResult{
		UserID:       user.ID,
		FoldersCount: len(folders),
		ChatsCount:   len(allChats),
		SyncedAt:     syncedAt,
	}, nil
}

func (s *SyncService) syncSourceGroups(ctx context.Context, userID int64, folders []domain.TelegramFolder, folderChatsByID map[int32][]domain.TelegramChat) error {
	if s.groups == nil {
		return nil
	}
	sourceGroups, err := s.groups.ListByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("list source groups: %w", err)
	}
	groupByName := make(map[string]domain.SourceGroup, len(sourceGroups))
	for _, group := range sourceGroups {
		groupByName[normalizeGroupName(group.Name)] = group
	}
	savedChats, err := s.chats.ListByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("list synced chats for source groups: %w", err)
	}
	chatByTelegramID := make(map[int64]domain.TelegramChat, len(savedChats))
	for _, chat := range savedChats {
		chatByTelegramID[chat.TelegramChatID] = chat
	}
	for _, folder := range folders {
		name := strings.TrimSpace(folder.Name)
		if name == "" {
			continue
		}
		group, ok := groupByName[normalizeGroupName(name)]
		if !ok {
			created, err := s.groups.Create(ctx, domain.SourceGroup{UserID: userID, Name: name})
			if err != nil {
				return fmt.Errorf("create source group from folder: %w", err)
			}
			group = *created
			groupByName[normalizeGroupName(name)] = group
		}
		for _, folderChat := range folderChatsByID[folder.TelegramID] {
			chat, ok := chatByTelegramID[folderChat.TelegramChatID]
			if !ok || chat.Type == domain.ChatTypePrivate {
				continue
			}
			if err := s.groups.AddChat(ctx, domain.SourceGroupChat{GroupID: group.ID, ChatID: chat.ID, Enabled: true}); err != nil {
				return fmt.Errorf("add folder chat to source group: %w", err)
			}
		}
	}
	return nil
}

func (s *SyncService) ListFolders(ctx context.Context, telegramUserID int64) ([]domain.TelegramFolder, error) {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	return s.folders.ListByUserID(ctx, user.ID)
}

func (s *SyncService) ListChats(ctx context.Context, telegramUserID int64) ([]domain.TelegramChat, error) {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	return s.chats.ListByUserID(ctx, user.ID)
}

func (s *SyncService) readyClient(ctx context.Context, telegramUserID int64) (*domain.User, *domain.TelegramSession, TelegramClient, error) {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, nil, nil, err
	}
	if user == nil {
		return nil, nil, nil, errors.New("telegram session is not started")
	}
	session, err := s.sessions.FindByUserID(ctx, user.ID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("find telegram session: %w", err)
	}
	if session == nil || session.Status != domain.SessionStatusConnected {
		return nil, nil, nil, errors.New("telegram session is not connected")
	}
	client, err := s.factory.NewClient(session.StoragePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create tdlib client: %w", err)
	}
	authState, err := client.AuthorizationState(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get tdlib authorization state: %w", err)
	}
	if authState != AuthorizationStateReady {
		return nil, nil, nil, fmt.Errorf("telegram authorization is not ready: %s", authState)
	}
	return user, session, client, nil
}

func (s *SyncService) ownerUser(ctx context.Context, telegramUserID int64) (*domain.User, error) {
	if s.ownerTelegramID == 0 || telegramUserID != s.ownerTelegramID {
		return nil, ErrUnauthorizedOwner
	}
	user, err := s.users.FindByTelegramID(ctx, telegramUserID)
	if err != nil {
		return nil, fmt.Errorf("find owner user: %w", err)
	}
	return user, nil
}

func normalizeChats(userID int64, chats []domain.TelegramChat, archived bool) []domain.TelegramChat {
	normalized := make([]domain.TelegramChat, 0, len(chats))
	for _, chat := range chats {
		chat.UserID = userID
		chat.IsArchived = archived
		normalized = append(normalized, chat)
	}
	return normalized
}

func filterPersonalChats(chats []domain.TelegramChat) []domain.TelegramChat {
	filtered := make([]domain.TelegramChat, 0, len(chats))
	for _, chat := range chats {
		if chat.Type == domain.ChatTypePrivate {
			continue
		}
		filtered = append(filtered, chat)
	}
	return filtered
}

func normalizeGroupName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
