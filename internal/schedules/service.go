package schedules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/storage"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

var ErrScheduleNotFound = errors.New("summary schedule not found")

type Request struct {
	TelegramUserID   int64
	GroupID          int64
	Time             string
	Timezone         string
	QuietHoursStart  string
	QuietHoursEnd    string
	SummaryType      string
	ExportToObsidian bool
	ExportProvided   bool
	Enabled          bool
	EnabledProvided  bool
}

type Service struct {
	ownerTelegramID int64
	users           storage.UserRepository
	schedules       storage.SummaryScheduleRepository
	queue           storage.JobRepository
	now             func() time.Time
}

func NewService(ownerTelegramID int64, users storage.UserRepository, schedules storage.SummaryScheduleRepository, queue storage.JobRepository) *Service {
	return &Service{ownerTelegramID: ownerTelegramID, users: users, schedules: schedules, queue: queue, now: time.Now}
}

func (s *Service) List(ctx context.Context, telegramUserID int64) ([]domain.SummarySchedule, error) {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	return s.schedules.ListByUser(ctx, user.ID)
}

func (s *Service) Get(ctx context.Context, telegramUserID, scheduleID int64) (*domain.SummarySchedule, error) {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	return s.requireSchedule(ctx, user.ID, scheduleID)
}

func (s *Service) Create(ctx context.Context, req Request) (*domain.SummarySchedule, error) {
	user, err := s.ownerUser(ctx, req.TelegramUserID)
	if err != nil {
		return nil, err
	}
	schedule, err := scheduleFromRequest(user.ID, 0, req, true)
	if err != nil {
		return nil, err
	}
	return s.schedules.Create(ctx, schedule)
}

func (s *Service) Update(ctx context.Context, scheduleID int64, req Request) (*domain.SummarySchedule, error) {
	user, err := s.ownerUser(ctx, req.TelegramUserID)
	if err != nil {
		return nil, err
	}
	current, err := s.requireSchedule(ctx, user.ID, scheduleID)
	if err != nil {
		return nil, err
	}
	updated := *current
	if req.GroupID != 0 {
		updated.GroupID = req.GroupID
	}
	if strings.TrimSpace(req.Time) != "" {
		updated.Cron = req.Time
	}
	if strings.TrimSpace(req.Timezone) != "" {
		updated.Timezone = req.Timezone
	}
	if strings.TrimSpace(req.SummaryType) != "" {
		updated.SummaryType = req.SummaryType
	}
	if req.EnabledProvided {
		updated.Enabled = req.Enabled
	}
	if strings.TrimSpace(req.QuietHoursStart) != "" {
		updated.QuietHoursStart = req.QuietHoursStart
	}
	if strings.TrimSpace(req.QuietHoursEnd) != "" {
		updated.QuietHoursEnd = req.QuietHoursEnd
	}
	if req.ExportProvided {
		updated.ExportToObsidian = req.ExportToObsidian
	}
	if err := validateSchedule(updated); err != nil {
		return nil, err
	}
	return s.schedules.Update(ctx, updated)
}

func (s *Service) Delete(ctx context.Context, telegramUserID, scheduleID int64) error {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return err
	}
	if _, err := s.requireSchedule(ctx, user.ID, scheduleID); err != nil {
		return err
	}
	return s.schedules.Delete(ctx, user.ID, scheduleID)
}

func (s *Service) SetEnabled(ctx context.Context, telegramUserID, scheduleID int64, enabled bool) (*domain.SummarySchedule, error) {
	current, err := s.Get(ctx, telegramUserID, scheduleID)
	if err != nil {
		return nil, err
	}
	current.Enabled = enabled
	return s.schedules.Update(ctx, *current)
}

func (s *Service) Run(ctx context.Context, telegramUserID, scheduleID int64) (*domain.Job, error) {
	schedule, err := s.Get(ctx, telegramUserID, scheduleID)
	if err != nil {
		return nil, err
	}
	if s.queue == nil {
		return nil, errors.New("job queue is not configured")
	}
	payload, err := json.Marshal(domain.JobPayloadScheduledPipeline{Schedule: *schedule})
	if err != nil {
		return nil, fmt.Errorf("marshal scheduled pipeline payload: %w", err)
	}
	key := fmt.Sprintf("scheduled_pipeline:manual:%d:%d", schedule.ID, s.now().Unix())
	return s.queue.Enqueue(ctx, domain.Job{
		Type:             domain.JobTypeScheduledPipeline,
		Status:           domain.JobStatusPending,
		Payload:          payload,
		MaxAttempts:      4,
		AvailableAt:      s.now(),
		DeduplicationKey: &key,
	})
}

func (s *Service) ListRuns(ctx context.Context, telegramUserID, scheduleID int64, limit int) ([]domain.ScheduleRun, error) {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireSchedule(ctx, user.ID, scheduleID); err != nil {
		return nil, err
	}
	return s.schedules.ListRuns(ctx, scheduleID, limit)
}

func (s *Service) ownerUser(ctx context.Context, telegramUserID int64) (*domain.User, error) {
	if s.ownerTelegramID == 0 || telegramUserID != s.ownerTelegramID {
		return nil, tdlib.ErrUnauthorizedOwner
	}
	user, err := s.users.FindByTelegramID(ctx, telegramUserID)
	if err != nil {
		return nil, fmt.Errorf("find owner user: %w", err)
	}
	if user == nil {
		return nil, errors.New("owner user is not initialized")
	}
	return user, nil
}

func (s *Service) requireSchedule(ctx context.Context, userID, scheduleID int64) (*domain.SummarySchedule, error) {
	schedule, err := s.schedules.FindByUser(ctx, userID, scheduleID)
	if err != nil {
		return nil, err
	}
	if schedule == nil {
		return nil, ErrScheduleNotFound
	}
	return schedule, nil
}

func scheduleFromRequest(userID, id int64, req Request, creating bool) (domain.SummarySchedule, error) {
	schedule := domain.SummarySchedule{
		ID:               id,
		UserID:           userID,
		GroupID:          req.GroupID,
		Cron:             strings.TrimSpace(req.Time),
		Timezone:         strings.TrimSpace(req.Timezone),
		Enabled:          req.Enabled,
		SummaryType:      strings.TrimSpace(req.SummaryType),
		QuietHoursStart:  strings.TrimSpace(req.QuietHoursStart),
		QuietHoursEnd:    strings.TrimSpace(req.QuietHoursEnd),
		ExportToObsidian: req.ExportToObsidian,
	}
	if creating && !req.EnabledProvided {
		schedule.Enabled = true
	}
	if schedule.Timezone == "" {
		schedule.Timezone = "UTC"
	}
	if schedule.SummaryType == "" {
		schedule.SummaryType = "standard"
	}
	if err := validateSchedule(schedule); err != nil {
		return domain.SummarySchedule{}, err
	}
	return schedule, nil
}

func validateSchedule(schedule domain.SummarySchedule) error {
	if schedule.GroupID <= 0 {
		return errors.New("source_group_id is required")
	}
	if !validTime(schedule.Cron) {
		return errors.New("time must be HH:MM")
	}
	if _, err := time.LoadLocation(schedule.Timezone); err != nil {
		return errors.New("timezone is invalid")
	}
	if schedule.QuietHoursStart != "" && !validTime(schedule.QuietHoursStart) {
		return errors.New("quiet_hours_start must be HH:MM")
	}
	if schedule.QuietHoursEnd != "" && !validTime(schedule.QuietHoursEnd) {
		return errors.New("quiet_hours_end must be HH:MM")
	}
	switch schedule.SummaryType {
	case "short", "standard", "detailed":
	default:
		return errors.New("summary_type must be short, standard, or detailed")
	}
	return nil
}

func validTime(value string) bool {
	_, err := time.Parse("15:04", value)
	return err == nil
}
