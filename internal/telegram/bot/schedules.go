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
		groupNames := make(map[int64]string)
		if r.groups != nil {
			if groups, groupErr := r.groups.List(ctx, userID); groupErr == nil {
				for _, group := range groups {
					groupNames[group.ID] = group.Name
				}
			}
		}
		text = scheduleListText(items, groupNames)
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
		return Outgoing{ChatID: in.ChatID, Text: "Сколько сводок создавать в день? При необходимости сначала добавьте открытый канал.", Menu: scheduleCountMenu(groupID), EditMessageID: in.CallbackMessage, AnswerCallback: "Группа выбрана."}, nil
	case "addpublic":
		if len(fields) != 3 {
			return unknownCallback(in), nil
		}
		groupID, ok := parseCallbackID(fields[2])
		if !ok {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестная группа.", AnswerCallback: "Неизвестная группа."}, nil
		}
		return r.promptPublicChannel(ctx, in.ChatID, in.UserID, groupID, "schedule", in.CallbackMessage, "Введите адрес канала.")
	case "count":
		if len(fields) != 4 {
			return unknownCallback(in), nil
		}
		groupID, ok := parseCallbackID(fields[2])
		count, countOK := parseScheduleCount(fields[3])
		if !ok || !countOK {
			return Outgoing{ChatID: in.ChatID, Text: "Некорректные параметры расписания.", AnswerCallback: "Некорректные параметры."}, nil
		}
		return Outgoing{ChatID: in.ChatID, Text: scheduleTimePrompt(count, nil), Menu: scheduleTimeMenu(groupID, count, nil), EditMessageID: in.CallbackMessage, AnswerCallback: "Количество выбрано."}, nil
	case "time":
		if len(fields) != 5 {
			return unknownCallback(in), nil
		}
		groupID, ok := parseCallbackID(fields[2])
		count, countOK := parseScheduleCount(fields[3])
		selected, timesOK := parseCallbackTimes(fields[4])
		if !ok || !countOK || !timesOK || len(selected) == 0 || len(selected) > count {
			return Outgoing{ChatID: in.ChatID, Text: "Некорректные параметры расписания.", AnswerCallback: "Некорректные параметры."}, nil
		}
		if len(selected) < count {
			return Outgoing{ChatID: in.ChatID, Text: scheduleTimePrompt(count, selected), Menu: scheduleTimeMenu(groupID, count, selected), EditMessageID: in.CallbackMessage, AnswerCallback: "Время добавлено."}, nil
		}
		return r.createSchedulesFromButtons(ctx, in.ChatID, in.UserID, groupID, selected, in.CallbackMessage, "Создаю расписания.")
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
	return r.createSchedulesFromButtons(ctx, chatID, userID, groupID, []string{scheduleTime}, editMessageID, callbackAnswer)
}

func (r *Router) createSchedulesFromButtons(ctx context.Context, chatID, userID, groupID int64, scheduleTimes []string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.schedules == nil {
		return Outgoing{ChatID: chatID, Text: "Расписания пока не настроены.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	created := make([]domain.SummarySchedule, 0, len(scheduleTimes))
	for _, scheduleTime := range scheduleTimes {
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
			for _, rollback := range created {
				_ = r.schedules.Delete(ctx, userID, rollback.ID)
			}
			return Outgoing{ChatID: chatID, Text: publicScheduleError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
		}
		created = append(created, *item)
	}
	if len(created) == 1 {
		return Outgoing{ChatID: chatID, Text: "Расписание создано:\n" + scheduleLine(created[0]), Menu: scheduleItemMenu(created[0]), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Создано расписаний: %d\n", len(created))
	for _, item := range created {
		fmt.Fprintf(&b, "\n• %s — включено", item.Cron)
	}
	return Outgoing{ChatID: chatID, Text: b.String(), Menu: scheduleListMenu(created), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func publicScheduleError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, tdlib.ErrUnauthorizedOwner) {
		return "Доступ запрещен."
	}
	if errors.Is(err, schedules.ErrScheduleAlreadyExists) {
		return "Для этой группы уже есть расписание на выбранное время. Выберите другое время."
	}
	return err.Error()
}

func parseScheduleID(args string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(args), 10, 64)
	return id, err == nil && id > 0
}

func scheduleLine(item domain.SummarySchedule) string {
	status := "выключено"
	if item.Enabled {
		status = "включено"
	}
	export := "без экспорта"
	if item.ExportToObsidian {
		export = "экспорт в Obsidian"
	}
	return fmt.Sprintf("#%d · группа %d · %s · %s · %s · %s", item.ID, item.GroupID, item.Cron, item.Timezone, status, export)
}

func scheduleListText(items []domain.SummarySchedule, groupNames map[int64]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Расписаний: %d\n", len(items))
	byGroup := make(map[int64][]domain.SummarySchedule)
	groupOrder := make([]int64, 0)
	for _, item := range items {
		if _, exists := byGroup[item.GroupID]; !exists {
			groupOrder = append(groupOrder, item.GroupID)
		}
		byGroup[item.GroupID] = append(byGroup[item.GroupID], item)
	}
	for _, groupID := range groupOrder {
		name := groupNames[groupID]
		if name == "" {
			name = fmt.Sprintf("Группа %d", groupID)
		}
		groupItems := byGroup[groupID]
		fmt.Fprintf(&b, "\n\n%s — расписаний в день: %d", name, len(groupItems))
		for _, item := range groupItems {
			status := "выключено"
			if item.Enabled {
				status = "включено"
			}
			fmt.Fprintf(&b, "\n• #%d · %s · %s", item.ID, item.Cron, status)
		}
	}
	return b.String()
}

func scheduleDetails(item domain.SummarySchedule, runs []domain.ScheduleRun) string {
	var b strings.Builder
	b.WriteString(scheduleLine(item))
	fmt.Fprintf(&b, "\nsummary_type=%s quiet=%s-%s", item.SummaryType, item.QuietHoursStart, item.QuietHoursEnd)
	if len(runs) == 0 {
		b.WriteString("\n\nЗапусков пока нет.")
		return b.String()
	}
	b.WriteString("\n\nПоследние запуски:")
	for _, run := range runs {
		fmt.Fprintf(&b, "\n#%d %s", run.ID, run.Status)
		if run.Error != nil {
			b.WriteString(": " + *run.Error)
		}
	}
	return b.String()
}

func scheduleListMenu(items []domain.SummarySchedule) Menu {
	menu := Menu{{{Text: "Создать расписание", Data: ActionScheduleNew}}}
	for _, item := range items {
		menu = append(menu, []MenuButton{{Text: fmt.Sprintf("#%d · %s", item.ID, item.Cron), Data: fmt.Sprintf("sched:open:%d", item.ID)}, {Text: "Запустить", Data: fmt.Sprintf("sched:run:%d", item.ID)}})
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

func scheduleCountMenu(groupID int64) Menu {
	return Menu{
		{{Text: "1 раз", Data: fmt.Sprintf("sched:count:%d:1", groupID)}, {Text: "2 раза", Data: fmt.Sprintf("sched:count:%d:2", groupID)}, {Text: "3 раза", Data: fmt.Sprintf("sched:count:%d:3", groupID)}},
		{{Text: "Добавить открытый канал", Data: fmt.Sprintf("sched:addpublic:%d", groupID)}},
		{{Text: "Назад", Data: ActionScheduleNew}},
	}
}

var scheduleTimeOptions = []string{"06:00", "09:00", "12:00", "15:00", "18:00", "21:00"}

func scheduleTimeMenu(groupID int64, count int, selected []string) Menu {
	selectedSet := make(map[string]bool, len(selected))
	for _, value := range selected {
		selectedSet[value] = true
	}
	menu := make(Menu, 0, 4)
	row := make([]MenuButton, 0, 3)
	for _, value := range scheduleTimeOptions {
		if selectedSet[value] {
			continue
		}
		times := append(append([]string(nil), selected...), value)
		row = append(row, MenuButton{Text: value, Data: fmt.Sprintf("sched:time:%d:%d:%s", groupID, count, encodeCallbackTimes(times))})
		if len(row) == 3 {
			menu = append(menu, row)
			row = make([]MenuButton, 0, 3)
		}
	}
	if len(row) > 0 {
		menu = append(menu, row)
	}
	menu = append(menu, []MenuButton{{Text: "Назад", Data: fmt.Sprintf("sched:group:%d", groupID)}})
	return menu
}

func scheduleTimePrompt(count int, selected []string) string {
	remaining := count - len(selected)
	if len(selected) == 0 {
		return fmt.Sprintf("Выберите время. Нужно выбрать: %d.", count)
	}
	return fmt.Sprintf("Выбрано: %s. Осталось выбрать: %d.", strings.Join(selected, ", "), remaining)
}

func parseScheduleCount(value string) (int, bool) {
	count, err := strconv.Atoi(value)
	return count, err == nil && count >= 1 && count <= 3
}

func encodeCallbackTimes(values []string) string {
	encoded := make([]string, 0, len(values))
	for _, value := range values {
		encoded = append(encoded, strings.ReplaceAll(value, ":", ""))
	}
	return strings.Join(encoded, ",")
}

func parseCallbackTimes(value string) ([]string, bool) {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		parsed := callbackTime(part)
		if seen[parsed] || !containsScheduleTime(parsed) {
			return nil, false
		}
		seen[parsed] = true
		result = append(result, parsed)
	}
	return result, true
}

func containsScheduleTime(value string) bool {
	for _, option := range scheduleTimeOptions {
		if option == value {
			return true
		}
	}
	return false
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
