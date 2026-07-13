package bot

import (
	"context"
	"fmt"
	"strings"
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
)

type Router struct {
	ownerID int64
	states  StateStore
}

func NewRouter(ownerID int64, states StateStore) *Router {
	return &Router{ownerID: ownerID, states: states}
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
	case "folders":
		return r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewFolders, "Папки Telegram появятся здесь после подключения синхронизации TDLib.")
	case "chats":
		return r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewSelectedSources, "Выбранные источники и чаты появятся здесь после синхронизации.")
	case "sync":
		return r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewFolders, "Ручная синхронизация Telegram будет подключена в PR-005.")
	case "settings":
		return r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewSettings, "Раздел настроек готов. Подключение аккаунта появится в PR-004.")
	default:
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
		out, err = r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewFolders, "Папки Telegram появятся здесь после подключения синхронизации TDLib.", in.CallbackMessage, "Раздел папок открыт.")
	case ActionGroups:
		out, err = r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewGroups, "Управление группами источников появится после синхронизации папок и чатов.", in.CallbackMessage, "Раздел групп открыт.")
	case ActionSelectedSources:
		out, err = r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewSelectedSources, "Управление выбранными источниками будет подключено к репозиториям в следующем этапе.", in.CallbackMessage, "Раздел источников открыт.")
	case ActionHistory:
		out, err = r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewHistory, "История появится здесь после генерации первых сводок.", in.CallbackMessage, "История открыта.")
	case ActionArticles:
		out, err = r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewArticles, "Черновики статей появятся здесь после реализации конвертации.", in.CallbackMessage, "Статьи открыты.")
	case ActionSettings:
		out, err = r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewSettings, "Раздел настроек готов. Подключение аккаунта появится в PR-004.", in.CallbackMessage, "Настройки открыты.")
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
