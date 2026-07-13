package bot

import (
	"context"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/schedules"
)

type IncomingKind string

const (
	IncomingMessage  IncomingKind = "message"
	IncomingCallback IncomingKind = "callback"
)

type Incoming struct {
	Kind            IncomingKind
	UserID          int64
	ChatID          int64
	MessageID       int
	Text            string
	Command         string
	CallbackID      string
	CallbackData    string
	CallbackMessage int
}

type Outgoing struct {
	ChatID         int64
	Text           string
	Menu           Menu
	DocumentPath   string
	DocumentName   string
	EditMessageID  int
	CallbackID     string
	AnswerCallback string
}

type Sender interface {
	Send(ctx context.Context, out Outgoing) error
}

type ScheduleController interface {
	List(ctx context.Context, telegramUserID int64) ([]domain.SummarySchedule, error)
	Get(ctx context.Context, telegramUserID, scheduleID int64) (*domain.SummarySchedule, error)
	Create(ctx context.Context, req schedules.Request) (*domain.SummarySchedule, error)
	Delete(ctx context.Context, telegramUserID, scheduleID int64) error
	SetEnabled(ctx context.Context, telegramUserID, scheduleID int64, enabled bool) (*domain.SummarySchedule, error)
	Run(ctx context.Context, telegramUserID, scheduleID int64) (*domain.Job, error)
	ListRuns(ctx context.Context, telegramUserID, scheduleID int64, limit int) ([]domain.ScheduleRun, error)
}

type MenuButton struct {
	Text string
	Data string
}

type Menu [][]MenuButton
