package domain

import "time"

type Article struct {
	ID              int64
	UserID          int64
	Title           string
	Slug            string
	Type            ArticleType
	Status          ArticleStatus
	ContentMarkdown string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ArticleSource struct {
	ID             int64
	ArticleID      int64
	TelegramChatID int64
	MessageID      int64
	SourceTitle    string
	SourceURL      string
	PublishedAt    time.Time
}
