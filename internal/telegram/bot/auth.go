package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
		return "Telegram отправил код подтверждения. Не отправляйте этот код в чат бота: Telegram может заблокировать вход как раскрытие кода. Передайте код через защищенный HTTP API /telegram/auth/code или локальный админ-интерфейс."
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
	if tdlib.IsLoginCodeCompromisedError(err) {
		return "Telegram заблокировал вход, потому что код подтверждения был сочтен раскрытым. Начните подключение заново и не отправляйте новый код в Telegram-чат; используйте защищенный HTTP API или локальный админ-интерфейс."
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "TELEGRAM_API_ID is not configured"):
		return "TELEGRAM_API_ID не настроен. Укажите Telegram API id из my.telegram.org и перезапустите сервис."
	case strings.Contains(message, "TELEGRAM_API_HASH is not configured"):
		return "TELEGRAM_API_HASH не настроен. Укажите Telegram API hash из my.telegram.org и перезапустите сервис."
	case strings.Contains(message, "TDLIB_DATABASE_DIR is not configured") || strings.Contains(message, "TDLib session path is not configured"):
		return "TDLIB_DATABASE_DIR не настроен. Укажите директорию для TDLib-сессии и перезапустите сервис."
	case strings.Contains(message, "native TDLib adapter is not connected yet"):
		return "Native TDLib adapter не подключен. Запустите сервис в Docker-сборке или локально с CGO_ENABLED=1 и -tags tdlib, чтобы авторизация работала."
	case strings.Contains(message, "summarize with llm") && (strings.Contains(message, "Client.Timeout exceeded") || strings.Contains(message, "context deadline exceeded")):
		return "LLM не успела создать сводку вовремя. Повторите действие позже или выберите группу/период с меньшим числом сообщений."
	case strings.Contains(message, "convert article with llm") && (strings.Contains(message, "Client.Timeout exceeded") || strings.Contains(message, "context deadline exceeded")):
		return "LLM не успела подготовить статью вовремя. Повторите действие позже или попробуйте более короткую сводку."
	case strings.Contains(message, "Client.Timeout exceeded") || strings.Contains(message, "context deadline exceeded"):
		return "Внутренний API не ответил вовремя. Проверьте, что API запущен и TDLib не зависла на авторизации."
	case strings.Contains(message, "connection refused"):
		return "Внутренний API недоступен. Запустите API-сервис и повторите действие."
	case strings.Contains(message, "telegram session is not started"):
		return "Telegram-сессия еще не запущена. Нажмите «Подключить аккаунт» в настройках или выполните /connect."
	case strings.Contains(message, "telegram session is not connected"):
		return "Telegram-аккаунт еще не подключен. Завершите авторизацию через /connect, затем повторите действие."
	case strings.Contains(message, "telegram authorization is not ready"):
		return "Авторизация Telegram еще не завершена. Проверьте статус через /session и завершите подключение."
	case strings.Contains(message, "PHONE_NUMBER_INVALID"):
		return "Telegram не принял номер телефона. Введите его в международном формате с плюсом, например +79204675533."
	case strings.Contains(message, "td.binlog") && strings.Contains(message, "already in use"):
		return "TDLib-сессия уже открыта другим процессом. Остановите API или другой запущенный экземпляр бота и повторите действие."
	}
	return fmt.Sprintf("Не удалось выполнить действие Telegram. Техническая причина: %s", message)
}
