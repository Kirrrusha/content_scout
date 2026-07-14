package tdlib

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/storage"
)

type ReadService struct {
	ownerTelegramID int64
	users           storage.UserRepository
	sessions        storage.TelegramSessionRepository
	factory         ClientFactory
}

func NewReadService(ownerTelegramID int64, users storage.UserRepository, sessions storage.TelegramSessionRepository, factory ClientFactory) *ReadService {
	return &ReadService{
		ownerTelegramID: ownerTelegramID,
		users:           users,
		sessions:        sessions,
		factory:         factory,
	}
}

func (s *ReadService) MarkCollectedMessagesRead(ctx context.Context, telegramUserID int64, messages []domain.CollectedMessage) error {
	if len(messages) == 0 {
		return nil
	}
	user, client, err := s.readyClient(ctx, telegramUserID)
	if err != nil {
		return err
	}

	messageIDsByChat := make(map[int64]map[int64]struct{})
	for _, message := range messages {
		if message.UserID != user.ID || message.TelegramChatID == 0 || message.MessageID <= 0 {
			continue
		}
		if messageIDsByChat[message.TelegramChatID] == nil {
			messageIDsByChat[message.TelegramChatID] = make(map[int64]struct{})
		}
		messageIDsByChat[message.TelegramChatID][message.MessageID] = struct{}{}
	}
	for chatID, ids := range messageIDsByChat {
		messageIDs := make([]int64, 0, len(ids))
		for messageID := range ids {
			messageIDs = append(messageIDs, messageID)
		}
		sort.Slice(messageIDs, func(i, j int) bool { return messageIDs[i] < messageIDs[j] })
		if err := client.MarkMessagesRead(ctx, chatID, messageIDs); err != nil {
			return fmt.Errorf("mark telegram messages read: %w", err)
		}
	}
	return nil
}

func (s *ReadService) readyClient(ctx context.Context, telegramUserID int64) (*domain.User, TelegramClient, error) {
	if s.ownerTelegramID == 0 || telegramUserID != s.ownerTelegramID {
		return nil, nil, ErrUnauthorizedOwner
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
	if authState != AuthorizationStateReady {
		return nil, nil, fmt.Errorf("telegram authorization is not ready: %s", authState)
	}
	return user, client, nil
}
