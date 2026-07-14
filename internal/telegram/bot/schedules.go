package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/schedules"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

func (r *Router) showSchedules(ctx context.Context, chatID, userID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.schedules == nil {
		return Outgoing{ChatID: chatID, Text: "Расписания пока не настроены.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	items, err := r.schedules.List(ctx, userID)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicScheduleError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	text := "Расписания не настроены."
	menu := scheduleListMenu(nil)
	if len(items) > 0 {
		var b strings.Builder
		b.WriteString("Расписания:\n")
		for _, item := range items {
			b.WriteString(scheduleLine(item))
			b.WriteByte('\n')
		}
		text = b.String()
		menu = scheduleListMenu(items)
	}
	if err := r.states.Set(ctx, userID, DialogState{View: ViewSchedules}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state: %w", err)
	}
	return Outgoing{ChatID: chatID, Text: text, Menu: menu, EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func (r *Router) createSchedule(ctx context.Context, chatID, userID int64, args string) (Outgoing, error) {
	if r.schedules == nil {
		return Outgoing{ChatID: chatID, Text: "Расписания пока не настроены.", Menu: BackMenu()}, nil
	}
	fields := strings.Fields(args)
	if len(fields) < 2 {
		return Outgoing{ChatID: chatID, Text: "Формат: /schedule_create <group_id> <HH:MM> [timezone] [export:true|false]", Menu: BackMenu()}, nil
	}
	groupID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || groupID <= 0 {
		return Outgoing{ChatID: chatID, Text: "group_id должен быть числом.", Menu: BackMenu()}, nil
	}
	timezone := "UTC"
	if len(fields) >= 3 {
		timezone = fields[2]
	}
	export := false
	if len(fields) >= 4 {
		export = fields[3] == "true" || fields[3] == "export:true" || fields[3] == "yes"
	}
	item, err := r.schedules.Create(ctx, schedules.Request{
		TelegramUserID:   userID,
		GroupID:          groupID,
		Time:             fields[1],
		Timezone:         timezone,
		SummaryType:      "standard",
		ExportToObsidian: export,
		ExportProvided:   true,
		Enabled:          true,
		EnabledProvided:  true,
	})
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicScheduleError(err), Menu: BackMenu()}, nil
	}
	return Outgoing{ChatID: chatID, Text: "Расписание создано:\n" + scheduleLine(*item), Menu: scheduleItemMenu(*item)}, nil
}

func (r *Router) showSchedule(ctx context.Context, chatID, userID int64, args string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.schedules == nil {
		return Outgoing{ChatID: chatID, Text: "Расписания пока не настроены.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	id, ok := parseScheduleID(args)
	if !ok {
		return Outgoing{ChatID: chatID, Text: "Формат: /schedule <id>", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	item, err := r.schedules.Get(ctx, userID, id)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicScheduleError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	runs, _ := r.schedules.ListRuns(ctx, userID, id, 5)
	text := scheduleDetails(*item, runs)
	return Outgoing{ChatID: chatID, Text: text, Menu: scheduleItemMenu(*item), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func (r *Router) setScheduleEnabled(ctx context.Context, chatID, userID int64, args string, enabled bool) (Outgoing, error) {
	if r.schedules == nil {
		return Outgoing{ChatID: chatID, Text: "Расписания пока не настроены.", Menu: BackMenu()}, nil
	}
	id, ok := parseScheduleID(args)
	if !ok {
		return Outgoing{ChatID: chatID, Text: "Укажите id расписания.", Menu: BackMenu()}, nil
	}
	item, err := r.schedules.SetEnabled(ctx, userID, id, enabled)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicScheduleError(err), Menu: BackMenu()}, nil
	}
	return Outgoing{ChatID: chatID, Text: "Расписание обновлено:\n" + scheduleLine(*item), Menu: scheduleItemMenu(*item)}, nil
}

func (r *Router) deleteSchedule(ctx context.Context, chatID, userID int64, args string) (Outgoing, error) {
	if r.schedules == nil {
		return Outgoing{ChatID: chatID, Text: "Расписания пока не настроены.", Menu: BackMenu()}, nil
	}
	id, ok := parseScheduleID(args)
	if !ok {
		return Outgoing{ChatID: chatID, Text: "Формат: /schedule_delete <id>", Menu: BackMenu()}, nil
	}
	if err := r.schedules.Delete(ctx, userID, id); err != nil {
		return Outgoing{ChatID: chatID, Text: publicScheduleError(err), Menu: BackMenu()}, nil
	}
	return Outgoing{ChatID: chatID, Text: "Расписание удалено.", Menu: BackMenu()}, nil
}

func (r *Router) runSchedule(ctx context.Context, chatID, userID int64, args string) (Outgoing, error) {
	if r.schedules == nil {
		return Outgoing{ChatID: chatID, Text: "Расписания пока не настроены.", Menu: BackMenu()}, nil
	}
	id, ok := parseScheduleID(args)
	if !ok {
		return Outgoing{ChatID: chatID, Text: "Формат: /schedule_run <id>", Menu: BackMenu()}, nil
	}
	job, err := r.schedules.Run(ctx, userID, id)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicScheduleError(err), Menu: BackMenu()}, nil
	}
	return Outgoing{ChatID: chatID, Text: fmt.Sprintf("Запуск поставлен в очередь. job_id=%d status=%s", job.ID, job.Status), Menu: BackMenu()}, nil
}

func (r *Router) handleScheduleCallback(ctx context.Context, in Incoming) (Outgoing, error) {
	fields := strings.Split(in.CallbackData, ":")
	if len(fields) < 2 {
		return unknownCallback(in), nil
	}
	switch fields[1] {
	case "new":
		return r.showScheduleGroupPicker(ctx, in.ChatID, in.UserID, in.CallbackMessage, "Создание расписания.")
	case "group":
		if len(fields) != 3 {
			return unknownCallback(in), nil
		}
		groupID, ok := parseCallbackID(fields[2])
		if !ok {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестная группа.", AnswerCallback: "Неизвестная группа."}, nil
		}
		return Outgoing{ChatID: in.ChatID, Text: "Выберите время ежедневной сводки.", Menu: scheduleTimeMenu(groupID), EditMessageID: in.CallbackMessage, AnswerCallback: "Время расписания."}, nil
	case "create":
		if len(fields) != 4 {
			return unknownCallback(in), nil
		}
		groupID, ok := parseCallbackID(fields[2])
		if !ok {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестная группа.", AnswerCallback: "Неизвестная группа."}, nil
		}
		return r.createScheduleFromButton(ctx, in.ChatID, in.UserID, groupID, callbackTime(fields[3]), in.CallbackMessage, "Создаю расписание.")
	case "open":
		if len(fields) != 3 {
			return unknownCallback(in), nil
		}
		id, ok := parseCallbackID(fields[2])
		if !ok {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестное расписание.", AnswerCallback: "Неизвестное расписание."}, nil
		}
		return r.showSchedule(ctx, in.ChatID, in.UserID, strconv.FormatInt(id, 10), in.CallbackMessage, "Расписание открыто.")
	case "run":
		if len(fields) != 3 {
			return unknownCallback(in), nil
		}
		id, ok := parseCallbackID(fields[2])
		if !ok {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестное расписание.", AnswerCallback: "Неизвестное расписание."}, nil
		}
		return r.runSchedule(ctx, in.ChatID, in.UserID, strconv.FormatInt(id, 10))
	case "enable", "disable":
		if len(fields) != 3 {
			return unknownCallback(in), nil
		}
		id, ok := parseCallbackID(fields[2])
		if !ok {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестное расписание.", AnswerCallback: "Неизвестное расписание."}, nil
		}
		return r.setScheduleEnabled(ctx, in.ChatID, in.UserID, strconv.FormatInt(id, 10), fields[1] == "enable")
	case "delete":
		if len(fields) != 3 {
			return unknownCallback(in), nil
		}
		id, ok := parseCallbackID(fields[2])
		if !ok {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестное расписание.", AnswerCallback: "Неизвестное расписание."}, nil
		}
		return r.deleteSchedule(ctx, in.ChatID, in.UserID, strconv.FormatInt(id, 10))
	default:
		return unknownCallback(in), nil
	}
}

func (r *Router) showScheduleGroupPicker(ctx context.Context, chatID, userID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.groups == nil {
		return Outgoing{ChatID: chatID, Text: "Группы источников пока не настроены.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	groups, err := r.groups.List(ctx, userID)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicGroupError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if len(groups) == 0 {
		return Outgoing{ChatID: chatID, Text: "Сначала создайте группу источников.", Menu: Menu{{{Text: "Мои группы", Data: ActionGroups}}, {{Text: "Назад", Data: ActionSchedules}}}, EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return Outgoing{ChatID: chatID, Text: "Выберите группу источников для расписания.", Menu: scheduleGroupsMenu(groups), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func (r *Router) createScheduleFromButton(ctx context.Context, chatID, userID, groupID int64, scheduleTime string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.schedules == nil {
		return Outgoing{ChatID: chatID, Text: "Расписания пока не настроены.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	item, err := r.schedules.Create(ctx, schedules.Request{
		TelegramUserID:  userID,
		GroupID:         groupID,
		Time:            scheduleTime,
		Timezone:        "Europe/Moscow",
		SummaryType:     "standard",
		Enabled:         true,
		EnabledProvided: true,
	})
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicScheduleError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return Outgoing{ChatID: chatID, Text: "Расписание создано:\n" + scheduleLine(*item), Menu: scheduleItemMenu(*item), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func publicScheduleError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, tdlib.ErrUnauthorizedOwner) {
		return "Доступ запрещен."
	}
	return err.Error()
}

func parseScheduleID(args string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(args), 10, 64)
	return id, err == nil && id > 0
}

func scheduleLine(item domain.SummarySchedule) string {
	status := "disabled"
	if item.Enabled {
		status = "enabled"
	}
	export := "no export"
	if item.ExportToObsidian {
		export = "export"
	}
	return fmt.Sprintf("#%d group=%d time=%s tz=%s %s %s", item.ID, item.GroupID, item.Cron, item.Timezone, status, export)
}

func scheduleDetails(item domain.SummarySchedule, runs []domain.ScheduleRun) string {
	var b strings.Builder
	b.WriteString(scheduleLine(item))
	b.WriteString(fmt.Sprintf("\nsummary_type=%s quiet=%s-%s", item.SummaryType, item.QuietHoursStart, item.QuietHoursEnd))
	if len(runs) == 0 {
		b.WriteString("\n\nЗапусков пока нет.")
		return b.String()
	}
	b.WriteString("\n\nПоследние запуски:")
	for _, run := range runs {
		b.WriteString(fmt.Sprintf("\n#%d %s", run.ID, run.Status))
		if run.Error != nil {
			b.WriteString(": " + *run.Error)
		}
	}
	return b.String()
}

func scheduleListMenu(items []domain.SummarySchedule) Menu {
	menu := Menu{{{Text: "Создать расписание", Data: ActionScheduleNew}}}
	for _, item := range items {
		menu = append(menu, []MenuButton{{Text: fmt.Sprintf("#%d", item.ID), Data: fmt.Sprintf("sched:open:%d", item.ID)}, {Text: "Запустить", Data: fmt.Sprintf("sched:run:%d", item.ID)}})
	}
	menu = append(menu, []MenuButton{{Text: "Назад", Data: ActionBackHome}})
	return menu
}

func scheduleItemMenu(item domain.SummarySchedule) Menu {
	toggleText := "Выключить"
	toggleAction := "disable"
	if !item.Enabled {
		toggleText = "Включить"
		toggleAction = "enable"
	}
	return Menu{
		{{Text: "Запустить сейчас", Data: fmt.Sprintf("sched:run:%d", item.ID)}},
		{{Text: toggleText, Data: fmt.Sprintf("sched:%s:%d", toggleAction, item.ID)}, {Text: "Удалить", Data: fmt.Sprintf("sched:delete:%d", item.ID)}},
		{{Text: "Все расписания", Data: ActionSchedules}, {Text: "Назад", Data: ActionBackHome}},
	}
}

func scheduleGroupsMenu(groups []domain.SourceGroup) Menu {
	menu := make(Menu, 0, len(groups)+1)
	for _, group := range groups {
		menu = append(menu, []MenuButton{{Text: fmt.Sprintf("%d. %s", group.ID, compactButtonTitle(group.Name)), Data: fmt.Sprintf("sched:group:%d", group.ID)}})
	}
	menu = append(menu, []MenuButton{{Text: "Назад", Data: ActionSchedules}})
	return menu
}

func scheduleTimeMenu(groupID int64) Menu {
	return Menu{
		{{Text: "09:00", Data: fmt.Sprintf("sched:create:%d:0900", groupID)}, {Text: "12:00", Data: fmt.Sprintf("sched:create:%d:1200", groupID)}},
		{{Text: "18:00", Data: fmt.Sprintf("sched:create:%d:1800", groupID)}, {Text: "21:00", Data: fmt.Sprintf("sched:create:%d:2100", groupID)}},
		{{Text: "Назад", Data: ActionScheduleNew}},
	}
}

func callbackTime(value string) string {
	if len(value) == 4 {
		return value[:2] + ":" + value[2:]
	}
	return value
}

func parseCallbackID(value string) (int64, bool) {
	id, err := strconv.ParseInt(value, 10, 64)
	return id, err == nil && id > 0
}
