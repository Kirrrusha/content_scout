package summary

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/storage"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

var ErrSummaryNotFound = errors.New("summary not found")
var ErrTopicNotFound = errors.New("summary topic not found")

type Browser struct {
	ownerTelegramID int64
	users           storage.UserRepository
	summaries       storage.SummaryRepository
}

type TopicCard struct {
	Summary domain.Summary
	Topic   domain.SummaryTopic
	Index   int
	Total   int
}

func NewBrowser(ownerTelegramID int64, users storage.UserRepository, summaries storage.SummaryRepository) *Browser {
	return &Browser{ownerTelegramID: ownerTelegramID, users: users, summaries: summaries}
}

func (b *Browser) ListSummaries(ctx context.Context, telegramUserID int64, limit int) ([]domain.Summary, error) {
	user, err := b.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	return b.summaries.ListSummariesByUser(ctx, user.ID, limit)
}

func (b *Browser) GetSummary(ctx context.Context, telegramUserID, summaryID int64) (*domain.Summary, error) {
	user, err := b.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	summary, err := b.summaries.FindSummaryByUser(ctx, user.ID, summaryID)
	if err != nil {
		return nil, err
	}
	if summary == nil {
		return nil, ErrSummaryNotFound
	}
	return summary, nil
}

func (b *Browser) ListTopics(ctx context.Context, telegramUserID, summaryID int64) ([]domain.SummaryTopic, error) {
	if _, err := b.GetSummary(ctx, telegramUserID, summaryID); err != nil {
		return nil, err
	}
	topics, err := b.summaries.ListTopics(ctx, summaryID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(topics, func(i, j int) bool {
		return topics[i].Position < topics[j].Position
	})
	return topics, nil
}

func (b *Browser) TopicCard(ctx context.Context, telegramUserID, summaryID int64, position int) (*TopicCard, error) {
	summary, err := b.GetSummary(ctx, telegramUserID, summaryID)
	if err != nil {
		return nil, err
	}
	topics, err := b.summaries.ListTopics(ctx, summaryID)
	if err != nil {
		return nil, err
	}
	if len(topics) == 0 {
		return nil, ErrTopicNotFound
	}
	sort.SliceStable(topics, func(i, j int) bool {
		return topics[i].Position < topics[j].Position
	})
	if position <= 0 {
		position = 1
	}
	if position > len(topics) {
		position = len(topics)
	}
	return &TopicCard{Summary: *summary, Topic: topics[position-1], Index: position, Total: len(topics)}, nil
}

func (b *Browser) ownerUser(ctx context.Context, telegramUserID int64) (*domain.User, error) {
	if b.ownerTelegramID == 0 || telegramUserID != b.ownerTelegramID {
		return nil, tdlib.ErrUnauthorizedOwner
	}
	user, err := b.users.FindByTelegramID(ctx, telegramUserID)
	if err != nil {
		return nil, fmt.Errorf("find owner user: %w", err)
	}
	if user == nil {
		return nil, errors.New("owner user is not initialized")
	}
	return user, nil
}
