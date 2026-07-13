package bot

import (
	"context"
	"errors"
	"fmt"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

type AuthController interface {
	Start(ctx context.Context, telegramUserID int64) (*tdlib.AuthStatus, error)
	SubmitPhoneNumber(ctx context.Context, telegramUserID int64, phone string) (*tdlib.AuthStatus, error)
	SubmitCode(ctx context.Context, telegramUserID int64, code string) (*tdlib.AuthStatus, error)
	SubmitPassword(ctx context.Context, telegramUserID int64, password string) (*tdlib.AuthStatus, error)
	Status(ctx context.Context, telegramUserID int64) (*tdlib.AuthStatus, error)
	DeleteSession(ctx context.Context, telegramUserID int64) error
}

func authStatusText(status *tdlib.AuthStatus) string {
	if status == nil {
		return "Сессия Telegram не запущена."
	}
	switch status.AuthState {
	case tdlib.AuthorizationStateWaitPhoneNumber:
		return "Введите номер телефона в международном формате."
	case tdlib.AuthorizationStateWaitCode:
		return "Введите код подтверждения из Telegram."
	case tdlib.AuthorizationStateWaitPassword:
		return "Введите пароль 2FA. Пароль не будет сохранен."
	case tdlib.AuthorizationStateReady:
		return "Telegram-аккаунт подключен."
	case tdlib.AuthorizationStateClosed:
		return "Сессия Telegram отключена."
	case tdlib.AuthorizationStateError:
		return "TDLib сообщил об ошибке авторизации."
	default:
		return fmt.Sprintf("Текущее состояние авторизации: %s.", status.AuthState)
	}
}

func authViewFor(status *tdlib.AuthStatus) domain.SessionStatus {
	if status == nil {
		return domain.SessionStatusDisconnected
	}
	return status.SessionState
}

func authDialogView(status *tdlib.AuthStatus) DialogView {
	if status == nil {
		return ViewSettings
	}
	switch status.AuthState {
	case tdlib.AuthorizationStateWaitPhoneNumber:
		return ViewAuthPhone
	case tdlib.AuthorizationStateWaitCode:
		return ViewAuthCode
	case tdlib.AuthorizationStateWaitPassword:
		return ViewAuthPassword
	default:
		return ViewSettings
	}
}

func publicAuthError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, tdlib.ErrUnauthorizedOwner) {
		return "Доступ запрещен."
	}
	return "Не удалось выполнить действие авторизации. Проверьте конфигурацию TDLib и повторите позже."
}
