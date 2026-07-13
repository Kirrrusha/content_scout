package summary

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/storage"
	"github.com/kirilllebedenko/content_scout/internal/summary/filter"
	"github.com/kirilllebedenko/content_scout/internal/summary/llm"
	"github.com/kirilllebedenko/content_scout/internal/summary/pipeline"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

type Service struct {
	ownerTelegramID int64
	users           storage.UserRepository
	collections     storage.MessageCollectionRepository
	summaries       storage.SummaryRepository
	chats           storage.TelegramChatRepository
	pipeline        *pipeline.Pipeline
	summarizer      llm.Summarizer
	now             func() time.Time
}

type GenerateRequest struct {
	TelegramUserID  int64
	CollectionJobID int64
	Format          string
}

type GenerateResult struct {
	SummaryID      int64
	SummaryJobID   int64
	TopicsCount    int
	MessagesCount  int
	DuplicateCount int
}

func NewService(ownerTelegramID int64, users storage.UserRepository, collections storage.MessageCollectionRepository, summaries storage.SummaryRepository, chats storage.TelegramChatRepository, summarizer llm.Summarizer) *Service {
	return &Service{
		ownerTelegramID: ownerTelegramID,
		users:           users,
		collections:     collections,
		summaries:       summaries,
		chats:           chats,
		pipeline:        pipeline.New(),
		summarizer:      summarizer,
		now:             time.Now,
	}
}

func (s *Service) GenerateFromCollection(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	if s.summarizer == nil {
		return nil, errors.New("llm summarizer is not configured")
	}
	user, err := s.ownerUser(ctx, req.TelegramUserID)
	if err != nil {
		return nil, err
	}
	collectionJob, err := s.collections.FindJob(ctx, req.CollectionJobID)
	if err != nil {
		return nil, err
	}
	if collectionJob == nil || collectionJob.UserID != user.ID {
		return nil, errors.New("collection job not found")
	}
	messages, err := s.collections.ListMessages(ctx, req.CollectionJobID)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, errors.New("collection job has no messages")
	}
	processed, err := s.pipeline.Process(ctx, messages, filter.Rules{MinTextLength: 30, DropAds: true, DropJobs: true})
	if err != nil {
		return nil, err
	}
	if len(processed.Clusters) == 0 {
		return nil, errors.New("no messages left after filtering")
	}

	startedAt := s.now()
	summaryJob, err := s.summaries.CreateJob(ctx, domain.SummaryJob{
		UserID:     user.ID,
		SourceType: domain.SummarySourceTypeCollection,
		SourceID:   req.CollectionJobID,
		Status:     domain.JobStatusProcessing,
		StartedAt:  &startedAt,
	})
	if err != nil {
		return nil, err
	}

	input := s.summaryInput(ctx, user.ID, processed, req.Format)
	llmResult, err := s.summarizer.Summarize(ctx, input)
	if err != nil {
		message := err.Error()
		_ = s.summaries.UpdateJobStatus(ctx, summaryJob.ID, domain.JobStatusFailed, &message)
		return nil, err
	}
	topics := topicsFromResult(llmResult)
	saved, err := s.summaries.CreateSummary(ctx, domain.Summary{
		JobID:         summaryJob.ID,
		Title:         llmResult.Title,
		Overview:      llmResult.Overview,
		MessagesCount: processed.Stats.KeptMessages,
		SourcesCount:  distinctChatCount(messages),
		TopicsCount:   len(topics),
		Markdown:      renderMarkdown(llmResult),
	}, topics)
	if err != nil {
		message := err.Error()
		_ = s.summaries.UpdateJobStatus(ctx, summaryJob.ID, domain.JobStatusFailed, &message)
		return nil, err
	}
	if err := s.summaries.UpdateJobStatus(ctx, summaryJob.ID, domain.JobStatusCompleted, nil); err != nil {
		return nil, err
	}
	return &GenerateResult{
		SummaryID:      saved.ID,
		SummaryJobID:   summaryJob.ID,
		TopicsCount:    len(topics),
		MessagesCount:  processed.Stats.KeptMessages,
		DuplicateCount: processed.Stats.DuplicateRemoved,
	}, nil
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

func (s *Service) summaryInput(ctx context.Context, userID int64, processed *pipeline.Result, format string) llm.SummaryInput {
	if format == "" {
		format = "standard"
	}
	chatTitles := s.chatTitles(ctx, userID)
	inputMessages := make([]llm.SummaryMessageInput, 0, len(processed.Clusters))
	for i, cluster := range processed.Clusters {
		message := cluster.Canonical
		inputMessages = append(inputMessages, llm.SummaryMessageInput{
			Index:       i,
			ChatTitle:   chatTitles[message.Source.ChatID],
			PublishedAt: message.Source.Date.Format(time.RFC3339),
			Text:        message.Content,
			URLs:        message.URLs,
		})
	}
	return llm.SummaryInput{Language: "ru", Format: format, Messages: inputMessages}
}

func (s *Service) chatTitles(ctx context.Context, userID int64) map[int64]string {
	titles := make(map[int64]string)
	userChats, err := s.chats.ListByUserID(ctx, userID)
	if err != nil {
		return titles
	}
	for _, chat := range userChats {
		titles[chat.ID] = chat.Title
	}
	return titles
}

func topicsFromResult(result *llm.SummaryResult) []domain.SummaryTopic {
	topics := make([]domain.SummaryTopic, 0, len(result.Topics))
	for i, topic := range result.Topics {
		topics = append(topics, domain.SummaryTopic{
			Title:         topic.Title,
			ShortSummary:  topic.ShortSummary,
			FullSummary:   topic.FullSummary,
			Category:      topic.Category,
			Importance:    topic.Importance,
			Confidence:    confidence(topic.Confidence),
			MessagesCount: len(topic.SourceIndexes),
			SourcesCount:  len(topic.SourceIndexes),
			Position:      i + 1,
		})
	}
	sort.SliceStable(topics, func(i, j int) bool {
		return topics[i].Importance > topics[j].Importance
	})
	for i := range topics {
		topics[i].Position = i + 1
	}
	return topics
}

func confidence(value string) domain.ConfidenceLevel {
	switch value {
	case "high":
		return domain.ConfidenceHigh
	case "low":
		return domain.ConfidenceLow
	default:
		return domain.ConfidenceMedium
	}
}

func renderMarkdown(result *llm.SummaryResult) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s\n\n%s\n", result.Title, result.Overview)
	for _, topic := range result.Topics {
		fmt.Fprintf(&builder, "\n## %s\n\n%s\n\n%s\n", topic.Title, topic.ShortSummary, topic.FullSummary)
		if topic.WhyImportant != "" {
			fmt.Fprintf(&builder, "\n**Почему важно:** %s\n", topic.WhyImportant)
		}
	}
	return builder.String()
}

func distinctChatCount(messages []domain.CollectedMessage) int {
	seen := make(map[int64]struct{})
	for _, message := range messages {
		seen[message.ChatID] = struct{}{}
	}
	return len(seen)
}
