package storage

import (
	"context"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type UserRepository interface {
	UpsertByTelegramID(ctx context.Context, telegramUserID int64) (*domain.User, error)
	FindByTelegramID(ctx context.Context, telegramUserID int64) (*domain.User, error)
}

type TelegramSessionRepository interface {
	Upsert(ctx context.Context, session domain.TelegramSession) (*domain.TelegramSession, error)
	FindByUserID(ctx context.Context, userID int64) (*domain.TelegramSession, error)
	DeleteByUserID(ctx context.Context, userID int64) error
}

type TelegramFolderRepository interface {
	UpsertMany(ctx context.Context, folders []domain.TelegramFolder) error
	ListByUserID(ctx context.Context, userID int64) ([]domain.TelegramFolder, error)
}

type TelegramChatRepository interface {
	UpsertMany(ctx context.Context, chats []domain.TelegramChat) error
	ListByUserID(ctx context.Context, userID int64) ([]domain.TelegramChat, error)
	FindByTelegramChatID(ctx context.Context, userID, telegramChatID int64) (*domain.TelegramChat, error)
}

type SourceGroupRepository interface {
	Create(ctx context.Context, group domain.SourceGroup) (*domain.SourceGroup, error)
	Update(ctx context.Context, group domain.SourceGroup) (*domain.SourceGroup, error)
	Delete(ctx context.Context, userID, groupID int64) error
	ListByUserID(ctx context.Context, userID int64) ([]domain.SourceGroup, error)
	AddChat(ctx context.Context, link domain.SourceGroupChat) error
	RemoveChat(ctx context.Context, groupID, chatID int64) error
	ListChats(ctx context.Context, groupID int64) ([]domain.SourceGroupChat, error)
}

type ReadPositionRepository interface {
	Upsert(ctx context.Context, position domain.ReadPosition) error
	Find(ctx context.Context, userID, chatID int64) (*domain.ReadPosition, error)
}

type MessageCollectionRepository interface {
	CreateJob(ctx context.Context, job domain.MessageCollectionJob) (*domain.MessageCollectionJob, error)
	FindJob(ctx context.Context, jobID int64) (*domain.MessageCollectionJob, error)
	UpdateJobStatus(ctx context.Context, jobID int64, status domain.JobStatus, message *string) error
	AddMessages(ctx context.Context, messages []domain.CollectedMessage) error
	ListMessages(ctx context.Context, jobID int64) ([]domain.CollectedMessage, error)
}

type SummaryRepository interface {
	CreateJob(ctx context.Context, job domain.SummaryJob) (*domain.SummaryJob, error)
	FindJob(ctx context.Context, jobID int64) (*domain.SummaryJob, error)
	UpdateJobStatus(ctx context.Context, jobID int64, status domain.JobStatus, message *string) error
	CreateSummary(ctx context.Context, summary domain.Summary, topics []domain.SummaryTopic) (*domain.Summary, error)
	FindSummary(ctx context.Context, summaryID int64) (*domain.Summary, error)
	FindSummaryByUser(ctx context.Context, userID, summaryID int64) (*domain.Summary, error)
	ListSummariesByUser(ctx context.Context, userID int64, limit int) ([]domain.Summary, error)
	ListTopics(ctx context.Context, summaryID int64) ([]domain.SummaryTopic, error)
}

type ArticleRepository interface {
	Create(ctx context.Context, article domain.Article, sources []domain.ArticleSource) (*domain.Article, error)
	Find(ctx context.Context, articleID int64) (*domain.Article, error)
	FindByUser(ctx context.Context, userID, articleID int64) (*domain.Article, error)
	FindBySlug(ctx context.Context, userID int64, slug string) (*domain.Article, error)
	ListByUser(ctx context.Context, userID int64, limit int) ([]domain.Article, error)
	ListSources(ctx context.Context, articleID int64) ([]domain.ArticleSource, error)
	Update(ctx context.Context, article domain.Article) (*domain.Article, error)
}

type ObsidianExportRepository interface {
	Create(ctx context.Context, export domain.ObsidianExport) (*domain.ObsidianExport, error)
	FindByContentHash(ctx context.Context, hash string) (*domain.ObsidianExport, error)
}

type SummaryScheduleRepository interface {
	Create(ctx context.Context, schedule domain.SummarySchedule) (*domain.SummarySchedule, error)
	Update(ctx context.Context, schedule domain.SummarySchedule) (*domain.SummarySchedule, error)
	ListByUser(ctx context.Context, userID int64) ([]domain.SummarySchedule, error)
	ListEnabled(ctx context.Context) ([]domain.SummarySchedule, error)
	CreateRun(ctx context.Context, run domain.ScheduleRun) (*domain.ScheduleRun, error)
	CompleteRun(ctx context.Context, runID int64, status domain.JobStatus, collectionJobID, summaryID, exportID *int64, message *string) error
	MarkScheduleRun(ctx context.Context, scheduleID int64, runAt time.Time) error
}
