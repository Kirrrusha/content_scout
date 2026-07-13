package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/summary"
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
	ownerID   int64
	states    StateStore
	auth      AuthController
	sync      SyncController
	groups    GroupController
	collector CollectionController
	summary   SummaryController
	browser   SummaryBrowserController
	articles  ArticleController
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

func NewRouterWithAllControllers(ownerID int64, states StateStore, auth AuthController, sync SyncController, groups GroupController) *Router {
	return NewRouterWithRuntime(ownerID, states, auth, sync, groups, nil)
}

func NewRouterWithRuntime(ownerID int64, states StateStore, auth AuthController, sync SyncController, groups GroupController, collector CollectionController) *Router {
	return NewRouterWithServices(ownerID, states, auth, sync, groups, collector, nil)
}

func NewRouterWithServices(ownerID int64, states StateStore, auth AuthController, sync SyncController, groups GroupController, collector CollectionController, summary SummaryController) *Router {
	return NewRouterWithBrowser(ownerID, states, auth, sync, groups, collector, summary, nil)
}

func NewRouterWithBrowser(ownerID int64, states StateStore, auth AuthController, sync SyncController, groups GroupController, collector CollectionController, summary SummaryController, browser SummaryBrowserController) *Router {
	return NewRouterWithArticle(ownerID, states, auth, sync, groups, collector, summary, browser, nil)
}

func NewRouterWithArticle(ownerID int64, states StateStore, auth AuthController, sync SyncController, groups GroupController, collector CollectionController, summary SummaryController, browser SummaryBrowserController, articles ArticleController) *Router {
	return &Router{ownerID: ownerID, states: states, auth: auth, sync: sync, groups: groups, collector: collector, summary: summary, browser: browser, articles: articles}
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
	case "groups":
		return r.showGroups(ctx, in.ChatID, in.UserID, 0, "")
	case "group_create":
		return r.createGroup(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/group_create")))
	case "group_rename":
		return r.renameGroup(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/group_rename")))
	case "group_delete":
		return r.deleteGroup(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/group_delete")))
	case "group_add_chat":
		return r.addGroupChat(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/group_add_chat")))
	case "group_remove_chat":
		return r.removeGroupChat(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/group_remove_chat")))
	case "group_chats":
		return r.showGroupChats(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/group_chats")))
	case "collect_group":
		return r.collectGroup(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/collect_group")))
	case "summarize_collection":
		return r.summarizeCollection(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/summarize_collection")))
	case "summaries":
		return r.showSummaries(ctx, in.ChatID, in.UserID, 0, "")
	case "summary":
		return r.showSummary(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/summary")), 0, "")
	case "summary_topics":
		return r.showSummaryTopics(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/summary_topics")), 0, "")
	case "topic":
		return r.showTopic(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/topic")), 0, "")
	case "article_from_summary":
		return r.createArticleFromSummary(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/article_from_summary")), 0, "")
	case "article_from_topic":
		return r.createArticleFromTopic(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/article_from_topic")), 0, "")
	case "articles":
		return r.showArticles(ctx, in.ChatID, in.UserID, 0, "")
	case "article":
		return r.showArticle(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/article")), 0, "")
	case "article_title":
		return r.renameArticle(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/article_title")), 0, "")
	case "article_tags":
		return r.updateArticleTags(ctx, in.ChatID, in.UserID, strings.TrimSpace(strings.TrimPrefix(in.Text, "/article_tags")), 0, "")
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
	if strings.HasPrefix(in.CallbackData, "sum:") {
		out, err = r.handleSummaryCallback(ctx, in)
		out.CallbackID = in.CallbackID
		return out, err
	}
	if strings.HasPrefix(in.CallbackData, "art:") {
		out, err = r.handleArticleCallback(ctx, in)
		out.CallbackID = in.CallbackID
		return out, err
	}
	switch in.CallbackData {
	case ActionBackHome:
		out, err = r.showHome(ctx, in.ChatID, in.UserID, in.CallbackMessage, "Открыто главное меню.")
	case ActionNewSummary:
		out, err = r.showPlaceholder(ctx, in.ChatID, in.UserID, ViewNewSummary, "Создание сводки будет запускаться через фоновые задачи в одном из следующих PR.", in.CallbackMessage, "Раздел сводки открыт.")
	case ActionFolders:
		out, err = r.showFolders(ctx, in.ChatID, in.UserID, in.CallbackMessage, "Раздел папок открыт.")
	case ActionGroups:
		out, err = r.showGroups(ctx, in.ChatID, in.UserID, in.CallbackMessage, "Раздел групп открыт.")
	case ActionSelectedSources:
		out, err = r.showChats(ctx, in.ChatID, in.UserID, in.CallbackMessage, "Раздел источников открыт.")
	case ActionHistory:
		out, err = r.showSummaries(ctx, in.ChatID, in.UserID, in.CallbackMessage, "История открыта.")
	case ActionArticles:
		out, err = r.showArticles(ctx, in.ChatID, in.UserID, in.CallbackMessage, "Статьи открыты.")
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

func (r *Router) showGroups(ctx context.Context, chatID, userID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.groups == nil {
		return Outgoing{ChatID: chatID, Text: "Группы источников пока не настроены.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	groups, err := r.groups.List(ctx, userID)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicGroupError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err := r.states.Set(ctx, userID, DialogState{View: ViewGroups}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state: %w", err)
	}
	return Outgoing{ChatID: chatID, Text: groupsText(groups), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func (r *Router) createGroup(ctx context.Context, chatID, userID int64, raw string) (Outgoing, error) {
	if r.groups == nil {
		return Outgoing{ChatID: chatID, Text: "Группы источников пока не настроены.", Menu: BackMenu()}, nil
	}
	if raw == "" {
		return Outgoing{ChatID: chatID, Text: "Использование: /group_create <name>", Menu: BackMenu()}, nil
	}
	group, err := r.groups.Create(ctx, userID, raw, "")
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicGroupError(err), Menu: BackMenu()}, nil
	}
	return Outgoing{ChatID: chatID, Text: fmt.Sprintf("Группа создана: %d. %s", group.ID, group.Name), Menu: BackMenu()}, nil
}

func (r *Router) renameGroup(ctx context.Context, chatID, userID int64, raw string) (Outgoing, error) {
	if r.groups == nil {
		return Outgoing{ChatID: chatID, Text: "Группы источников пока не настроены.", Menu: BackMenu()}, nil
	}
	parts := strings.Fields(raw)
	if len(parts) < 2 {
		return Outgoing{ChatID: chatID, Text: "Использование: /group_rename <id> <name>", Menu: BackMenu()}, nil
	}
	groupID, err := parseID(parts[0])
	if err != nil {
		return Outgoing{ChatID: chatID, Text: "Некорректный ID группы.", Menu: BackMenu()}, nil
	}
	group, err := r.groups.Update(ctx, userID, groupID, strings.Join(parts[1:], " "), "")
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicGroupError(err), Menu: BackMenu()}, nil
	}
	return Outgoing{ChatID: chatID, Text: fmt.Sprintf("Группа переименована: %d. %s", group.ID, group.Name), Menu: BackMenu()}, nil
}

func (r *Router) deleteGroup(ctx context.Context, chatID, userID int64, raw string) (Outgoing, error) {
	if r.groups == nil {
		return Outgoing{ChatID: chatID, Text: "Группы источников пока не настроены.", Menu: BackMenu()}, nil
	}
	groupID, err := parseID(strings.TrimSpace(raw))
	if err != nil {
		return Outgoing{ChatID: chatID, Text: "Использование: /group_delete <id>", Menu: BackMenu()}, nil
	}
	if err := r.groups.Delete(ctx, userID, groupID); err != nil {
		return Outgoing{ChatID: chatID, Text: publicGroupError(err), Menu: BackMenu()}, nil
	}
	return Outgoing{ChatID: chatID, Text: "Группа удалена.", Menu: BackMenu()}, nil
}

func (r *Router) addGroupChat(ctx context.Context, chatID, userID int64, raw string) (Outgoing, error) {
	if r.groups == nil {
		return Outgoing{ChatID: chatID, Text: "Группы источников пока не настроены.", Menu: BackMenu()}, nil
	}
	parts := strings.Fields(raw)
	if len(parts) < 2 {
		return Outgoing{ChatID: chatID, Text: "Использование: /group_add_chat <group_id> <chat_id> [priority]", Menu: BackMenu()}, nil
	}
	groupID, err := parseID(parts[0])
	if err != nil {
		return Outgoing{ChatID: chatID, Text: "Некорректный ID группы.", Menu: BackMenu()}, nil
	}
	sourceChatID, err := parseID(parts[1])
	if err != nil {
		return Outgoing{ChatID: chatID, Text: "Некорректный ID чата.", Menu: BackMenu()}, nil
	}
	priority := 0
	if len(parts) > 2 {
		parsed, err := strconv.Atoi(parts[2])
		if err != nil {
			return Outgoing{ChatID: chatID, Text: "Некорректный priority.", Menu: BackMenu()}, nil
		}
		priority = parsed
	}
	if err := r.groups.AddChat(ctx, userID, groupID, sourceChatID, priority, true); err != nil {
		return Outgoing{ChatID: chatID, Text: publicGroupError(err), Menu: BackMenu()}, nil
	}
	return Outgoing{ChatID: chatID, Text: "Источник добавлен в группу.", Menu: BackMenu()}, nil
}

func (r *Router) removeGroupChat(ctx context.Context, chatID, userID int64, raw string) (Outgoing, error) {
	if r.groups == nil {
		return Outgoing{ChatID: chatID, Text: "Группы источников пока не настроены.", Menu: BackMenu()}, nil
	}
	parts := strings.Fields(raw)
	if len(parts) != 2 {
		return Outgoing{ChatID: chatID, Text: "Использование: /group_remove_chat <group_id> <chat_id>", Menu: BackMenu()}, nil
	}
	groupID, err := parseID(parts[0])
	if err != nil {
		return Outgoing{ChatID: chatID, Text: "Некорректный ID группы.", Menu: BackMenu()}, nil
	}
	sourceChatID, err := parseID(parts[1])
	if err != nil {
		return Outgoing{ChatID: chatID, Text: "Некорректный ID чата.", Menu: BackMenu()}, nil
	}
	if err := r.groups.RemoveChat(ctx, userID, groupID, sourceChatID); err != nil {
		return Outgoing{ChatID: chatID, Text: publicGroupError(err), Menu: BackMenu()}, nil
	}
	return Outgoing{ChatID: chatID, Text: "Источник удален из группы.", Menu: BackMenu()}, nil
}

func (r *Router) showGroupChats(ctx context.Context, chatID, userID int64, raw string) (Outgoing, error) {
	if r.groups == nil {
		return Outgoing{ChatID: chatID, Text: "Группы источников пока не настроены.", Menu: BackMenu()}, nil
	}
	groupID, err := parseID(strings.TrimSpace(raw))
	if err != nil {
		return Outgoing{ChatID: chatID, Text: "Использование: /group_chats <id>", Menu: BackMenu()}, nil
	}
	group, err := r.groups.ListChats(ctx, userID, groupID)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicGroupError(err), Menu: BackMenu()}, nil
	}
	return Outgoing{ChatID: chatID, Text: groupChatsText(group), Menu: BackMenu()}, nil
}

func (r *Router) collectGroup(ctx context.Context, chatID, userID int64, raw string) (Outgoing, error) {
	if r.collector == nil {
		return Outgoing{ChatID: chatID, Text: "Сбор сообщений пока не настроен.", Menu: BackMenu()}, nil
	}
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return Outgoing{ChatID: chatID, Text: "Использование: /collect_group <group_id> [new|24h|3d|week|latest_n] [limit]", Menu: BackMenu()}, nil
	}
	groupID, err := parseID(parts[0])
	if err != nil {
		return Outgoing{ChatID: chatID, Text: "Некорректный ID группы.", Menu: BackMenu()}, nil
	}
	mode := domain.CollectionModeNewOnly
	if len(parts) > 1 {
		mode = parseCollectionMode(parts[1])
	}
	limit := 100
	if len(parts) > 2 {
		parsed, err := strconv.Atoi(parts[2])
		if err != nil || parsed <= 0 {
			return Outgoing{ChatID: chatID, Text: "Некорректный limit.", Menu: BackMenu()}, nil
		}
		limit = parsed
	}
	result, err := r.collector.CollectGroup(ctx, collection.Request{
		TelegramUserID: userID,
		GroupID:        groupID,
		Mode:           mode,
		Limit:          limit,
	})
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicGroupError(err), Menu: BackMenu()}, nil
	}
	return Outgoing{ChatID: chatID, Text: collectionResultText(result), Menu: BackMenu()}, nil
}

func (r *Router) summarizeCollection(ctx context.Context, chatID, userID int64, raw string) (Outgoing, error) {
	if r.summary == nil {
		return Outgoing{ChatID: chatID, Text: "Генерация summary пока не настроена.", Menu: BackMenu()}, nil
	}
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return Outgoing{ChatID: chatID, Text: "Использование: /summarize_collection <collection_job_id> [short|standard|detailed]", Menu: BackMenu()}, nil
	}
	collectionJobID, err := parseID(parts[0])
	if err != nil {
		return Outgoing{ChatID: chatID, Text: "Некорректный ID collection job.", Menu: BackMenu()}, nil
	}
	format := "standard"
	if len(parts) > 1 {
		format = parts[1]
	}
	result, err := r.summary.GenerateFromCollection(ctx, summary.GenerateRequest{
		TelegramUserID:  userID,
		CollectionJobID: collectionJobID,
		Format:          format,
	})
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicGroupError(err), Menu: BackMenu()}, nil
	}
	return Outgoing{ChatID: chatID, Text: summaryResultText(result), Menu: BackMenu()}, nil
}
