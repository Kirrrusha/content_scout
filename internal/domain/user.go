package domain

import "time"

type User struct {
	ID             int64
	TelegramUserID int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
