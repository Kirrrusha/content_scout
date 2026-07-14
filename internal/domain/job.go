package domain

import (
	"encoding/json"
	"time"
)

type JobType string

const (
	JobTypeTelegramSync      JobType = "telegram_sync"
	JobTypeMessageCollection JobType = "message_collection"
	JobTypeSummaryGeneration JobType = "summary_generation"
	JobTypeArticleGeneration JobType = "article_generation"
	JobTypeObsidianExport    JobType = "obsidian_export"
	JobTypeScheduledPipeline JobType = "scheduled_pipeline"
)

type Job struct {
	ID               int64
	Type             JobType
	Status           JobStatus
	Payload          json.RawMessage
	Result           json.RawMessage
	Attempt          int
	MaxAttempts      int
	AvailableAt      time.Time
	LockedAt         *time.Time
	LockedBy         *string
	LeaseExpiresAt   *time.Time
	LastError        *string
	CreatedAt        time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
	DeduplicationKey *string
}

type JobPayloadScheduledPipeline struct {
	Schedule SummarySchedule `json:"schedule"`
}

type JobPayloadSummaryGeneration struct {
	TelegramUserID  int64  `json:"telegram_user_id"`
	CollectionJobID int64  `json:"collection_job_id"`
	Format          string `json:"format"`
}

type JobResultSummaryGeneration struct {
	SummaryID      int64 `json:"summary_id"`
	SummaryJobID   int64 `json:"summary_job_id"`
	TopicsCount    int   `json:"topics_count"`
	MessagesCount  int   `json:"messages_count"`
	DuplicateCount int   `json:"duplicate_count"`
}
