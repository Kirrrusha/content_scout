package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

const (
	ActionNewSummary      = "summary:new"
	ActionFolders         = "folders:list"
	ActionGroups          = "groups:list"
	ActionSelectedSources = "sources:selected"
	ActionHistory         = "history:list"
	ActionArticles        = "articles:list"
	ActionSettings        = "settings:open"
	ActionBackHome        = "home"
	ActionAuthConnect     = "auth:connect"
	ActionAuthStatus      = "auth:status"
	ActionAuthDelete      = "auth:delete"
)

type Router struct {
	ownerID int64
	states  StateStore
	auth    AuthController
	sync    SyncController
}

func NewRouter(ownerID int64, states StateStore) *Router {
	return &Router{ownerID: ownerID, states: states}
}

func NewRouterWithAuth(ownerID int64, states StateStore, auth AuthController) *Router {
	return &Router{ownerID: ownerID, states: states, auth: auth}
}

func NewRouterWithControllers(ownerID int64, states StateStore, auth AuthController, sync SyncController) *Router {
	return &Router{ownerID: ownerID, states: states, auth: auth, sync: sync}
}

func (r *Router) Handle(ctx context.Context, in Incoming) (Outgoing, error) {
	if r.ownerID == 0 {
		return Outgoing{}, fmt.Errorf("telegram owner id is not configured")
	}
	if in.UserID != r.ownerID {
		return Outgoing{ChatID: in.ChatID, Text: "Доступ запрещен."}, nil
	}

	if in.Kind == IncomingCallback {
		return r.handleCallback(ctx, in)
	}
	return r.handleMessage(ctx, in)
}

func (r *Router) handleMessage(ctx context.Context, in Incoming) (Outgoing, error) {
	command := in.Command
	if command == "" && strings.HasPrefix(in.Text, "/") {
		fields := strings.Fields(strings.TrimPrefix(in.Text, "/"))
		if len(fields) > 0 {
			command = fields[0]
		}
	}

	switch command {
	case "start":
		return r.showHome(ctx, in.ChatID, in.UserID, 0, "")
	case "connect":
		return r.startAuth(ctx, in.ChatID, in.UserID, 0, "")
	case "session":
		return r.showAuthStatus(ctx, in.ChatID, in.UserID, 0, "")
	case "delete_session":
		return r.deleteSession(ctx, in.ChatID, in.UserID, 0, "")
	case "phone":
		return r.submitPhone(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/phone")))
	case "code":
		return r.submitCode(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/code")))
	case "password":
		return r.submitPassword(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/password")))
	case "folders":
		return r.showFolders(ctx, in.ChatID, in.UserID, 0, "")
	case "chats":
		return r.showChats(ctx, in.ChatID, in.UserID, 0, "")
	case "sync":
		return r.syncTelegram(ctx, in.ChatID, in.UserID, 0, "")
	case "settings":
		return r.showSettings(ctx, in.ChatID, in.UserID, 0, "")
	default:
		if out, ok, err := r.handleDialogInput(ctx, in); ok || err != nil {
			return out, err
		}
		return r.showHome(ctx, in.ChatID, in.UserID, 0, "")
	}
}

func (r *Router) handleCallback(ctx context.Context, in Incoming) (Outgoing, error) {
	var out Outgoing
	var err error
	switch in.CallbackData {
	case ActionBackHome:
		out, err = r.showHome(ctx, in.ChatID, in.UserID, in.CallbackMessage, "Открыто главное меню.")
	case ActionNewSummary:
		out, err = r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewNewSummary, "Создание сводки будет запускаться через фоновые задачи в одном из следующих PR.", in.CallbackMessage, "Раздел сводки открыт.")
	case ActionFolders:
		out, err = r.showFolders(ctx, in.ChatID, in.UserID, in.CallbackMessage, "Раздел папок открыт.")
	case ActionGroups:
		out, err = r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewGroups, "Управление группами источников появится после синхронизации папок и чатов.", in.CallbackMessage, "Раздел групп открыт.")
	case ActionSelectedSources:
		out, err = r.showChats(ctx, in.ChatID, in.UserID, in.CallbackMessage, "Раздел источников открыт.")
	case ActionHistory:
		out, err = r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewHistory, "История появится здесь после генерации первых сводок.", in.CallbackMessage, "История открыта.")
	case ActionArticles:
		out, err = r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewArticles, "Черновики статей появятся здесь после реализации конвертации.", in.CallbackMessage, "Статьи открыты.")
	case ActionSettings:
		out, err = r.showSettings(ctx, in.ChatID, in.UserID, in.CallbackMessage, "Настройки открыты.")
	case ActionAuthConnect:
		out, err = r.startAuth(ctx, in.ChatID, in.UserID, in.CallbackMessage, "Подключение аккаунта.")
	case ActionAuthStatus:
		out, err = r.showAuthStatus(ctx, in.ChatID, in.UserID, in.CallbackMessage, "Статус сессии.")
	case ActionAuthDelete:
		out, err = r.deleteSession(ctx, in.ChatID, in.UserID, in.CallbackMessage, "Сессия удаляется.")
	default:
		out = Outgoing{ChatID: in.ChatID, Text: "Неизвестное действие.", AnswerCallback: "Неизвестное действие."}
	}
	out.CallbackID = in.CallbackID
	return out, err
}

func (r *Router) showHome(ctx context.Context, chatID, userID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if err := r.states.Set(ctx, userID, DialogState{View: ViewStart}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state: %w", err)
	}
	return Outgoing{
		ChatID:         chatID,
		Text:           "Telegram Summary Bot\n\nВыберите действие.",
		Menu:           MainMenu(),
		EditMessageID:  editMessageID,
		AnswerCallback: callbackAnswer,
	}, nil
}

func (r *Router) showPlaceholder(ctx context.Context, chatID, userID int64, view DialogView, text string, args ...any) (Outgoing, error) {
	editMessageID := 0
	callbackAnswer := ""
	if len(args) > 0 {
		editMessageID, _ = args[0].(int)
	}
	if len(args) > 1 {
		callbackAnswer, _ = args[1].(string)
	}

	if err := r.states.Set(ctx, userID, DialogState{View: view}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state: %w", err)
	}
	return Outgoing{
		ChatID:         chatID,
		Text:           text,
		Menu:           BackMenu(),
		EditMessageID:  editMessageID,
		AnswerCallback: callbackAnswer,
	}, nil
}

func MainMenu() Menu {
	return Menu{
		{{Text: "Новая сводка", Data: ActionNewSummary}},
		{{Text: "Папки Telegram", Data: ActionFolders}, {Text: "Мои группы", Data: ActionGroups}},
		{{Text: "Выбранные источники", Data: ActionSelectedSources}},
		{{Text: "История", Data: ActionHistory}, {Text: "Статьи", Data: ActionArticles}},
		{{Text: "Настройки", Data: ActionSettings}},
	}
}

func BackMenu() Menu {
	return Menu{{{Text: "Назад", Data: ActionBackHome}}}
}

func SettingsMenu() Menu {
	return Menu{
		{{Text: "Подключить аккаунт", Data: ActionAuthConnect}},
		{{Text: "Статус сессии", Data: ActionAuthStatus}, {Text: "Удалить сессию", Data: ActionAuthDelete}},
		{{Text: "Назад", Data: ActionBackHome}},
	}
}

func (r *Router) showSettings(ctx context.Context, chatID, userID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if err := r.states.Set(ctx, userID, DialogState{View: ViewSettings}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state: %w", err)
	}
	return Outgoing{
		ChatID:         chatID,
		Text:           "Настройки Telegram-аккаунта.",
		Menu:           SettingsMenu(),
		EditMessageID:  editMessageID,
		AnswerCallback: callbackAnswer,
	}, nil
}

func (r *Router) startAuth(ctx context.Context, chatID, userID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.auth == nil {
		return Outgoing{ChatID: chatID, Text: "Авторизация TDLib пока не настроена.", Menu: SettingsMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	status, err := r.auth.Start(ctx, userID)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: SettingsMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return r.showAuthPrompt(ctx, chatID, userID, status, editMessageID, callbackAnswer)
}

func (r *Router) showAuthStatus(ctx context.Context, chatID, userID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.auth == nil {
		return Outgoing{ChatID: chatID, Text: "Авторизация TDLib пока не настроена.", Menu: SettingsMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	status, err := r.auth.Status(ctx, userID)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: SettingsMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	text := authStatusText(status)
	if status != nil {
		text = fmt.Sprintf("%s\n\nСтатус сессии: %s", text, authViewFor(status))
	}
	return Outgoing{ChatID: chatID, Text: text, Menu: SettingsMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func (r *Router) deleteSession(ctx context.Context, chatID, userID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.auth == nil {
		return Outgoing{ChatID: chatID, Text: "Авторизация TDLib пока не настроена.", Menu: SettingsMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err := r.auth.DeleteSession(ctx, userID); err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: SettingsMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err := r.states.Set(ctx, userID, DialogState{View: ViewSettings}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state: %w", err)
	}
	return Outgoing{ChatID: chatID, Text: "TDLib-сессия удалена.", Menu: SettingsMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func (r *Router) handleDialogInput(ctx context.Context, in Incoming) (Outgoing, bool, error) {
	state, ok, err := r.states.Get(ctx, in.UserID)
	if err != nil || !ok {
		return Outgoing{}, false, err
	}
	text := strings.TrimSpace(in.Text)
	switch state.View {
	case ViewAuthPhone:
		out, err := r.submitPhone(ctx, in.ChatID, in.UserID, text)
		return out, true, err
	case ViewAuthCode:
		out, err := r.submitCode(ctx, in.ChatID, in.UserID, text)
		return out, true, err
	case ViewAuthPassword:
		out, err := r.submitPassword(ctx, in.ChatID, in.UserID, text)
		return out, true, err
	default:
		return Outgoing{}, false, nil
	}
}

func (r *Router) submitPhone(ctx context.Context, chatID, userID int64, phone string) (Outgoing, error) {
	if r.auth == nil {
		return Outgoing{ChatID: chatID, Text: "Авторизация TDLib пока не настроена.", Menu: SettingsMenu()}, nil
	}
	if phone == "" {
		return Outgoing{ChatID: chatID, Text: "Введите номер телефона после команды /phone или отдельным сообщением.", Menu: BackMenu()}, nil
	}
	status, err := r.auth.SubmitPhoneNumber(ctx, userID, phone)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: SettingsMenu()}, nil
	}
	return r.showAuthPrompt(ctx, chatID, userID, status, 0, "")
}

func (r *Router) submitCode(ctx context.Context, chatID, userID int64, code string) (Outgoing, error) {
	if r.auth == nil {
		return Outgoing{ChatID: chatID, Text: "Авторизация TDLib пока не настроена.", Menu: SettingsMenu()}, nil
	}
	if code == "" {
		return Outgoing{ChatID: chatID, Text: "Введите код после команды /code или отдельным сообщением.", Menu: BackMenu()}, nil
	}
	status, err := r.auth.SubmitCode(ctx, userID, code)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: SettingsMenu()}, nil
	}
	return r.showAuthPrompt(ctx, chatID, userID, status, 0, "")
}

func (r *Router) submitPassword(ctx context.Context, chatID, userID int64, password string) (Outgoing, error) {
	if r.auth == nil {
		return Outgoing{ChatID: chatID, Text: "Авторизация TDLib пока не настроена.", Menu: SettingsMenu()}, nil
	}
	if password == "" {
		return Outgoing{ChatID: chatID, Text: "Введите пароль 2FA после команды /password или отдельным сообщением.", Menu: BackMenu()}, nil
	}
	status, err := r.auth.SubmitPassword(ctx, userID, password)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: SettingsMenu()}, nil
	}
	return r.showAuthPrompt(ctx, chatID, userID, status, 0, "")
}

func (r *Router) showAuthPrompt(ctx context.Context, chatID, userID int64, status *tdlib.AuthStatus, editMessageID int, callbackAnswer string) (Outgoing, error) {
	view := authDialogView(status)
	if err := r.states.Set(ctx, userID, DialogState{View: view}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state: %w", err)
	}
	menu := BackMenu()
	if view == ViewSettings {
		menu = SettingsMenu()
	}
	return Outgoing{
		ChatID:         chatID,
		Text:           authStatusText(status),
		Menu:           menu,
		EditMessageID:  editMessageID,
		AnswerCallback: callbackAnswer,
	}, nil
}

func (r *Router) syncTelegram(ctx context.Context, chatID, userID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.sync == nil {
		return Outgoing{ChatID: chatID, Text: "Синхронизация Telegram пока не настроена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	result, err := r.sync.Sync(ctx, userID)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err := r.states.Set(ctx, userID, DialogState{View: ViewFolders}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state: %w", err)
	}
	return Outgoing{ChatID: chatID, Text: syncResultText(result), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func (r *Router) showFolders(ctx context.Context, chatID, userID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.sync == nil {
		return Outgoing{ChatID: chatID, Text: "Синхронизация Telegram пока не настроена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	folders, err := r.sync.ListFolders(ctx, userID)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err := r.states.Set(ctx, userID, DialogState{View: ViewFolders}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state: %w", err)
	}
	return Outgoing{ChatID: chatID, Text: foldersText(folders), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func (r *Router) showChats(ctx context.Context, chatID, userID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.sync == nil {
		return Outgoing{ChatID: chatID, Text: "Синхронизация Telegram пока не настроена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	chats, err := r.sync.ListChats(ctx, userID)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err := r.states.Set(ctx, userID, DialogState{View: ViewSelectedSources}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state: %w", err)
	}
	return Outgoing{ChatID: chatID, Text: chatsText(chats), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}
