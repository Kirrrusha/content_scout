package tdlib

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	ListFolderChats(ctx context.Context, folderID int32) ([]domain.TelegramChat, error)
	GetChatHistory(ctx context.Context, chatID int64, fromMessageID int64, limit int) ([]domain.TelegramMessage, error)
	MarkMessagesRead(ctx context.Context, chatID int64, messageIDs []int64) error
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

type TDLibError struct {
	Code    int
	Message string
}

func (e *TDLibError) Error() string {
	return fmt.Sprintf("tdlib error %d: %s", e.Code, e.Message)
}

func IsLoginCodeCompromisedError(err error) bool {
	var tdlibErr *TDLibError
	if !errors.As(err, &tdlibErr) {
		return false
	}
	message := strings.ToLower(tdlibErr.Message)
	return strings.Contains(message, "reported this code") ||
		strings.Contains(message, "shared this code") ||
		strings.Contains(message, "code was reported")
}

func isChatsFullyLoadedError(err error) bool {
	var tdlibErr *TDLibError
	if !errors.As(err, &tdlibErr) {
		return false
	}
	return tdlibErr.Code == 404
}

func isUnexpectedSetTDLibParametersError(err error) bool {
	var tdlibErr *TDLibError
	if !errors.As(err, &tdlibErr) {
		return false
	}
	return tdlibErr.Code == 400 && strings.Contains(tdlibErr.Message, "Unexpected setTdlibParameters")
}

func CloseClientFactory(ctx context.Context, factory ClientFactory) error {
	closer, ok := factory.(ClientFactoryCloser)
	if !ok {
		return nil
	}
	return closer.Close(ctx)
}
