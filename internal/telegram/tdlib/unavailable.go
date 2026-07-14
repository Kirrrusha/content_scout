package tdlib

import (
	"context"
	"errors"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type UnavailableClientFactory struct{}

func (UnavailableClientFactory) NewClient(string) (TelegramClient, error) {
	return unavailableClient{}, nil
}

type unavailableClient struct{}

func (unavailableClient) Start(context.Context) error {
	return errors.New("native TDLib adapter is not connected yet")
}

func (unavailableClient) Stop(context.Context) error {
	return nil
}

func (unavailableClient) AuthorizationState(context.Context) (AuthorizationState, error) {
	return AuthorizationStateClosed, nil
}

func (unavailableClient) SubmitPhoneNumber(context.Context, string) error {
	return errors.New("native TDLib adapter is not connected yet")
}

func (unavailableClient) SubmitCode(context.Context, string) error {
	return errors.New("native TDLib adapter is not connected yet")
}

func (unavailableClient) SubmitPassword(context.Context, string) error {
	return errors.New("native TDLib adapter is not connected yet")
}

func (unavailableClient) ListFolders(context.Context) ([]domain.TelegramFolder, error) {
	return nil, errors.New("native TDLib adapter is not connected yet")
}

func (unavailableClient) ListChats(context.Context, ChatList) ([]domain.TelegramChat, error) {
	return nil, errors.New("native TDLib adapter is not connected yet")
}

func (unavailableClient) ListFolderChats(context.Context, int32) ([]domain.TelegramChat, error) {
	return nil, errors.New("native TDLib adapter is not connected yet")
}

func (unavailableClient) GetChatHistory(context.Context, int64, int64, int) ([]domain.TelegramMessage, error) {
	return nil, errors.New("native TDLib adapter is not connected yet")
}

func (unavailableClient) MarkMessagesRead(context.Context, int64, []int64) error {
	return errors.New("native TDLib adapter is not connected yet")
}
