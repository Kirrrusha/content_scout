package summary

import (
	"context"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/summary/llm"
)

func TestGenerateFromCollectionPersistsSummary(t *testing.T) {
	ctx := context.Background()
	users := &fakeUsers{user: &domain.User{ID: 1, TelegramUserID: 42}}
	collections := &fakeCollections{
		job: &domain.MessageCollectionJob{ID: 10, UserID: 1, GroupID: 7, Status: domain.JobStatusCompleted},
		messages: []domain.CollectedMessage{
			{JobID: 10, UserID: 1, ChatID: 5, MessageID: 101, Date: time.Now(), Text: "Go team published a detailed compiler performance update https://example.com/go"},
			{JobID: 10, UserID: 1, ChatID: 5, MessageID: 102, Date: time.Now(), Text: "Repost https://example.com/go"},
		},
	}
	summaries := &fakeSummaries{}
	chats := &fakeChats{chats: []domain.TelegramChat{{ID: 5, UserID: 1, Title: "Backend"}}}
	service := NewService(42, users, collections, summaries, chats, fakeSummarizer{})

	result, err := service.GenerateFromCollection(ctx, GenerateRequest{
		TelegramUserID:  42,
		CollectionJobID: 10,
		Format:          "standard",
	})
	if err != nil {
		t.Fatalf("GenerateFromCollection() error = %v", err)
	}
	if result.SummaryID != 100 || result.TopicsCount != 1 || result.DuplicateCount != 1 {
		t.Fatalf("result = %+v", result)
	}
	if summaries.status != domain.JobStatusCompleted {
		t.Fatalf("summary job status = %s", summaries.status)
	}
	if summaries.summary.MessagesCount != 2 || summaries.summary.SourcesCount != 1 {
		t.Fatalf("summary = %+v", summaries.summary)
	}
}

type fakeSummarizer struct{}

func (fakeSummarizer) Summarize(context.Context, llm.SummaryInput) (*llm.SummaryResult, error) {
	return &llm.SummaryResult{
		Title:    "Digest",
		Overview: "Overview",
		Topics: []llm.SummaryTopicResult{{
			Title:         "Topic",
			Category:      "Go",
			ShortSummary:  "Short",
			FullSummary:   "Full",
			WhyImportant:  "Important",
			Confidence:    "high",
			Importance:    9,
			SourceIndexes: []int{0},
		}},
	}, nil
}

func (fakeSummarizer) ConvertToArticle(context.Context, llm.ArticleInput) (*llm.ArticleResult, error) {
	return nil, nil
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

type fakeCollections struct {
	job      *domain.MessageCollectionJob
	messages []domain.CollectedMessage
}

func (f *fakeCollections) CreateJob(context.Context, domain.MessageCollectionJob) (*domain.MessageCollectionJob, error) {
	return nil, nil
}
func (f *fakeCollections) FindJob(context.Context, int64) (*domain.MessageCollectionJob, error) {
	return f.job, nil
}
func (f *fakeCollections) UpdateJobStatus(context.Context, int64, domain.JobStatus, *string) error {
	return nil
}
func (f *fakeCollections) AddMessages(context.Context, []domain.CollectedMessage) error { return nil }
func (f *fakeCollections) ListMessages(context.Context, int64) ([]domain.CollectedMessage, error) {
	return f.messages, nil
}

type fakeSummaries struct {
	summary    domain.Summary
	found      *domain.Summary
	owned      []domain.Summary
	status     domain.JobStatus
	listUserID int64
	listLimit  int
}

func (f *fakeSummaries) CreateJob(_ context.Context, job domain.SummaryJob) (*domain.SummaryJob, error) {
	job.ID = 50
	return &job, nil
}
func (f *fakeSummaries) FindJob(context.Context, int64) (*domain.SummaryJob, error) { return nil, nil }
func (f *fakeSummaries) UpdateJobStatus(_ context.Context, _ int64, status domain.JobStatus, _ *string) error {
	f.status = status
	return nil
}
func (f *fakeSummaries) CreateSummary(_ context.Context, summary domain.Summary, _ []domain.SummaryTopic) (*domain.Summary, error) {
	summary.ID = 100
	f.summary = summary
	return &summary, nil
}
func (f *fakeSummaries) FindSummary(context.Context, int64) (*domain.Summary, error) {
	return f.found, nil
}
func (f *fakeSummaries) FindSummaryByUser(context.Context, int64, int64) (*domain.Summary, error) {
	return f.found, nil
}
func (f *fakeSummaries) ListSummariesByUser(_ context.Context, userID int64, limit int) ([]domain.Summary, error) {
	f.listUserID = userID
	f.listLimit = limit
	return f.owned, nil
}
func (f *fakeSummaries) ListTopics(context.Context, int64) ([]domain.SummaryTopic, error) {
	return nil, nil
}

type fakeChats struct{ chats []domain.TelegramChat }

func (f *fakeChats) UpsertMany(context.Context, []domain.TelegramChat) error { return nil }
func (f *fakeChats) ListByUserID(_ context.Context, userID int64) ([]domain.TelegramChat, error) {
	var out []domain.TelegramChat
	for _, chat := range f.chats {
		if chat.UserID == userID {
			out = append(out, chat)
		}
	}
	return out, nil
}
func (f *fakeChats) FindByTelegramChatID(context.Context, int64, int64) (*domain.TelegramChat, error) {
	return nil, nil
}
