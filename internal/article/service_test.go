package article

import (
	"context"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/summary/llm"
)

func TestConvertTopicCreatesDraftArticle(t *testing.T) {
	ctx := context.Background()
	users := &fakeUsers{user: &domain.User{ID: 1, TelegramUserID: 42}}
	summaries := &fakeSummaries{
		summary: &domain.Summary{ID: 10, JobID: 50, Title: "Digest", Markdown: "# Digest"},
		job:     &domain.SummaryJob{ID: 50, UserID: 1, SourceType: domain.SummarySourceTypeCollection, SourceID: 70},
		topics:  []domain.SummaryTopic{{SummaryID: 10, Title: "Go", ShortSummary: "Short", FullSummary: "Full", Category: "Tech", Position: 1}},
	}
	collections := &fakeCollections{messages: []domain.CollectedMessage{{
		JobID:          70,
		UserID:         1,
		ChatID:         5,
		TelegramChatID: -100123,
		MessageID:      900,
		Date:           time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
		Text:           "Go release notes",
	}}}
	username := "golang_digest"
	chats := &fakeChats{chats: []domain.TelegramChat{{ID: 5, UserID: 1, TelegramChatID: -100123, Title: "Golang Digest", Username: &username}}}
	articles := &fakeArticles{}
	service := NewService(42, users, summaries, collections, chats, articles, fakeConverter{})

	result, err := service.ConvertTopic(ctx, ConvertRequest{TelegramUserID: 42, SummaryID: 10, TopicPosition: 1, Type: domain.ArticleTypeGuide, Tags: []string{"Go", "Article"}})
	if err != nil {
		t.Fatalf("ConvertTopic() error = %v", err)
	}
	if result.Article.Status != domain.ArticleStatusDraft || result.Article.Type != domain.ArticleTypeGuide {
		t.Fatalf("article = %+v", result.Article)
	}
	if result.Article.Slug != "go-guide" {
		t.Fatalf("slug = %q, want go-guide", result.Article.Slug)
	}
	if len(result.Article.Tags) != 3 || result.Article.Tags[0] != "go" {
		t.Fatalf("tags = %+v", result.Article.Tags)
	}
	if result.Sources != 1 || articles.sources[0].SourceURL != "https://t.me/golang_digest/900" {
		t.Fatalf("sources = %+v", articles.sources)
	}
	if articles.created.ContentMarkdown == "" {
		t.Fatal("content markdown is empty")
	}
}

func TestUpdateMetadataKeepsContentAndStatus(t *testing.T) {
	ctx := context.Background()
	users := &fakeUsers{user: &domain.User{ID: 1, TelegramUserID: 42}}
	articles := &fakeArticles{found: &domain.Article{ID: 7, UserID: 1, Title: "Old", Slug: "old", Type: domain.ArticleTypeAnalysis, Status: domain.ArticleStatusDraft, Tags: []string{"old"}, ContentMarkdown: "# Old"}}
	service := NewService(42, users, nil, nil, nil, articles, nil)

	updated, err := service.UpdateMetadata(ctx, 42, 7, "New title", []string{"AI", "#Go"})
	if err != nil {
		t.Fatalf("UpdateMetadata() error = %v", err)
	}
	if updated.Title != "New title" || updated.ContentMarkdown != "# Old" || updated.Status != domain.ArticleStatusDraft {
		t.Fatalf("updated = %+v", updated)
	}
	if len(updated.Tags) != 2 || updated.Tags[0] != "ai" || updated.Tags[1] != "go" {
		t.Fatalf("tags = %+v", updated.Tags)
	}
}

type fakeConverter struct{}

func (fakeConverter) Summarize(context.Context, llm.SummaryInput) (*llm.SummaryResult, error) {
	return nil, nil
}

func (fakeConverter) ConvertToArticle(context.Context, llm.ArticleInput) (*llm.ArticleResult, error) {
	return &llm.ArticleResult{Title: "Go Guide", Type: "guide", Tags: []string{"Backend"}, ContentMarkdown: "# Go Guide\n\n## Источники\n- https://t.me/golang_digest/900"}, nil
}

type fakeUsers struct{ user *domain.User }

func (f *fakeUsers) UpsertByTelegramID(context.Context, int64) (*domain.User, error) {
	return f.user, nil
}
func (f *fakeUsers) FindByTelegramID(_ context.Context, telegramUserID int64) (*domain.User, error) {
	if f.user != nil && f.user.TelegramUserID == telegramUserID {
		return f.user, nil
	}
	return nil, nil
}

type fakeSummaries struct {
	summary *domain.Summary
	job     *domain.SummaryJob
	topics  []domain.SummaryTopic
}

func (f *fakeSummaries) CreateJob(context.Context, domain.SummaryJob) (*domain.SummaryJob, error) {
	return nil, nil
}
func (f *fakeSummaries) FindJob(context.Context, int64) (*domain.SummaryJob, error) {
	return f.job, nil
}
func (f *fakeSummaries) UpdateJobStatus(context.Context, int64, domain.JobStatus, *string) error {
	return nil
}
func (f *fakeSummaries) CreateSummary(context.Context, domain.Summary, []domain.SummaryTopic) (*domain.Summary, error) {
	return nil, nil
}
func (f *fakeSummaries) FindSummary(context.Context, int64) (*domain.Summary, error) {
	return f.summary, nil
}
func (f *fakeSummaries) FindSummaryByUser(context.Context, int64, int64) (*domain.Summary, error) {
	return f.summary, nil
}
func (f *fakeSummaries) ListSummariesByUser(context.Context, int64, int) ([]domain.Summary, error) {
	return nil, nil
}
func (f *fakeSummaries) ListTopics(context.Context, int64) ([]domain.SummaryTopic, error) {
	return f.topics, nil
}

type fakeCollections struct {
	messages []domain.CollectedMessage
}

func (f *fakeCollections) CreateJob(context.Context, domain.MessageCollectionJob) (*domain.MessageCollectionJob, error) {
	return nil, nil
}
func (f *fakeCollections) FindJob(context.Context, int64) (*domain.MessageCollectionJob, error) {
	return nil, nil
}
func (f *fakeCollections) UpdateJobStatus(context.Context, int64, domain.JobStatus, *string) error {
	return nil
}
func (f *fakeCollections) AddMessages(context.Context, []domain.CollectedMessage) error { return nil }
func (f *fakeCollections) ListMessages(context.Context, int64) ([]domain.CollectedMessage, error) {
	return f.messages, nil
}

type fakeChats struct {
	chats []domain.TelegramChat
}

func (f *fakeChats) UpsertMany(context.Context, []domain.TelegramChat) error { return nil }
func (f *fakeChats) ListByUserID(context.Context, int64) ([]domain.TelegramChat, error) {
	return f.chats, nil
}
func (f *fakeChats) FindByTelegramChatID(context.Context, int64, int64) (*domain.TelegramChat, error) {
	return nil, nil
}

type fakeArticles struct {
	created domain.Article
	sources []domain.ArticleSource
	found   *domain.Article
}

func (f *fakeArticles) Create(_ context.Context, article domain.Article, sources []domain.ArticleSource) (*domain.Article, error) {
	article.ID = 100
	f.created = article
	f.sources = sources
	return &article, nil
}
func (f *fakeArticles) Find(context.Context, int64) (*domain.Article, error) {
	return f.found, nil
}
func (f *fakeArticles) FindByUser(context.Context, int64, int64) (*domain.Article, error) {
	return f.found, nil
}
func (f *fakeArticles) FindBySlug(context.Context, int64, string) (*domain.Article, error) {
	return nil, nil
}
func (f *fakeArticles) ListByUser(context.Context, int64, int) ([]domain.Article, error) {
	if f.found == nil {
		return nil, nil
	}
	return []domain.Article{*f.found}, nil
}
func (f *fakeArticles) Update(_ context.Context, article domain.Article) (*domain.Article, error) {
	f.found = &article
	return &article, nil
}
