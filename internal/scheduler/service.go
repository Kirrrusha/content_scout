package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/obsidian"
	"github.com/kirilllebedenko/content_scout/internal/storage"
	"github.com/kirilllebedenko/content_scout/internal/summary"
)

type JobQueue interface {
	Enqueue(ctx context.Context, job domain.Job) (*domain.Job, error)
}

type Collector interface {
	CollectGroup(ctx context.Context, req collection.Request) (*collection.Result, error)
}

type Summarizer interface {
	GenerateFromCollection(ctx context.Context, req summary.GenerateRequest) (*summary.GenerateResult, error)
}

type Exporter interface {
	ExportSummary(ctx context.Context, telegramUserID, summaryID int64) (*obsidian.Result, error)
}

type Service struct {
	ownerTelegramID int64
	schedules       storage.SummaryScheduleRepository
	collector       Collector
	summarizer      Summarizer
	exporter        Exporter
	logger          *slog.Logger
	now             func() time.Time
}

func NewService(ownerTelegramID int64, schedules storage.SummaryScheduleRepository, collector Collector, summarizer Summarizer, exporter Exporter, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{ownerTelegramID: ownerTelegramID, schedules: schedules, collector: collector, summarizer: summarizer, exporter: exporter, logger: logger, now: time.Now}
}

func (s *Service) RunDue(ctx context.Context) (int, error) {
	items, err := s.schedules.ListEnabled(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	now := s.now()
	for _, schedule := range items {
		if !IsDue(schedule, now) {
			continue
		}
		if err := s.RunSchedule(ctx, schedule); err != nil {
			s.logger.Warn("scheduled summary failed", "schedule_id", schedule.ID, "error", err)
		}
		count++
	}
	return count, nil
}

func (s *Service) EnqueueDue(ctx context.Context, queue JobQueue) (int, error) {
	if queue == nil {
		return 0, fmt.Errorf("job queue is not configured")
	}
	items, err := s.schedules.ListEnabled(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	now := s.now()
	for _, schedule := range items {
		if !IsDue(schedule, now) {
			continue
		}
		payload, err := json.Marshal(domain.JobPayloadScheduledPipeline{Schedule: schedule})
		if err != nil {
			return count, fmt.Errorf("marshal scheduled pipeline payload: %w", err)
		}
		key := scheduleDeduplicationKey(schedule, now)
		if _, err := queue.Enqueue(ctx, domain.Job{
			Type:             domain.JobTypeScheduledPipeline,
			Status:           domain.JobStatusPending,
			Payload:          payload,
			MaxAttempts:      4,
			AvailableAt:      now,
			DeduplicationKey: &key,
		}); err != nil {
			return count, fmt.Errorf("enqueue scheduled pipeline: %w", err)
		}
		count++
	}
	return count, nil
}

func (s *Service) RunSchedule(ctx context.Context, schedule domain.SummarySchedule) error {
	run, err := s.schedules.CreateRun(ctx, domain.ScheduleRun{ScheduleID: schedule.ID, Status: domain.JobStatusProcessing})
	if err != nil {
		return err
	}
	var collectionJobID *int64
	var summaryID *int64
	var exportID *int64
	var runErr error
	defer func() {
		status := domain.JobStatusCompleted
		var message *string
		if runErr != nil {
			status = domain.JobStatusFailed
			text := runErr.Error()
			message = &text
		}
		_ = s.schedules.CompleteRun(ctx, run.ID, status, collectionJobID, summaryID, exportID, message)
		if runErr == nil {
			_ = s.schedules.MarkScheduleRun(ctx, schedule.ID, s.now())
		}
	}()

	if s.collector == nil || s.summarizer == nil {
		runErr = fmt.Errorf("scheduler runtime is not configured")
		return runErr
	}
	collected, err := s.collector.CollectGroup(ctx, collection.Request{
		TelegramUserID: s.ownerTelegramID,
		GroupID:        schedule.GroupID,
		Mode:           domain.CollectionModeNewOnly,
		Limit:          100,
	})
	if err != nil {
		runErr = err
		return runErr
	}
	collectionJobID = &collected.JobID
	if collected.MessagesCount == 0 {
		return nil
	}
	generated, err := s.summarizer.GenerateFromCollection(ctx, summary.GenerateRequest{
		TelegramUserID:  s.ownerTelegramID,
		CollectionJobID: collected.JobID,
		Format:          schedule.SummaryType,
	})
	if err != nil {
		runErr = err
		return runErr
	}
	summaryID = &generated.SummaryID
	if schedule.ExportToObsidian && s.exporter != nil {
		exported, err := s.exporter.ExportSummary(ctx, s.ownerTelegramID, generated.SummaryID)
		if err != nil {
			runErr = err
			return runErr
		}
		exportID = &exported.Export.ID
	}
	return nil
}

func IsDue(schedule domain.SummarySchedule, now time.Time) bool {
	if !schedule.Enabled {
		return false
	}
	location, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		location = time.UTC
	}
	localNow := now.In(location)
	if inQuietHours(localNow, schedule.QuietHoursStart, schedule.QuietHoursEnd) {
		return false
	}
	hour, minute, ok := parseDailyTime(schedule.Cron)
	if !ok {
		return false
	}
	target := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, minute, 0, 0, location)
	if localNow.Before(target) {
		return false
	}
	if schedule.LastRunAt == nil {
		return true
	}
	last := schedule.LastRunAt.In(location)
	return last.Year() != localNow.Year() || last.YearDay() != localNow.YearDay()
}

func parseDailyTime(value string) (int, int, bool) {
	value = strings.TrimSpace(value)
	if value == "@daily" {
		return 9, 0, true
	}
	if strings.HasPrefix(value, "daily@") {
		value = strings.TrimPrefix(value, "daily@")
	}
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, false
	}
	return hour, minute, true
}

func inQuietHours(now time.Time, start, end string) bool {
	startHour, startMinute, okStart := parseDailyTime(start)
	endHour, endMinute, okEnd := parseDailyTime(end)
	if !okStart || !okEnd {
		return false
	}
	minutes := now.Hour()*60 + now.Minute()
	startTotal := startHour*60 + startMinute
	endTotal := endHour*60 + endMinute
	if startTotal == endTotal {
		return false
	}
	if startTotal < endTotal {
		return minutes >= startTotal && minutes < endTotal
	}
	return minutes >= startTotal || minutes < endTotal
}

func scheduleDeduplicationKey(schedule domain.SummarySchedule, now time.Time) string {
	location, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		location = time.UTC
	}
	localNow := now.In(location)
	return fmt.Sprintf("scheduled_pipeline:%d:%04d-%02d-%02d", schedule.ID, localNow.Year(), localNow.Month(), localNow.Day())
}
