package collection

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/sourcegroups"
	"github.com/kirilllebedenko/content_scout/internal/storage"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

type Request struct {
	TelegramUserID int64
	GroupID        int64
	Mode           domain.CollectionMode
	Limit          int
}

type Result struct {
	JobID         int64
	UserID        int64
	GroupID       int64
	ChatsCount    int
	MessagesCount int
}

type Service struct {
	ownerTelegramID int64
	users           storage.UserRepository
	sessions        storage.TelegramSessionRepository
	groups          storage.SourceGroupRepository
	chats           storage.TelegramChatRepository
	positions       storage.ReadPositionRepository
	collections     storage.MessageCollectionRepository
	factory         tdlib.ClientFactory
	now             func() time.Time
}

func NewService(ownerTelegramID int64, users storage.UserRepository, sessions storage.TelegramSessionRepository, groups storage.SourceGroupRepository, chats storage.TelegramChatRepository, positions storage.ReadPositionRepository, collections storage.MessageCollectionRepository, factory tdlib.ClientFactory) *Service {
	return &Service{
		ownerTelegramID: ownerTelegramID,
		users:           users,
		sessions:        sessions,
		groups:          groups,
		chats:           chats,
		positions:       positions,
		collections:     collections,
		factory:         factory,
		now:             time.Now,
	}
}

func (s *Service) CollectGroup(ctx context.Context, req Request) (*Result, error) {
	if req.Mode == "" {
		req.Mode = domain.CollectionModeNewOnly
	}
	if req.Limit <= 0 {
		req.Limit = 100
	}
	user, client, err := s.readyClient(ctx, req.TelegramUserID)
	if err != nil {
		return nil, err
	}
	groupLinks, err := s.groups.ListChats(ctx, req.GroupID)
	if err != nil {
		return nil, fmt.Errorf("list source group chats: %w", err)
	}
	if len(groupLinks) == 0 {
		return nil, sourcegroups.ErrGroupNotFound
	}
	chatMap, err := s.userChatMap(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	job, err := s.collections.CreateJob(ctx, domain.MessageCollectionJob{
		UserID:  user.ID,
		GroupID: req.GroupID,
		Mode:    req.Mode,
		Limit:   req.Limit,
		Status:  domain.JobStatusCollecting,
	})
	if err != nil {
		return nil, fmt.Errorf("create message collection job: %w", err)
	}

	collected, chatsCount, collectErr := s.collectMessages(ctx, user.ID, job.ID, req, groupLinks, chatMap, client)
	if collectErr != nil {
		message := collectErr.Error()
		_ = s.collections.UpdateJobStatus(ctx, job.ID, domain.JobStatusFailed, &message)
		return nil, collectErr
	}
	if err := s.collections.AddMessages(ctx, collected); err != nil {
		message := err.Error()
		_ = s.collections.UpdateJobStatus(ctx, job.ID, domain.JobStatusFailed, &message)
		return nil, err
	}
	if err := s.collections.UpdateJobStatus(ctx, job.ID, domain.JobStatusCompleted, nil); err != nil {
		return nil, err
	}
	return &Result{
		JobID:         job.ID,
		UserID:        user.ID,
		GroupID:       req.GroupID,
		ChatsCount:    chatsCount,
		MessagesCount: len(collected),
	}, nil
}

func (s *Service) collectMessages(ctx context.Context, userID, jobID int64, req Request, links []domain.SourceGroupChat, chatMap map[int64]domain.TelegramChat, client tdlib.TelegramClient) ([]domain.CollectedMessage, int, error) {
	var collected []domain.CollectedMessage
	chatsCount := 0
	since := sinceForMode(req.Mode, s.now())
	for _, link := range links {
		if !link.Enabled {
			continue
		}
		chat, ok := chatMap[link.ChatID]
		if !ok {
			continue
		}
		if chat.Type == domain.ChatTypePrivate {
			continue
		}
		chatsCount++
		position, err := s.positions.Find(ctx, userID, chat.ID)
		if err != nil {
			return nil, 0, fmt.Errorf("find read position: %w", err)
		}
		fromMessageID := int64(0)
		if position != nil && req.Mode == domain.CollectionModeNewOnly {
			fromMessageID = position.LastSummarizedMessageID
		}
		messages, err := client.GetChatHistory(ctx, chat.TelegramChatID, fromMessageID, req.Limit)
		if err != nil {
			return nil, 0, fmt.Errorf("get chat history: %w", err)
		}
		for _, message := range messages {
			if shouldSkipMessage(message, position, since, req.Mode) {
				continue
			}
			collected = append(collected, collectedMessage(userID, jobID, chat, message))
		}
	}
	return collected, chatsCount, nil
}

func (s *Service) readyClient(ctx context.Context, telegramUserID int64) (*domain.User, tdlib.TelegramClient, error) {
	if s.ownerTelegramID == 0 || telegramUserID != s.ownerTelegramID {
		return nil, nil, tdlib.ErrUnauthorizedOwner
	}
	user, err := s.users.FindByTelegramID(ctx, telegramUserID)
	if err != nil {
		return nil, nil, fmt.Errorf("find owner user: %w", err)
	}
	if user == nil {
		return nil, nil, errors.New("owner user is not initialized")
	}
	session, err := s.sessions.FindByUserID(ctx, user.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("find telegram session: %w", err)
	}
	if session == nil || session.Status != domain.SessionStatusConnected {
		return nil, nil, errors.New("telegram session is not connected")
	}
	client, err := s.factory.NewClient(session.StoragePath)
	if err != nil {
		return nil, nil, fmt.Errorf("create tdlib client: %w", err)
	}
	authState, err := client.AuthorizationState(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("get tdlib authorization state: %w", err)
	}
	if authState != tdlib.AuthorizationStateReady {
		return nil, nil, fmt.Errorf("telegram authorization is not ready: %s", authState)
	}
	return user, client, nil
}

func (s *Service) userChatMap(ctx context.Context, userID int64) (map[int64]domain.TelegramChat, error) {
	chats, err := s.chats.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user chats: %w", err)
	}
	chatMap := make(map[int64]domain.TelegramChat, len(chats))
	for _, chat := range chats {
		chatMap[chat.ID] = chat
	}
	return chatMap, nil
}

func sinceForMode(mode domain.CollectionMode, now time.Time) *time.Time {
	var since time.Time
	switch mode {
	case domain.CollectionModeLast24H:
		since = now.Add(-24 * time.Hour)
	case domain.CollectionModeLast3D:
		since = now.Add(-72 * time.Hour)
	case domain.CollectionModeWeek:
		since = now.AddDate(0, 0, -7)
	default:
		return nil
	}
	return &since
}

func shouldSkipMessage(message domain.TelegramMessage, position *domain.ReadPosition, since *time.Time, mode domain.CollectionMode) bool {
	if strings.TrimSpace(message.Text) == "" && strings.TrimSpace(message.Caption) == "" {
		return true
	}
	if mode == domain.CollectionModeNewOnly && position != nil && message.MessageID <= position.LastSummarizedMessageID {
		return true
	}
	if since != nil && message.Date.Before(*since) {
		return true
	}
	return false
}

func collectedMessage(userID, jobID int64, chat domain.TelegramChat, message domain.TelegramMessage) domain.CollectedMessage {
	return domain.CollectedMessage{
		JobID:          jobID,
		UserID:         userID,
		ChatID:         chat.ID,
		TelegramChatID: chat.TelegramChatID,
		MessageID:      message.MessageID,
		Date:           message.Date,
		EditDate:       message.EditDate,
		SenderID:       message.SenderID,
		SenderName:     message.SenderName,
		Text:           message.Text,
		Caption:        message.Caption,
		URL:            message.URL,
		ReplyToID:      message.ReplyToID,
		Forwarded:      message.Forwarded,
		HasMedia:       message.HasMedia,
		MediaType:      message.MediaType,
	}
}
