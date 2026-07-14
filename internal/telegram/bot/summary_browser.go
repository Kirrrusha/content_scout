package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/summary"
)

type SummaryBrowserController interface {
	ListSummaries(ctx context.Context, telegramUserID int64, limit int) ([]domain.Summary, error)
	GetSummary(ctx context.Context, telegramUserID, summaryID int64) (*domain.Summary, error)
	ListTopics(ctx context.Context, telegramUserID, summaryID int64) ([]domain.SummaryTopic, error)
	TopicCard(ctx context.Context, telegramUserID, summaryID int64, position int) (*summary.TopicCard, error)
}

func (r *Router) showSummaries(ctx context.Context, chatID, userID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.browser == nil {
		return Outgoing{ChatID: chatID, Text: "История summary пока не настроена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	items, err := r.browser.ListSummaries(ctx, userID, 10)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err := r.states.Set(ctx, userID, DialogState{View: ViewHistory}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state: %w", err)
	}
	if len(items) == 0 {
		return Outgoing{ChatID: chatID, Text: "История summary пуста.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return Outgoing{
		ChatID:         chatID,
		Text:           summariesListText(items),
		Menu:           summariesMenu(items),
		EditMessageID:  editMessageID,
		AnswerCallback: callbackAnswer,
	}, nil
}

func (r *Router) showSummary(ctx context.Context, chatID, userID int64, args string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	summaryID, err := parseRequiredInt64(args)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: "Использование: /summary <summary_id>", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return r.renderSummary(ctx, chatID, userID, summaryID, editMessageID, callbackAnswer)
}

func (r *Router) showSummaryTopics(ctx context.Context, chatID, userID int64, args string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	summaryID, err := parseRequiredInt64(args)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: "Использование: /summary_topics <summary_id>", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return r.renderTopic(ctx, chatID, userID, summaryID, 1, editMessageID, callbackAnswer)
}

func (r *Router) showTopic(ctx context.Context, chatID, userID int64, args string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	fields := strings.Fields(args)
	if len(fields) < 2 {
		return Outgoing{ChatID: chatID, Text: "Использование: /topic <summary_id> <position>", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	summaryID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || summaryID <= 0 {
		return Outgoing{ChatID: chatID, Text: "summary_id должен быть положительным числом.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	position, err := strconv.Atoi(fields[1])
	if err != nil || position <= 0 {
		return Outgoing{ChatID: chatID, Text: "position должен быть положительным числом.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return r.renderTopic(ctx, chatID, userID, summaryID, position, editMessageID, callbackAnswer)
}

func (r *Router) handleSummaryCallback(ctx context.Context, in Incoming) (Outgoing, error) {
	fields := strings.Split(in.CallbackData, ":")
	if len(fields) < 2 {
		return Outgoing{ChatID: in.ChatID, Text: "Неизвестное действие.", AnswerCallback: "Неизвестное действие."}, nil
	}
	switch fields[1] {
	case "list":
		return r.showSummaries(ctx, in.ChatID, in.UserID, in.CallbackMessage, "История открыта.")
	case "open":
		if len(fields) != 3 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестное действие.", AnswerCallback: "Неизвестное действие."}, nil
		}
		summaryID, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || summaryID <= 0 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестное summary.", AnswerCallback: "Неизвестное summary."}, nil
		}
		return r.renderSummary(ctx, in.ChatID, in.UserID, summaryID, in.CallbackMessage, "Сводка открыта.")
	case "topic":
		if len(fields) != 4 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестное действие.", AnswerCallback: "Неизвестное действие."}, nil
		}
		summaryID, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || summaryID <= 0 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестное summary.", AnswerCallback: "Неизвестное summary."}, nil
		}
		position, err := strconv.Atoi(fields[3])
		if err != nil || position <= 0 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестная тема.", AnswerCallback: "Неизвестная тема."}, nil
		}
		return r.renderTopic(ctx, in.ChatID, in.UserID, summaryID, position, in.CallbackMessage, "Тема открыта.")
	default:
		return Outgoing{ChatID: in.ChatID, Text: "Неизвестное действие.", AnswerCallback: "Неизвестное действие."}, nil
	}
}

func (r *Router) renderSummary(ctx context.Context, chatID, userID, summaryID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.browser == nil {
		return Outgoing{ChatID: chatID, Text: "Просмотр сводок пока не настроен.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	item, err := r.browser.GetSummary(ctx, userID, summaryID)
	if errors.Is(err, summary.ErrSummaryNotFound) {
		return Outgoing{ChatID: chatID, Text: "Сводка не найдена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return Outgoing{
		ChatID:         chatID,
		Text:           summaryText(*item),
		Menu:           summaryMenu(item.ID),
		EditMessageID:  editMessageID,
		AnswerCallback: callbackAnswer,
	}, nil
}

func (r *Router) renderTopic(ctx context.Context, chatID, userID, summaryID int64, position int, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.browser == nil {
		return Outgoing{ChatID: chatID, Text: "История summary пока не настроена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	card, err := r.browser.TopicCard(ctx, userID, summaryID, position)
	if errors.Is(err, summary.ErrSummaryNotFound) {
		return Outgoing{ChatID: chatID, Text: "Summary не найдено.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if errors.Is(err, summary.ErrTopicNotFound) {
		return Outgoing{ChatID: chatID, Text: "У этого summary пока нет тем.", Menu: summaryMenu(summaryID), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return Outgoing{
		ChatID:         chatID,
		Text:           topicCardText(*card),
		Menu:           topicMenu(card.Summary.ID, card.Index, card.Total),
		EditMessageID:  editMessageID,
		AnswerCallback: callbackAnswer,
	}, nil
}

func summariesListText(items []domain.Summary) string {
	var b strings.Builder
	b.WriteString("История summary\n")
	for _, item := range items {
		fmt.Fprintf(&b, "\n#%d %s\nТем: %d | сообщений: %d | источников: %d", item.ID, fallbackTitle(item.Title), item.TopicsCount, item.MessagesCount, item.SourcesCount)
	}
	return b.String()
}

func summaryText(item domain.Summary) string {
	return fmt.Sprintf("Сводка #%d\n\n%s\n\n%s\n\nТем: %d\nСообщений: %d\nИсточников: %d", item.ID, fallbackTitle(item.Title), fallbackTitle(item.Overview), item.TopicsCount, item.MessagesCount, item.SourcesCount)
}

func topicCardText(card summary.TopicCard) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n%s\n\n%s", fallbackTitle(card.Topic.Title), fallbackTitle(card.Topic.ShortSummary), fallbackTitle(card.Topic.FullSummary))
	if sources := topicSourcesText(card.Topic.Sources); sources != "" {
		fmt.Fprintf(&b, "\n\nКаналы:\n%s", sources)
	}
	if messages := topicMessagesText(card.Topic.Messages); messages != "" {
		fmt.Fprintf(&b, "\n\nСообщения:\n%s", messages)
	}
	fmt.Fprintf(&b, "\n\nТема %d/%d | важность: %d | confidence: %s | сообщений: %d", card.Index, card.Total, card.Topic.Importance, card.Topic.Confidence, card.Topic.MessagesCount)
	return b.String()
}

func topicSourcesText(sources []domain.SummaryTopicSource) string {
	if len(sources) == 0 {
		return ""
	}
	const limit = 5
	lines := make([]string, 0, min(len(sources), limit))
	for i, source := range sources {
		if i >= limit {
			break
		}
		title := fallbackTitle(source.Title)
		username := strings.TrimPrefix(strings.TrimSpace(stringValue(source.Username)), "@")
		if username == "" {
			lines = append(lines, "- "+title)
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: https://t.me/%s", title, username))
	}
	if len(sources) > limit {
		lines = append(lines, fmt.Sprintf("- ещё %d", len(sources)-limit))
	}
	return strings.Join(lines, "\n")
}

func topicMessagesText(messages []domain.SummaryTopicMessage) string {
	if len(messages) == 0 {
		return ""
	}
	const limit = 6
	lines := make([]string, 0, min(len(messages), limit))
	for i, message := range messages {
		if i >= limit {
			break
		}
		title := fallbackTitle(message.SourceTitle)
		if strings.TrimSpace(message.SourceURL) == "" {
			lines = append(lines, fmt.Sprintf("- %s", title))
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", title, message.SourceURL))
	}
	if len(messages) > limit {
		lines = append(lines, fmt.Sprintf("- ещё %d", len(messages)-limit))
	}
	return strings.Join(lines, "\n")
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func summariesMenu(items []domain.Summary) Menu {
	menu := make(Menu, 0, len(items)+1)
	for _, item := range items {
		menu = append(menu, []MenuButton{{Text: fmt.Sprintf("#%d %s", item.ID, compactButtonTitle(item.Title)), Data: fmt.Sprintf("sum:open:%d", item.ID)}})
	}
	menu = append(menu, []MenuButton{{Text: "Назад", Data: ActionBackHome}})
	return menu
}

func summaryMenu(summaryID int64) Menu {
	return Menu{
		{{Text: "Темы", Data: fmt.Sprintf("sum:topic:%d:1", summaryID)}},
		{{Text: "Сделать статью", Data: fmt.Sprintf("art:summary:%d", summaryID)}, {Text: "Экспорт", Data: fmt.Sprintf("exp:summary:%d", summaryID)}},
		{{Text: "К истории", Data: "sum:list"}, {Text: "Назад", Data: ActionBackHome}},
	}
}

func topicMenu(summaryID int64, index, total int) Menu {
	if total <= 0 {
		return summaryMenu(summaryID)
	}
	prev := index - 1
	if prev < 1 {
		prev = total
	}
	next := index + 1
	if next > total {
		next = 1
	}
	return Menu{
		{{Text: "Назад", Data: fmt.Sprintf("sum:topic:%d:%d", summaryID, prev)}, {Text: "Вперёд", Data: fmt.Sprintf("sum:topic:%d:%d", summaryID, next)}},
		{{Text: "Сделать статью", Data: fmt.Sprintf("art:topic:%d:%d", summaryID, index)}},
		{{Text: "Summary", Data: fmt.Sprintf("sum:open:%d", summaryID)}, {Text: "К истории", Data: "sum:list"}},
	}
}

func parseRequiredInt64(args string) (int64, error) {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return 0, fmt.Errorf("missing id")
	}
	value, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return value, nil
}

func fallbackTitle(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Без названия"
	}
	return value
}

func compactButtonTitle(value string) string {
	value = fallbackTitle(value)
	const limit = 32
	if len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit-1]) + "…"
}
