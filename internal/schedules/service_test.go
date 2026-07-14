package schedules

import (
	"context"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

func TestServiceCreatesScheduleWithDefaults(t *testing.T) {
	service, repo, _ := newTestService()

	item, err := service.Create(context.Background(), Request{
		TelegramUserID: 42,
		GroupID:        7,
		Time:           "09:00",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if item.ID == 0 || item.UserID != 1 || item.Timezone != "UTC" || item.SummaryType != "standard" || !item.Enabled {
		t.Fatalf("schedule = %+v", item)
	}
	if len(repo.items) != 1 {
		t.Fatalf("repo items = %d", len(repo.items))
	}
}

func TestServiceRejectsInvalidSchedule(t *testing.T) {
	service, _, _ := newTestService()

	_, err := service.Create(context.Background(), Request{
		TelegramUserID: 42,
		GroupID:        7,
		Time:           "bad",
	})
	if err == nil {
		t.Fatal("Create() error = nil, want validation error")
	}
}

func TestServiceRunEnqueuesJob(t *testing.T) {
	service, repo, queue := newTestService()
	repo.items[10] = domain.SummarySchedule{ID: 10, UserID: 1, GroupID: 7, Cron: "09:00", Timezone: "UTC", SummaryType: "standard", Enabled: true}
	service.now = func() time.Time { return time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC) }

	job, err := service.Run(context.Background(), 42, 10)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if job.Type != domain.JobTypeScheduledPipeline || len(queue.jobs) != 1 {
		t.Fatalf("job = %+v queue=%d", job, len(queue.jobs))
	}
}

func newTestService() (*Service, *fakeScheduleRepo, *fakeJobQueue) {
	users := &fakeUsers{user: &domain.User{ID: 1, TelegramUserID: 42}}
	repo := &fakeScheduleRepo{nextID: 1, items: make(map[int64]domain.SummarySchedule)}
	queue := &fakeJobQueue{}
	return NewService(42, users, repo, queue), repo, queue
}

type fakeUsers struct {
	user *domain.User
}

func (f *fakeUsers) UpsertByTelegramID(context.Context, int64) (*domain.User, error) {
	return f.user, nil
}

func (f *fakeUsers) FindByTelegramID(_ context.Context, telegramUserID int64) (*domain.User, error) {
	if f.user != nil && f.user.TelegramUserID == telegramUserID {
		return f.user, nil
	}
	return nil, nil
}

type fakeScheduleRepo struct {
	nextID int64
	items  map[int64]domain.SummarySchedule
}

func (f *fakeScheduleRepo) Create(_ context.Context, schedule domain.SummarySchedule) (*domain.SummarySchedule, error) {
	schedule.ID = f.nextID
	f.nextID++
	f.items[schedule.ID] = schedule
	return &schedule, nil
}

func (f *fakeScheduleRepo) Update(_ context.Context, schedule domain.SummarySchedule) (*domain.SummarySchedule, error) {
	f.items[schedule.ID] = schedule
	return &schedule, nil
}

func (f *fakeScheduleRepo) FindByUser(_ context.Context, userID, scheduleID int64) (*domain.SummarySchedule, error) {
	item, ok := f.items[scheduleID]
	if !ok || item.UserID != userID {
		return nil, nil
	}
	return &item, nil
}

func (f *fakeScheduleRepo) Delete(_ context.Context, _ int64, scheduleID int64) error {
	delete(f.items, scheduleID)
	return nil
}

func (f *fakeScheduleRepo) ListByUser(_ context.Context, userID int64) ([]domain.SummarySchedule, error) {
	var out []domain.SummarySchedule
	for _, item := range f.items {
		if item.UserID == userID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (f *fakeScheduleRepo) ListEnabled(context.Context) ([]domain.SummarySchedule, error) {
	return nil, nil
}

func (f *fakeScheduleRepo) ListRuns(context.Context, int64, int) ([]domain.ScheduleRun, error) {
	return nil, nil
}

func (f *fakeScheduleRepo) CreateRun(context.Context, domain.ScheduleRun) (*domain.ScheduleRun, error) {
	return nil, nil
}

func (f *fakeScheduleRepo) CompleteRun(context.Context, int64, domain.JobStatus, *int64, *int64, *int64, *string) error {
	return nil
}

func (f *fakeScheduleRepo) MarkScheduleRun(context.Context, int64, time.Time) error {
	return nil
}

type fakeJobQueue struct {
	jobs []domain.Job
}

func (f *fakeJobQueue) Enqueue(_ context.Context, job domain.Job) (*domain.Job, error) {
	job.ID = int64(len(f.jobs) + 1)
	f.jobs = append(f.jobs, job)
	return &job, nil
}

func (f *fakeJobQueue) ClaimNext(context.Context, string, time.Duration) (*domain.Job, error) {
	return nil, nil
}

func (f *fakeJobQueue) Find(context.Context, int64) (*domain.Job, error)                { return nil, nil }
func (f *fakeJobQueue) Complete(context.Context, int64) error                           { return nil }
func (f *fakeJobQueue) CompleteWithResult(context.Context, int64, []byte) error         { return nil }
func (f *fakeJobQueue) Retry(context.Context, int64, time.Time, string) error           { return nil }
func (f *fakeJobQueue) Dead(context.Context, int64, string) error                       { return nil }
func (f *fakeJobQueue) RecoverExpiredLeases(context.Context) (int64, error)             { return 0, nil }
func (f *fakeJobQueue) ExtendLease(context.Context, int64, string, time.Duration) error { return nil }
