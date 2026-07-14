package article

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/storage"
	"github.com/kirilllebedenko/content_scout/internal/summary"
	"github.com/kirilllebedenko/content_scout/internal/summary/llm"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

var ErrArticleNotFound = errors.New("article not found")

type Service struct {
	ownerTelegramID int64
	users           storage.UserRepository
	summaries       storage.SummaryRepository
	collections     storage.MessageCollectionRepository
	chats           storage.TelegramChatRepository
	articles        storage.ArticleRepository
	converter       llm.Summarizer
}

type ConvertRequest struct {
	TelegramUserID int64
	SummaryID      int64
	TopicPosition  int
	Type           domain.ArticleType
	Title          string
	Tags           []string
}

type Result struct {
	Article domain.Article
	Sources int
}

func NewService(ownerTelegramID int64, users storage.UserRepository, summaries storage.SummaryRepository, collections storage.MessageCollectionRepository, chats storage.TelegramChatRepository, articles storage.ArticleRepository, converter llm.Summarizer) *Service {
	return &Service{ownerTelegramID: ownerTelegramID, users: users, summaries: summaries, collections: collections, chats: chats, articles: articles, converter: converter}
}

func (s *Service) ConvertSummary(ctx context.Context, req ConvertRequest) (*Result, error) {
	return s.convert(ctx, req, nil)
}

func (s *Service) ConvertTopic(ctx context.Context, req ConvertRequest) (*Result, error) {
	if req.TopicPosition <= 0 {
		return nil, summary.ErrTopicNotFound
	}
	return s.convert(ctx, req, &req.TopicPosition)
}

func (s *Service) List(ctx context.Context, telegramUserID int64, limit int) ([]domain.Article, error) {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	return s.articles.ListByUser(ctx, user.ID, limit)
}

func (s *Service) Get(ctx context.Context, telegramUserID, articleID int64) (*domain.Article, error) {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	article, err := s.articles.FindByUser(ctx, user.ID, articleID)
	if err != nil {
		return nil, err
	}
	if article == nil {
		return nil, ErrArticleNotFound
	}
	return article, nil
}

func (s *Service) UpdateMetadata(ctx context.Context, telegramUserID, articleID int64, title string, tags []string) (*domain.Article, error) {
	article, err := s.Get(ctx, telegramUserID, articleID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(title) != "" {
		article.Title = strings.TrimSpace(title)
	}
	if tags != nil {
		article.Tags = normalizeTags(tags)
	}
	updated, err := s.articles.Update(ctx, *article)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, ErrArticleNotFound
	}
	return updated, nil
}

func (s *Service) convert(ctx context.Context, req ConvertRequest, topicPosition *int) (*Result, error) {
	if s.converter == nil {
		return nil, errors.New("article converter is not configured")
	}
	user, err := s.ownerUser(ctx, req.TelegramUserID)
	if err != nil {
		return nil, err
	}
	item, err := s.summaries.FindSummaryByUser(ctx, user.ID, req.SummaryID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, summary.ErrSummaryNotFound
	}
	topic, err := s.topic(ctx, item.ID, topicPosition)
	if err != nil {
		return nil, err
	}
	messages, sources, err := s.sources(ctx, user.ID, item.JobID)
	if err != nil {
		return nil, err
	}
	messages, sources = topicScopedSources(topic, messages, sources)
	input := articleInput(*item, topic, messages, sources, req)
	converted, err := s.converter.ConvertToArticle(ctx, input)
	if err != nil {
		return nil, err
	}
	articleType := articleType(req.Type, converted.Type)
	title := firstNonEmpty(req.Title, converted.Title, item.Title)
	tags := normalizeTags(append(req.Tags, converted.Tags...))
	slug, err := s.uniqueSlug(ctx, user.ID, title)
	if err != nil {
		return nil, err
	}
	created, err := s.articles.Create(ctx, domain.Article{
		UserID:          user.ID,
		Title:           title,
		Slug:            slug,
		Type:            articleType,
		Status:          domain.ArticleStatusDraft,
		Tags:            tags,
		ContentMarkdown: converted.ContentMarkdown,
	}, sources)
	if err != nil {
		return nil, err
	}
	return &Result{Article: *created, Sources: len(sources)}, nil
}

func (s *Service) topic(ctx context.Context, summaryID int64, position *int) (*domain.SummaryTopic, error) {
	if position == nil {
		return nil, nil
	}
	topics, err := s.summaries.ListTopics(ctx, summaryID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(topics, func(i, j int) bool {
		return topics[i].Position < topics[j].Position
	})
	for _, topic := range topics {
		if topic.Position == *position {
			return &topic, nil
		}
	}
	return nil, summary.ErrTopicNotFound
}

func (s *Service) sources(ctx context.Context, userID, summaryJobID int64) ([]domain.CollectedMessage, []domain.ArticleSource, error) {
	job, err := s.summaries.FindJob(ctx, summaryJobID)
	if err != nil {
		return nil, nil, err
	}
	if job == nil || job.UserID != userID || job.SourceType != domain.SummarySourceTypeCollection {
		return nil, nil, nil
	}
	messages, err := s.collections.ListMessages(ctx, job.SourceID)
	if err != nil {
		return nil, nil, err
	}
	chatTitles, chatUsernames := s.chatMetadata(ctx, userID)
	sources := make([]domain.ArticleSource, 0, len(messages))
	seen := make(map[string]struct{})
	for _, message := range messages {
		key := fmt.Sprintf("%d:%d", message.TelegramChatID, message.MessageID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		source := domain.ArticleSource{
			TelegramChatID: message.TelegramChatID,
			MessageID:      message.MessageID,
			SourceTitle:    firstNonEmpty(chatTitles[message.ChatID], message.SenderName, "Telegram"),
			SourceURL:      telegramMessageURL(message.TelegramChatID, message.MessageID, chatUsernames[message.ChatID], message.URL),
			PublishedAt:    message.Date,
		}
		sources = append(sources, source)
	}
	return messages, sources, nil
}

func topicScopedSources(topic *domain.SummaryTopic, messages []domain.CollectedMessage, sources []domain.ArticleSource) ([]domain.CollectedMessage, []domain.ArticleSource) {
	if topic == nil {
		return messages, sources
	}
	if len(topic.Messages) > 0 {
		messagesByKey := make(map[string]domain.CollectedMessage, len(messages))
		sourcesByKey := make(map[string]domain.ArticleSource, len(sources))
		for _, message := range messages {
			messagesByKey[messageKey(message.TelegramChatID, message.MessageID)] = message
		}
		for _, source := range sources {
			sourcesByKey[messageKey(source.TelegramChatID, source.MessageID)] = source
		}
		scopedMessages := make([]domain.CollectedMessage, 0, len(topic.Messages))
		scopedSources := make([]domain.ArticleSource, 0, len(topic.Messages))
		seen := make(map[string]struct{}, len(topic.Messages))
		for _, topicMessage := range topic.Messages {
			key := messageKey(topicMessage.TelegramChatID, topicMessage.MessageID)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			message, hasMessage := messagesByKey[key]
			source, hasSource := sourcesByKey[key]
			if !hasSource {
				source = domain.ArticleSource{
					TelegramChatID: topicMessage.TelegramChatID,
					MessageID:      topicMessage.MessageID,
					SourceTitle:    firstNonEmpty(topicMessage.SourceTitle, "Telegram"),
					SourceURL:      topicMessage.SourceURL,
				}
			}
			if hasMessage {
				scopedMessages = append(scopedMessages, message)
				if source.PublishedAt.IsZero() {
					source.PublishedAt = message.Date
				}
			}
			scopedSources = append(scopedSources, source)
		}
		return scopedMessages, scopedSources
	}
	if len(topic.Sources) == 0 {
		return messages, sources
	}
	topicChats := make(map[int64]struct{}, len(topic.Sources))
	for _, source := range topic.Sources {
		topicChats[source.TelegramChatID] = struct{}{}
	}
	scopedMessages := make([]domain.CollectedMessage, 0, len(messages))
	for _, message := range messages {
		if _, ok := topicChats[message.TelegramChatID]; ok {
			scopedMessages = append(scopedMessages, message)
		}
	}
	scopedSources := make([]domain.ArticleSource, 0, len(sources))
	for _, source := range sources {
		if _, ok := topicChats[source.TelegramChatID]; ok {
			scopedSources = append(scopedSources, source)
		}
	}
	return scopedMessages, scopedSources
}

func (s *Service) chatMetadata(ctx context.Context, userID int64) (map[int64]string, map[int64]string) {
	titles := make(map[int64]string)
	usernames := make(map[int64]string)
	chats, err := s.chats.ListByUserID(ctx, userID)
	if err != nil {
		return titles, usernames
	}
	for _, chat := range chats {
		titles[chat.ID] = chat.Title
		if chat.Username != nil {
			usernames[chat.ID] = *chat.Username
		}
	}
	return titles, usernames
}

func (s *Service) uniqueSlug(ctx context.Context, userID int64, title string) (string, error) {
	base := slugify(title)
	for i := 0; i < 100; i++ {
		candidate := base
		if i > 0 {
			candidate = base + "-" + strconv.Itoa(i+1)
		}
		found, err := s.articles.FindBySlug(ctx, userID, candidate)
		if err != nil {
			return "", err
		}
		if found == nil {
			return candidate, nil
		}
	}
	return base + "-" + strconv.FormatInt(time.Now().Unix(), 10), nil
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

func articleInput(item domain.Summary, topic *domain.SummaryTopic, messages []domain.CollectedMessage, sources []domain.ArticleSource, req ConvertRequest) llm.ArticleInput {
	input := llm.ArticleInput{
		Language:   "ru",
		Type:       string(articleType(req.Type, "")),
		Title:      req.Title,
		Tags:       normalizeTags(req.Tags),
		SourceKind: "summary",
		Summary:    item.Markdown,
		Sources:    make([]llm.ArticleSourceInput, 0, len(sources)),
	}
	if topic != nil {
		input.SourceKind = "summary_topic"
		input.Topic = &llm.ArticleTopicInput{Title: topic.Title, Category: topic.Category, ShortSummary: topic.ShortSummary, FullSummary: topic.FullSummary}
	}
	messageByKey := make(map[string]domain.CollectedMessage, len(messages))
	for _, message := range messages {
		messageByKey[messageKey(message.TelegramChatID, message.MessageID)] = message
	}
	for _, source := range sources {
		message := messageByKey[messageKey(source.TelegramChatID, source.MessageID)]
		input.Sources = append(input.Sources, llm.ArticleSourceInput{
			Title:       source.SourceTitle,
			URL:         source.SourceURL,
			PublishedAt: source.PublishedAt.Format(time.RFC3339),
			Text:        strings.TrimSpace(message.Text + "\n" + message.Caption),
		})
	}
	return input
}

func messageKey(telegramChatID, messageID int64) string {
	return fmt.Sprintf("%d:%d", telegramChatID, messageID)
}

func articleType(preferred domain.ArticleType, fallback string) domain.ArticleType {
	switch preferred {
	case domain.ArticleTypeEducational, domain.ArticleTypeGuide, domain.ArticleTypeAnalysis, domain.ArticleTypeOutline, domain.ArticleTypeTelegram:
		return preferred
	}
	switch fallback {
	case string(domain.ArticleTypeEducational):
		return domain.ArticleTypeEducational
	case string(domain.ArticleTypeGuide):
		return domain.ArticleTypeGuide
	case string(domain.ArticleTypeOutline):
		return domain.ArticleTypeOutline
	case string(domain.ArticleTypeTelegram):
		return domain.ArticleTypeTelegram
	default:
		return domain.ArticleTypeAnalysis
	}
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.TrimPrefix(tag, "#"))
		tag = strings.ToLower(tag)
		tag = regexp.MustCompile(`[^a-z0-9а-яё_-]+`).ReplaceAllString(tag, "-")
		tag = strings.Trim(tag, "-_")
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9а-яё]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "article"
	}
	return value
}

func telegramMessageURL(telegramChatID, messageID int64, username, fallback string) string {
	if username != "" {
		return fmt.Sprintf("https://t.me/%s/%d", strings.TrimPrefix(username, "@"), messageID)
	}
	if strings.HasPrefix(strconv.FormatInt(telegramChatID, 10), "-100") {
		return fmt.Sprintf("https://t.me/c/%s/%d", strings.TrimPrefix(strconv.FormatInt(telegramChatID, 10), "-100"), messageID)
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
