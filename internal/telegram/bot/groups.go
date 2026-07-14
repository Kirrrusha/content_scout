package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/sourcegroups"
)

type GroupController interface {
	Create(ctx context.Context, telegramUserID int64, name, description string) (*domain.SourceGroup, error)
	Update(ctx context.Context, telegramUserID, groupID int64, name, description string) (*domain.SourceGroup, error)
	Delete(ctx context.Context, telegramUserID, groupID int64) error
	List(ctx context.Context, telegramUserID int64) ([]domain.SourceGroup, error)
	AddChat(ctx context.Context, telegramUserID, groupID, chatID int64, priority int, enabled bool) error
	RemoveChat(ctx context.Context, telegramUserID, groupID, chatID int64) error
	ListChats(ctx context.Context, telegramUserID, groupID int64) (*sourcegroups.GroupWithChats, error)
}

func groupsText(groups []domain.SourceGroup) string {
	if len(groups) == 0 {
		return "Группы источников пока не созданы.\n\nВыполните /sync, чтобы подтянуть папки Telegram, или создайте группу вручную."
	}
	var builder strings.Builder
	builder.WriteString("Мои группы:\n\n")
	for _, group := range groups {
		description := ""
		if group.Description != "" {
			description = " — " + group.Description
		}
		fmt.Fprintf(&builder, "%d. %s%s\n", group.ID, group.Name, description)
	}
	builder.WriteString("\nОткройте группу, чтобы посмотреть источники.")
	return builder.String()
}

func groupChatsText(group *sourcegroups.GroupWithChats) string {
	if group == nil {
		return "Группа не найдена."
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "Группа: %s\n\n", group.Group.Name)
	if len(group.Chats) == 0 {
		builder.WriteString("В группе пока нет источников.\n\nОткройте /chats и добавьте нужные чаты в эту группу.")
		return builder.String()
	}
	linkByChatID := make(map[int64]domain.SourceGroupChat, len(group.Links))
	for _, link := range group.Links {
		linkByChatID[link.ChatID] = link
	}
	fmt.Fprintf(&builder, "Источников: %d\n\n", len(group.Chats))
	for index, chat := range group.Chats {
		link := linkByChatID[chat.ID]
		status := ""
		if !link.Enabled {
			status = " (выключен)"
		}
		fmt.Fprintf(&builder, "%d. %s%s\n", index+1, chat.Title, status)
	}
	builder.WriteString("\nМожно создать сводку по этой группе или вернуться к списку групп.")
	return builder.String()
}

func groupsMenu(groups []domain.SourceGroup) Menu {
	menu := make(Menu, 0, len(groups)+2)
	for _, group := range groups {
		menu = append(menu, []MenuButton{{Text: fmt.Sprintf("%d. %s", group.ID, compactButtonTitle(group.Name)), Data: fmt.Sprintf("groups:open:%d", group.ID)}})
	}
	menu = append(menu, []MenuButton{{Text: "Синхронизировать", Data: "groups:sync"}})
	menu = append(menu, []MenuButton{{Text: "Назад", Data: ActionBackHome}})
	return menu
}

func groupDetailsMenu(groupID int64) Menu {
	return Menu{
		{{Text: "Создать сводку", Data: fmt.Sprintf("newsum:group:%d", groupID)}},
		{{Text: "Все группы", Data: ActionGroups}, {Text: "Назад", Data: ActionBackHome}},
	}
}

func publicGroupError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, sourcegroups.ErrGroupNotFound):
		return "Группа источников не найдена или в ней нет чатов. Откройте /chats, затем добавьте источник командой /group_add_chat <group_id> <chat_id>."
	case errors.Is(err, sourcegroups.ErrChatNotFound):
		return "Чат не найден среди синхронизированных источников. Сначала выполните /sync и посмотрите /chats."
	default:
		message := err.Error()
		switch {
		case strings.Contains(message, sourcegroups.ErrGroupNotFound.Error()):
			return "Группа источников не найдена или в ней нет чатов. Откройте /chats, затем добавьте источник командой /group_add_chat <group_id> <chat_id>."
		case strings.Contains(message, sourcegroups.ErrChatNotFound.Error()):
			return "Чат не найден среди синхронизированных источников. Сначала выполните /sync и посмотрите /chats."
		default:
			return publicAuthError(err)
		}
	}
}

func parseID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}
