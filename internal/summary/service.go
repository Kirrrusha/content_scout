package summary

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
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
	positions       storage.ReadPositionRepository
	readMarker      TelegramReadMarker
	pipeline        *pipeline.Pipeline
	summarizer      llm.Summarizer
	now             func() time.Time
}

type TelegramReadMarker interface {
	MarkCollectedMessagesRead(ctx context.Context, telegramUserID int64, messages []domain.CollectedMessage) error
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

func NewService(ownerTelegramID int64, users storage.UserRepository, collections storage.MessageCollectionRepository, summaries storage.SummaryRepository, chats storage.TelegramChatRepository, positions storage.ReadPositionRepository, summarizer llm.Summarizer) *Service {
	return &Service{
		ownerTelegramID: ownerTelegramID,
		users:           users,
		collections:     collections,
		summaries:       summaries,
		chats:           chats,
		positions:       positions,
		pipeline:        pipeline.New(),
		summarizer:      summarizer,
		now:             time.Now,
	}
}

func (s *Service) SetTelegramReadMarker(readMarker TelegramReadMarker) {
	s.readMarker = readMarker
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

	chatByID := s.chatsByID(ctx, user.ID)
	input := summaryInput(processed, req.Format, chatByID)
	llmResult, err := s.summarizer.Summarize(ctx, input)
	if err != nil {
		message := err.Error()
		_ = s.summaries.UpdateJobStatus(ctx, summaryJob.ID, domain.JobStatusFailed, &message)
		return nil, err
	}
	topics := topicsFromResult(llmResult, processed, chatByID)
	saved, err := s.summaries.CreateSummary(ctx, domain.Summary{
		JobID:         summaryJob.ID,
		Title:         llmResult.Title,
		Overview:      llmResult.Overview,
		MessagesCount: processed.Stats.KeptMessages,
		SourcesCount:  distinctChatCount(messages),
		TopicsCount:   len(topics),
		Markdown:      renderMarkdown(llmResult, processed, chatByID),
	}, topics)
	if err != nil {
		message := err.Error()
		_ = s.summaries.UpdateJobStatus(ctx, summaryJob.ID, domain.JobStatusFailed, &message)
		return nil, err
	}
	if err := s.summaries.UpdateJobStatus(ctx, summaryJob.ID, domain.JobStatusCompleted, nil); err != nil {
		return nil, err
	}
	// Read markers are intentionally best-effort and happen only after the summary
	// is fully saved. A read-marker failure must not turn a persisted summary into
	// a failed generation or hide it from the user.
	_ = s.markReadPositions(ctx, user.ID, messages)
	_ = s.markTelegramMessagesRead(ctx, req.TelegramUserID, messages)
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

func summaryInput(processed *pipeline.Result, format string, chatByID map[int64]domain.TelegramChat) llm.SummaryInput {
	if format == "" {
		format = "standard"
	}
	inputMessages := make([]llm.SummaryMessageInput, 0, len(processed.Clusters))
	for i, cluster := range processed.Clusters {
		message := cluster.Canonical
		inputMessages = append(inputMessages, llm.SummaryMessageInput{
			Index:       i,
			ChatTitle:   chatTitle(chatByID[message.Source.ChatID]),
			PublishedAt: message.Source.Date.Format(time.RFC3339),
			Text:        message.Content,
			URLs:        message.URLs,
		})
	}
	return llm.SummaryInput{Language: "ru", Format: format, Messages: inputMessages}
}

func (s *Service) chatsByID(ctx context.Context, userID int64) map[int64]domain.TelegramChat {
	byID := make(map[int64]domain.TelegramChat)
	userChats, err := s.chats.ListByUserID(ctx, userID)
	if err != nil {
		return byID
	}
	for _, chat := range userChats {
		byID[chat.ID] = chat
	}
	return byID
}

func (s *Service) markReadPositions(ctx context.Context, userID int64, messages []domain.CollectedMessage) error {
	if s.positions == nil {
		return nil
	}
	maxMessageByChat := make(map[int64]int64)
	for _, message := range messages {
		if message.UserID != userID {
			continue
		}
		if message.MessageID > maxMessageByChat[message.ChatID] {
			maxMessageByChat[message.ChatID] = message.MessageID
		}
	}
	for chatID, messageID := range maxMessageByChat {
		position, err := s.positions.Find(ctx, userID, chatID)
		if err != nil {
			return fmt.Errorf("find read position: %w", err)
		}
		if position != nil && position.LastSummarizedMessageID >= messageID {
			continue
		}
		if err := s.positions.Upsert(ctx, domain.ReadPosition{
			UserID:                  userID,
			ChatID:                  chatID,
			LastSummarizedMessageID: messageID,
		}); err != nil {
			return fmt.Errorf("mark read position: %w", err)
		}
	}
	return nil
}

func (s *Service) markTelegramMessagesRead(ctx context.Context, telegramUserID int64, messages []domain.CollectedMessage) error {
	if s.readMarker == nil {
		return nil
	}
	if err := s.readMarker.MarkCollectedMessagesRead(ctx, telegramUserID, messages); err != nil {
		return fmt.Errorf("mark telegram messages read: %w", err)
	}
	return nil
}

func topicsFromResult(result *llm.SummaryResult, processed *pipeline.Result, chatByID map[int64]domain.TelegramChat) []domain.SummaryTopic {
	topics := make([]domain.SummaryTopic, 0, len(result.Topics))
	for i, topic := range result.Topics {
		sources := topicSources(topic.SourceIndexes, processed, chatByID)
		messages := topicMessages(topic.SourceIndexes, processed, chatByID)
		topics = append(topics, domain.SummaryTopic{
			Title:         topic.Title,
			ShortSummary:  topic.ShortSummary,
			FullSummary:   topic.FullSummary,
			Category:      topic.Category,
			Importance:    topic.Importance,
			Confidence:    confidence(topic.Confidence),
			MessagesCount: len(messages),
			SourcesCount:  len(sources),
			Position:      i + 1,
			Sources:       sources,
			Messages:      messages,
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

func topicSources(sourceIndexes []int, processed *pipeline.Result, chatByID map[int64]domain.TelegramChat) []domain.SummaryTopicSource {
	sources := make([]domain.SummaryTopicSource, 0, len(sourceIndexes))
	seen := make(map[int64]struct{}, len(sourceIndexes))
	for _, index := range sourceIndexes {
		if index < 0 || index >= len(processed.Clusters) {
			continue
		}
		chatID := processed.Clusters[index].Canonical.Source.ChatID
		if _, ok := seen[chatID]; ok {
			continue
		}
		chat, ok := chatByID[chatID]
		if !ok {
			continue
		}
		seen[chatID] = struct{}{}
		sources = append(sources, domain.SummaryTopicSource{
			ChatID:         chat.ID,
			TelegramChatID: chat.TelegramChatID,
			Title:          chat.Title,
			Username:       chat.Username,
		})
	}
	return sources
}

func topicMessages(sourceIndexes []int, processed *pipeline.Result, chatByID map[int64]domain.TelegramChat) []domain.SummaryTopicMessage {
	messages := make([]domain.SummaryTopicMessage, 0, len(sourceIndexes))
	seen := make(map[int64]struct{}, len(sourceIndexes))
	for _, index := range sourceIndexes {
		if index < 0 || index >= len(processed.Clusters) {
			continue
		}
		cluster := processed.Clusters[index]
		canonicalID := cluster.Canonical.Source.ID
		for _, message := range cluster.Messages {
			collectedID := message.Source.ID
			if collectedID == 0 {
				continue
			}
			if _, ok := seen[collectedID]; ok {
				continue
			}
			seen[collectedID] = struct{}{}
			chat := chatByID[message.Source.ChatID]
			messages = append(messages, domain.SummaryTopicMessage{
				CollectedMessageID: collectedID,
				ChatID:             message.Source.ChatID,
				TelegramChatID:     message.Source.TelegramChatID,
				MessageID:          message.Source.MessageID,
				SourceTitle:        chatTitle(chat),
				SourceURL:          telegramMessageURL(message.Source.TelegramChatID, message.Source.MessageID, stringValue(chat.Username), message.Source.URL),
				ClusterIndex:       index,
				IsCanonical:        collectedID == canonicalID,
			})
		}
	}
	return messages
}

func chatTitle(chat domain.TelegramChat) string {
	if strings.TrimSpace(chat.Title) == "" {
		return "Без названия"
	}
	return chat.Title
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func telegramMessageURL(telegramChatID, messageID int64, username, fallback string) string {
	if strings.TrimSpace(username) != "" {
		return fmt.Sprintf("https://t.me/%s/%d", strings.TrimPrefix(strings.TrimSpace(username), "@"), messageID)
	}
	chatID := strconv.FormatInt(telegramChatID, 10)
	if strings.HasPrefix(chatID, "-100") {
		return fmt.Sprintf("https://t.me/c/%s/%d", strings.TrimPrefix(chatID, "-100"), messageID)
	}
	return strings.TrimSpace(fallback)
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

func renderMarkdown(result *llm.SummaryResult, processed *pipeline.Result, chatByID map[int64]domain.TelegramChat) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s\n\n%s\n", result.Title, result.Overview)
	for _, topic := range result.Topics {
		fmt.Fprintf(&builder, "\n## %s\n\n%s\n\n%s\n", topic.Title, topic.ShortSummary, topic.FullSummary)
		if topic.WhyImportant != "" {
			fmt.Fprintf(&builder, "\n**Почему важно:** %s\n", topic.WhyImportant)
		}
		if links := topicMessageLinks(topic.SourceIndexes, processed, chatByID); len(links) > 0 {
			builder.WriteString("\n**Сообщения:**\n")
			for _, link := range links {
				fmt.Fprintf(&builder, "- [%s](%s)\n", link.title, link.url)
			}
		}
	}
	return builder.String()
}

type topicMessageLink struct {
	title string
	url   string
}

func topicMessageLinks(sourceIndexes []int, processed *pipeline.Result, chatByID map[int64]domain.TelegramChat) []topicMessageLink {
	const limit = 8
	links := make([]topicMessageLink, 0, min(len(sourceIndexes), limit))
	seen := make(map[string]struct{}, len(sourceIndexes))
	for _, index := range sourceIndexes {
		if index < 0 || index >= len(processed.Clusters) {
			continue
		}
		for _, message := range processed.Clusters[index].Messages {
			chat := chatByID[message.Source.ChatID]
			url := telegramMessageURL(message.Source.TelegramChatID, message.Source.MessageID, stringValue(chat.Username), message.Source.URL)
			if url == "" {
				continue
			}
			key := fmt.Sprintf("%d:%d", message.Source.TelegramChatID, message.Source.MessageID)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			links = append(links, topicMessageLink{title: chatTitle(chat), url: url})
			if len(links) >= limit {
				return links
			}
		}
	}
	return links
}

func distinctChatCount(messages []domain.CollectedMessage) int {
	seen := make(map[int64]struct{})
	for _, message := range messages {
		seen[message.ChatID] = struct{}{}
	}
	return len(seen)
}
