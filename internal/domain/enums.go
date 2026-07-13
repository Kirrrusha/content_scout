package domain

type SessionStatus string

const (
	SessionStatusDisconnected SessionStatus = "disconnected"
	SessionStatusAuthorizing  SessionStatus = "authorizing"
	SessionStatusConnected    SessionStatus = "connected"
	SessionStatusError        SessionStatus = "error"
)

type ChatType string

const (
	ChatTypePrivate    ChatType = "private"
	ChatTypeGroup      ChatType = "group"
	ChatTypeSupergroup ChatType = "supergroup"
	ChatTypeChannel    ChatType = "channel"
	ChatTypeUnknown    ChatType = "unknown"
)

type SummarySourceType string

const (
	SummarySourceTypeChat   SummarySourceType = "chat"
	SummarySourceTypeGroup  SummarySourceType = "group"
	SummarySourceTypeFolder SummarySourceType = "folder"
)

type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusCollecting JobStatus = "collecting"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
)

type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "high"
	ConfidenceMedium ConfidenceLevel = "medium"
	ConfidenceLow    ConfidenceLevel = "low"
)

type ArticleType string

const (
	ArticleTypeEducational ArticleType = "educational"
	ArticleTypeGuide       ArticleType = "guide"
	ArticleTypeAnalysis    ArticleType = "analysis"
	ArticleTypeOutline     ArticleType = "outline"
	ArticleTypeTelegram    ArticleType = "telegram_post"
)

type ArticleStatus string

const (
	ArticleStatusRaw       ArticleStatus = "raw"
	ArticleStatusDraft     ArticleStatus = "draft"
	ArticleStatusReview    ArticleStatus = "review"
	ArticleStatusReady     ArticleStatus = "ready"
	ArticleStatusPublished ArticleStatus = "published"
	ArticleStatusArchived  ArticleStatus = "archived"
)
