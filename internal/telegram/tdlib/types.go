package tdlib

import (
	"context"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type AuthorizationState string

const (
	AuthorizationStateUnknown         AuthorizationState = "unknown"
	AuthorizationStateWaitPhoneNumber AuthorizationState = "wait_phone_number"
	AuthorizationStateWaitCode        AuthorizationState = "wait_code"
	AuthorizationStateWaitPassword    AuthorizationState = "wait_password"
	AuthorizationStateReady           AuthorizationState = "ready"
	AuthorizationStateClosed          AuthorizationState = "closed"
	AuthorizationStateError           AuthorizationState = "error"
)

type ChatList string

const (
	ChatListMain    ChatList = "main"
	ChatListArchive ChatList = "archive"
)

type TelegramClient interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	AuthorizationState(ctx context.Context) (AuthorizationState, error)
	SubmitPhoneNumber(ctx context.Context, phone string) error
	SubmitCode(ctx context.Context, code string) error
	SubmitPassword(ctx context.Context, password string) error
	ListFolders(ctx context.Context) ([]domain.TelegramFolder, error)
	ListChats(ctx context.Context, list ChatList) ([]domain.TelegramChat, error)
	GetChatHistory(ctx context.Context, chatID int64, fromMessageID int64, limit int) ([]domain.TelegramMessage, error)
}

type ClientFactory interface {
	NewClient(sessionPath string) (TelegramClient, error)
}

type ClientFactoryCloser interface {
	Close(context.Context) error
}

type ClientConfig struct {
	APIID   int
	APIHash string
}

func CloseClientFactory(ctx context.Context, factory ClientFactory) error {
	closer, ok := factory.(ClientFactoryCloser)
	if !ok {
		return nil
	}
	return closer.Close(ctx)
}
