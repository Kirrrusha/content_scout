package summary

import (
	"context"
	"errors"
	"strings"
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
			{ID: 1001, JobID: 10, UserID: 1, ChatID: 5, MessageID: 101, Date: time.Now(), Text: "Go team published a detailed compiler performance update https://example.com/go"},
			{ID: 1002, JobID: 10, UserID: 1, ChatID: 5, MessageID: 102, Date: time.Now(), Text: "Repost https://example.com/go"},
		},
	}
	summaries := &fakeSummaries{}
	username := "backend"
	chats := &fakeChats{chats: []domain.TelegramChat{{ID: 5, UserID: 1, TelegramChatID: -1005, Title: "Backend", Username: &username}}}
	positions := newFakePositions()
	readMarker := &fakeReadMarker{}
	service := NewService(42, users, collections, summaries, chats, positions, fakeSummarizer{})
	service.SetTelegramReadMarker(readMarker)

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
	if !strings.Contains(summaries.summary.Markdown, "https://t.me/backend/101") {
		t.Fatalf("summary markdown has no message link: %s", summaries.summary.Markdown)
	}
	if len(summaries.topics) != 1 || len(summaries.topics[0].Sources) != 1 {
		t.Fatalf("topic sources = %+v", summaries.topics)
	}
	source := summaries.topics[0].Sources[0]
	if source.Title != "Backend" || source.Username == nil || *source.Username != "backend" {
		t.Fatalf("topic source = %+v", source)
	}
	if len(summaries.topics[0].Messages) != 2 {
		t.Fatalf("topic messages = %+v", summaries.topics[0].Messages)
	}
	if summaries.topics[0].Messages[0].CollectedMessageID != 1001 || !summaries.topics[0].Messages[0].IsCanonical {
		t.Fatalf("first topic message = %+v", summaries.topics[0].Messages[0])
	}
	if summaries.topics[0].Messages[0].SourceURL != "https://t.me/backend/101" || summaries.topics[0].Messages[0].SourceTitle != "Backend" {
		t.Fatalf("first topic message source = %+v", summaries.topics[0].Messages[0])
	}
	if summaries.topics[0].Messages[1].CollectedMessageID != 1002 || summaries.topics[0].Messages[1].IsCanonical {
		t.Fatalf("second topic message = %+v", summaries.topics[0].Messages[1])
	}
	position, err := positions.Find(ctx, 1, 5)
	if err != nil {
		t.Fatalf("position Find() error = %v", err)
	}
	if position == nil || position.LastSummarizedMessageID != 102 {
		t.Fatalf("position = %+v, want message 102", position)
	}
	if readMarker.telegramUserID != 42 || len(readMarker.messages) != 2 {
		t.Fatalf("read marker telegramUserID=%d messages=%+v", readMarker.telegramUserID, readMarker.messages)
	}
}

func TestGenerateFromCollectionDoesNotMoveReadPositionBackwards(t *testing.T) {
	ctx := context.Background()
	users := &fakeUsers{user: &domain.User{ID: 1, TelegramUserID: 42}}
	collections := &fakeCollections{
		job: &domain.MessageCollectionJob{ID: 10, UserID: 1, GroupID: 7, Status: domain.JobStatusCompleted},
		messages: []domain.CollectedMessage{
			{JobID: 10, UserID: 1, ChatID: 5, MessageID: 101, Date: time.Now(), Text: "Go team published a detailed compiler performance update https://example.com/go"},
		},
	}
	summaries := &fakeSummaries{}
	chats := &fakeChats{chats: []domain.TelegramChat{{ID: 5, UserID: 1, TelegramChatID: -1005, Title: "Backend"}}}
	positions := newFakePositions()
	if err := positions.Upsert(ctx, domain.ReadPosition{UserID: 1, ChatID: 5, LastSummarizedMessageID: 200}); err != nil {
		t.Fatalf("position Upsert() error = %v", err)
	}
	service := NewService(42, users, collections, summaries, chats, positions, fakeSummarizer{})

	_, err := service.GenerateFromCollection(ctx, GenerateRequest{
		TelegramUserID:  42,
		CollectionJobID: 10,
		Format:          "standard",
	})
	if err != nil {
		t.Fatalf("GenerateFromCollection() error = %v", err)
	}
	position, err := positions.Find(ctx, 1, 5)
	if err != nil {
		t.Fatalf("position Find() error = %v", err)
	}
	if position.LastSummarizedMessageID != 200 {
		t.Fatalf("position = %+v, want message 200", position)
	}
}

func TestGenerateFromCollectionDoesNotFailSavedSummaryWhenTelegramReadMarkFails(t *testing.T) {
	ctx := context.Background()
	users := &fakeUsers{user: &domain.User{ID: 1, TelegramUserID: 42}}
	collections := &fakeCollections{
		job: &domain.MessageCollectionJob{ID: 10, UserID: 1, GroupID: 7, Status: domain.JobStatusCompleted},
		messages: []domain.CollectedMessage{
			{ID: 1001, JobID: 10, UserID: 1, ChatID: 5, MessageID: 101, Date: time.Now(), Text: "Go team published a detailed compiler performance update https://example.com/go"},
		},
	}
	summaries := &fakeSummaries{}
	chats := &fakeChats{chats: []domain.TelegramChat{{ID: 5, UserID: 1, TelegramChatID: -1005, Title: "Backend"}}}
	positions := newFakePositions()
	readMarker := &fakeReadMarker{err: errors.New("telegram read failed")}
	service := NewService(42, users, collections, summaries, chats, positions, fakeSummarizer{})
	service.SetTelegramReadMarker(readMarker)

	result, err := service.GenerateFromCollection(ctx, GenerateRequest{
		TelegramUserID:  42,
		CollectionJobID: 10,
		Format:          "standard",
	})
	if err != nil {
		t.Fatalf("GenerateFromCollection() error = %v", err)
	}
	if result.SummaryID != 100 || summaries.status != domain.JobStatusCompleted {
		t.Fatalf("result=%+v status=%s", result, summaries.status)
	}
	position, err := positions.Find(ctx, 1, 5)
	if err != nil {
		t.Fatalf("position Find() error = %v", err)
	}
	if position == nil || position.LastSummarizedMessageID != 101 {
		t.Fatalf("position = %+v, want message 101", position)
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
	topics     []domain.SummaryTopic
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
func (f *fakeSummaries) CreateSummary(_ context.Context, summary domain.Summary, topics []domain.SummaryTopic) (*domain.Summary, error) {
	summary.ID = 100
	f.summary = summary
	f.topics = topics
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
func (f *fakeSummaries) DeleteSummariesOlderThan(context.Context, time.Time) (int64, error) {
	return 0, nil
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

type fakePositions struct {
	positions map[[2]int64]domain.ReadPosition
}

func newFakePositions() *fakePositions {
	return &fakePositions{positions: make(map[[2]int64]domain.ReadPosition)}
}

func (f *fakePositions) Upsert(_ context.Context, position domain.ReadPosition) error {
	f.positions[[2]int64{position.UserID, position.ChatID}] = position
	return nil
}

func (f *fakePositions) Find(_ context.Context, userID, chatID int64) (*domain.ReadPosition, error) {
	position, ok := f.positions[[2]int64{userID, chatID}]
	if !ok {
		return nil, nil
	}
	return &position, nil
}

type fakeReadMarker struct {
	telegramUserID int64
	messages       []domain.CollectedMessage
	err            error
}

func (f *fakeReadMarker) MarkCollectedMessagesRead(_ context.Context, telegramUserID int64, messages []domain.CollectedMessage) error {
	if f.err != nil {
		return f.err
	}
	f.telegramUserID = telegramUserID
	f.messages = append([]domain.CollectedMessage(nil), messages...)
	return nil
}
