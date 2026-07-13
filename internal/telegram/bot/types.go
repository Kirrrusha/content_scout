package bot

import "context"

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

type MenuButton struct {
	Text string
	Data string
}

type Menu [][]MenuButton
