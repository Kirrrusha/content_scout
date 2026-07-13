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
	text := "Расписания не настроены.\n\nСоздайте: /schedule_create <group_id> <HH:MM> [timezone] [export:true|false]"
	if len(items) > 0 {
		var b strings.Builder
		b.WriteString("Расписания:\n")
		for _, item := range items {
			b.WriteString(scheduleLine(item))
			b.WriteByte('\n')
		}
		b.WriteString("\nКоманды: /schedule <id>, /schedule_run <id>, /schedule_enable <id>, /schedule_disable <id>, /schedule_delete <id>")
		text = b.String()
	}
	if err := r.states.Set(ctx, userID, DialogState{View: ViewSchedules}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state: %w", err)
	}
	return Outgoing{ChatID: chatID, Text: text, Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
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
	return Outgoing{ChatID: chatID, Text: "Расписание создано:\n" + scheduleLine(*item), Menu: BackMenu()}, nil
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
	return Outgoing{ChatID: chatID, Text: text, Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
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
	return Outgoing{ChatID: chatID, Text: "Расписание обновлено:\n" + scheduleLine(*item), Menu: BackMenu()}, nil
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
