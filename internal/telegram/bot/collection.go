package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/summary"
)

type CollectionController interface {
	CollectGroup(ctx context.Context, req collection.Request) (*collection.Result, error)
}

type SummaryController interface {
	GenerateFromCollection(ctx context.Context, req summary.GenerateRequest) (*summary.GenerateResult, error)
}

func collectionResultText(result *collection.Result) string {
	if result == nil {
		return "Сбор сообщений не выполнен."
	}
	if result.MessagesCount == 0 {
		return fmt.Sprintf("Новых материалов нет.\n\nПроверено источников: %d\n\nМожно выбрать другой период или вернуться позже.", result.ChatsCount)
	}
	return fmt.Sprintf("Материал для сводки найден.\n\nИсточников: %d\nСообщений: %d\n\nТеперь можно создать сводку.", result.ChatsCount, result.MessagesCount)
}

func summaryResultText(result *summary.GenerateResult) string {
	if result == nil {
		return "Сводка не создана."
	}
	return fmt.Sprintf("Сводка готова.\n\nТем: %d\nСообщений в работе: %d\nДубликатов убрано: %d", result.TopicsCount, result.MessagesCount, result.DuplicateCount)
}

func parseCollectionMode(value string) domain.CollectionMode {
	if value == "" {
		return domain.CollectionModeNewOnly
	}
	return domain.CollectionMode(value)
}

func (r *Router) showNewSummary(ctx context.Context, chatID, userID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.groups == nil {
		return Outgoing{ChatID: chatID, Text: "Группы источников пока не настроены.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	groups, err := r.groups.List(ctx, userID)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicGroupError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err := r.states.Set(ctx, userID, DialogState{View: ViewNewSummary}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state: %w", err)
	}
	if len(groups) == 0 {
		return Outgoing{ChatID: chatID, Text: "Сначала создайте группу источников и добавьте в нее чаты.", Menu: Menu{{{Text: "Мои группы", Data: ActionGroups}}, {{Text: "Назад", Data: ActionBackHome}}}, EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return Outgoing{ChatID: chatID, Text: "Выберите группу источников для новой сводки.", Menu: newSummaryGroupsMenu(groups), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func (r *Router) handleNewSummaryCallback(ctx context.Context, in Incoming) (Outgoing, error) {
	fields := strings.Split(in.CallbackData, ":")
	if len(fields) < 2 {
		return unknownCallback(in), nil
	}
	switch fields[1] {
	case "group":
		if len(fields) != 3 {
			return unknownCallback(in), nil
		}
		groupID, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || groupID <= 0 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестная группа.", AnswerCallback: "Неизвестная группа."}, nil
		}
		return Outgoing{
			ChatID:         in.ChatID,
			Text:           "Какие сообщения взять в сводку?\n\nНовые (непрочитанные) — сообщения после последней успешной сводки по этой группе. После создания сводки они будут помечены как прочитанные.\n\nПериоды 24 часа, 3 дня и неделя берут сообщения по дате публикации.",
			Menu:           newSummaryModeMenu(groupID),
			EditMessageID:  in.CallbackMessage,
			AnswerCallback: "Период сбора.",
		}, nil
	case "mode":
		if len(fields) != 4 {
			return unknownCallback(in), nil
		}
		groupID, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || groupID <= 0 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестная группа.", AnswerCallback: "Неизвестная группа."}, nil
		}
		return r.collectGroupFromButton(ctx, in.ChatID, in.UserID, groupID, parseCollectionMode(fields[3]), in.CallbackMessage, "Собираю сообщения.")
	case "generate":
		if len(fields) != 3 {
			return unknownCallback(in), nil
		}
		jobID, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || jobID <= 0 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестный job.", AnswerCallback: "Неизвестный job."}, nil
		}
		return r.generateSummaryFromButton(ctx, in.ChatID, in.UserID, jobID, in.CallbackMessage, "Создаю сводку.")
	default:
		return unknownCallback(in), nil
	}
}

func (r *Router) collectGroupFromButton(ctx context.Context, chatID, userID, groupID int64, mode domain.CollectionMode, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.collector == nil {
		return Outgoing{ChatID: chatID, Text: "Сбор сообщений пока не настроен.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	result, err := r.collector.CollectGroup(ctx, collection.Request{
		TelegramUserID: userID,
		GroupID:        groupID,
		Mode:           mode,
		Limit:          100,
	})
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicGroupError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return Outgoing{ChatID: chatID, Text: collectionResultText(result), Menu: collectionResultMenu(result), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func (r *Router) generateSummaryFromButton(ctx context.Context, chatID, userID, collectionJobID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.summary == nil {
		return Outgoing{ChatID: chatID, Text: "Генерация summary пока не настроена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	result, err := r.summary.GenerateFromCollection(ctx, summary.GenerateRequest{
		TelegramUserID:  userID,
		CollectionJobID: collectionJobID,
		Format:          "standard",
	})
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicGroupError(fmt.Errorf("summarize with llm: %w", err)), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return Outgoing{ChatID: chatID, Text: summaryResultText(result), Menu: summaryResultMenu(result), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func newSummaryGroupsMenu(groups []domain.SourceGroup) Menu {
	menu := make(Menu, 0, len(groups)+1)
	for _, group := range groups {
		menu = append(menu, []MenuButton{{Text: fmt.Sprintf("%d. %s", group.ID, compactButtonTitle(group.Name)), Data: fmt.Sprintf("newsum:group:%d", group.ID)}})
	}
	menu = append(menu, []MenuButton{{Text: "Назад", Data: ActionBackHome}})
	return menu
}

func newSummaryModeMenu(groupID int64) Menu {
	return Menu{
		{{Text: "Новые (непрочитанные)", Data: fmt.Sprintf("newsum:mode:%d:%s", groupID, domain.CollectionModeNewOnly)}},
		{{Text: "24 часа", Data: fmt.Sprintf("newsum:mode:%d:%s", groupID, domain.CollectionModeLast24H)}, {Text: "3 дня", Data: fmt.Sprintf("newsum:mode:%d:%s", groupID, domain.CollectionModeLast3D)}},
		{{Text: "Неделя", Data: fmt.Sprintf("newsum:mode:%d:%s", groupID, domain.CollectionModeWeek)}},
		{{Text: "Назад", Data: ActionNewSummary}},
	}
}

func collectionResultMenu(result *collection.Result) Menu {
	if result == nil || result.JobID <= 0 {
		return BackMenu()
	}
	return Menu{
		{{Text: "Создать сводку", Data: fmt.Sprintf("newsum:generate:%d", result.JobID)}},
		{{Text: "Другой период", Data: ActionNewSummary}, {Text: "Назад", Data: ActionBackHome}},
	}
}

func summaryResultMenu(result *summary.GenerateResult) Menu {
	if result == nil || result.SummaryID <= 0 {
		return BackMenu()
	}
	return Menu{
		{{Text: "Открыть сводку", Data: fmt.Sprintf("sum:open:%d", result.SummaryID)}},
		{{Text: "История", Data: "sum:list"}, {Text: "Назад", Data: ActionBackHome}},
	}
}
