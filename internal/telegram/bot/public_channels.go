package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

func (r *Router) addPublicChannelCommand(ctx context.Context, chatID, userID int64, raw string) (Outgoing, error) {
	parts := strings.Fields(raw)
	if len(parts) != 2 {
		return Outgoing{ChatID: chatID, Text: "Использование: /group_add_public <group_id> <@username или ссылка t.me>", Menu: BackMenu()}, nil
	}
	groupID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || groupID <= 0 {
		return Outgoing{ChatID: chatID, Text: "group_id должен быть числом.", Menu: BackMenu()}, nil
	}
	return r.addPublicChannel(ctx, chatID, userID, groupID, parts[1], "groups", 0, "")
}

func (r *Router) promptPublicChannel(ctx context.Context, chatID, userID, groupID int64, returnAction string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if _, ok := r.sync.(PublicChannelController); !ok {
		return Outgoing{ChatID: chatID, Text: "Поиск открытых каналов пока не настроен.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err := r.states.Set(ctx, userID, DialogState{View: ViewAddPublicChannel, GroupID: groupID, ReturnAction: returnAction}); err != nil {
		return Outgoing{}, fmt.Errorf("set public channel dialog state: %w", err)
	}
	return Outgoing{
		ChatID: chatID,
		Text:   "Отправьте @username или ссылку вида https://t.me/channel. Подписываться на канал не нужно.",
		Menu:   publicChannelBackMenu(groupID, returnAction), EditMessageID: editMessageID, AnswerCallback: callbackAnswer,
	}, nil
}

func (r *Router) addPublicChannelFromDialog(ctx context.Context, chatID, userID int64, state DialogState, reference string) (Outgoing, error) {
	if reference == "" {
		return Outgoing{ChatID: chatID, Text: "Отправьте @username или ссылку t.me на открытый канал.", Menu: publicChannelBackMenu(state.GroupID, state.ReturnAction)}, nil
	}
	return r.addPublicChannel(ctx, chatID, userID, state.GroupID, reference, state.ReturnAction, 0, "")
}

func (r *Router) addPublicChannel(ctx context.Context, chatID, userID, groupID int64, reference, returnAction string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	controller, ok := r.sync.(PublicChannelController)
	if !ok {
		return Outgoing{ChatID: chatID, Text: "Поиск открытых каналов пока не настроен.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	channel, err := controller.AddPublicChannel(ctx, userID, groupID, reference)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicChannelError(err), Menu: publicChannelBackMenu(groupID, returnAction), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err := r.states.Set(ctx, userID, DialogState{View: returnDialogView(returnAction)}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state after adding public channel: %w", err)
	}
	return Outgoing{
		ChatID: chatID,
		Text:   fmt.Sprintf("Канал «%s» добавлен в группу источников.", channel.Title),
		Menu:   publicChannelSuccessMenu(groupID, returnAction), EditMessageID: editMessageID, AnswerCallback: callbackAnswer,
	}, nil
}

func publicChannelError(err error) string {
	switch {
	case errors.Is(err, tdlib.ErrInvalidPublicChannel), strings.Contains(err.Error(), tdlib.ErrInvalidPublicChannel.Error()):
		return "Некорректный адрес. Отправьте @username или ссылку t.me на открытый канал."
	case errors.Is(err, tdlib.ErrPublicChannelOnly), strings.Contains(err.Error(), tdlib.ErrPublicChannelOnly.Error()):
		return "По этому адресу найден не публичный канал. Добавить можно только открытый Telegram-канал."
	case errors.Is(err, tdlib.ErrSourceGroupNotFound), strings.Contains(err.Error(), tdlib.ErrSourceGroupNotFound.Error()):
		return "Группа источников не найдена."
	case strings.Contains(err.Error(), "resolve public channel"):
		return "Открытый канал не найден или недоступен. Проверьте @username или ссылку."
	default:
		return publicAuthError(err)
	}
}

func publicChannelBackMenu(groupID int64, returnAction string) Menu {
	return Menu{{{Text: "Назад", Data: publicChannelReturnCallback(groupID, returnAction)}}}
}

func publicChannelSuccessMenu(groupID int64, returnAction string) Menu {
	switch returnAction {
	case "newsum":
		return newSummaryModeMenu(groupID)
	case "schedule":
		return scheduleCountMenu(groupID)
	default:
		return groupDetailsMenu(groupID)
	}
}

func publicChannelReturnCallback(groupID int64, returnAction string) string {
	switch returnAction {
	case "newsum":
		return fmt.Sprintf("newsum:group:%d", groupID)
	case "schedule":
		return fmt.Sprintf("sched:group:%d", groupID)
	default:
		return fmt.Sprintf("groups:open:%d", groupID)
	}
}

func returnDialogView(returnAction string) DialogView {
	switch returnAction {
	case "newsum":
		return ViewNewSummary
	case "schedule":
		return ViewSchedules
	default:
		return ViewGroups
	}
}
