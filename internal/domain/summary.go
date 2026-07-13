package domain

import "time"

type SummaryJob struct {
	ID          int64
	UserID      int64
	SourceType  SummarySourceType
	SourceID    int64
	Status      JobStatus
	StartedAt   *time.Time
	CompletedAt *time.Time
	Error       *string
	CreatedAt   time.Time
}

type Summary struct {
	ID            int64
	JobID         int64
	Title         string
	Overview      string
	MessagesCount int
	SourcesCount  int
	TopicsCount   int
	Markdown      string
	CreatedAt     time.Time
}

type SummaryTopic struct {
	ID            int64
	SummaryID     int64
	Title         string
	ShortSummary  string
	FullSummary   string
	Category      string
	Importance    int
	Confidence    ConfidenceLevel
	MessagesCount int
	SourcesCount  int
	Position      int
}
