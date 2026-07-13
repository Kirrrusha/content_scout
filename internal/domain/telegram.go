package domain

import "time"

type TelegramSession struct {
	ID            int64
	UserID        int64
	StoragePath   string
	Status        SessionStatus
	LastConnected *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type TelegramFolder struct {
	ID         int64
	UserID     int64
	TelegramID int32
	Name       string
	SyncedAt   time.Time
}

type TelegramChat struct {
	ID             int64
	UserID         int64
	TelegramChatID int64
	Title          string
	Username       *string
	Type           ChatType
	IsArchived     bool
	IsMuted        bool
	UnreadCount    int
	LastMessageID  int64
	UpdatedAt      time.Time
}

type SourceGroup struct {
	ID          int64
	UserID      int64
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type SourceGroupChat struct {
	GroupID  int64
	ChatID   int64
	Priority int
	Enabled  bool
}

type ReadPosition struct {
	UserID                  int64
	ChatID                  int64
	LastSummarizedMessageID int64
	UpdatedAt               time.Time
}

type TelegramMessage struct {
	ChatID     int64
	MessageID  int64
	Date       time.Time
	EditDate   *time.Time
	SenderID   int64
	SenderName string
	Text       string
	Caption    string
	URL        string
	ReplyToID  *int64
	Forwarded  bool
	HasMedia   bool
	MediaType  string
}

type MessageCollectionJob struct {
	ID        int64
	UserID    int64
	GroupID   int64
	Mode      CollectionMode
	Limit     int
	Status    JobStatus
	Error     *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CollectedMessage struct {
	ID             int64
	JobID          int64
	UserID         int64
	ChatID         int64
	TelegramChatID int64
	MessageID      int64
	Date           time.Time
	EditDate       *time.Time
	SenderID       int64
	SenderName     string
	Text           string
	Caption        string
	URL            string
	ReplyToID      *int64
	Forwarded      bool
	HasMedia       bool
	MediaType      string
	CreatedAt      time.Time
}
