package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/summary"
)

func TestIsDueHonorsTimezoneAndQuietHours(t *testing.T) {
	now := time.Date(2026, 7, 13, 9, 30, 0, 0, time.UTC)
	schedule := domain.SummarySchedule{Enabled: true, Cron: "12:30", Timezone: "Europe/Moscow"}
	if !IsDue(schedule, now) {
		t.Fatal("schedule should be due at local 12:30")
	}
	schedule.QuietHoursStart = "12:00"
	schedule.QuietHoursEnd = "13:00"
	if IsDue(schedule, now) {
		t.Fatal("schedule should be blocked by quiet hours")
	}
	last := now
	schedule.QuietHoursStart = ""
	schedule.QuietHoursEnd = ""
	schedule.LastRunAt = &last
	if IsDue(schedule, now) {
		t.Fatal("schedule should not run twice on same local day")
	}
}

func TestRunScheduleCollectsSummarizesAndMarksRun(t *testing.T) {
	ctx := context.Background()
	repo := &fakeSchedules{}
	collector := &fakeCollector{result: &collection.Result{JobID: 10, MessagesCount: 3}}
	summarizer := &fakeSummarizer{result: &summary.GenerateResult{SummaryID: 20}}
	service := NewService(42, repo, collector, summarizer, nil, nil)
	service.now = func() time.Time { return time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC) }

	err := service.RunSchedule(ctx, domain.SummarySchedule{ID: 1, GroupID: 7, SummaryType: "detailed"})
	if err != nil {
		t.Fatalf("RunSchedule() error = %v", err)
	}
	if collector.request.GroupID != 7 || collector.request.TelegramUserID != 42 {
		t.Fatalf("collector request = %+v", collector.request)
	}
	if summarizer.request.CollectionJobID != 10 || summarizer.request.Format != "detailed" {
		t.Fatalf("summary request = %+v", summarizer.request)
	}
	if repo.completedStatus != domain.JobStatusCompleted || repo.summaryID == nil || *repo.summaryID != 20 {
		t.Fatalf("repo status=%s summary=%v", repo.completedStatus, repo.summaryID)
	}
	if repo.markedScheduleID != 1 {
		t.Fatalf("marked schedule = %d", repo.markedScheduleID)
	}
}

type fakeSchedules struct {
	runID            int64
	completedStatus  domain.JobStatus
	summaryID        *int64
	markedScheduleID int64
}

func (f *fakeSchedules) Create(context.Context, domain.SummarySchedule) (*domain.SummarySchedule, error) {
	return nil, nil
}
func (f *fakeSchedules) Update(context.Context, domain.SummarySchedule) (*domain.SummarySchedule, error) {
	return nil, nil
}
func (f *fakeSchedules) ListByUser(context.Context, int64) ([]domain.SummarySchedule, error) {
	return nil, nil
}
func (f *fakeSchedules) ListEnabled(context.Context) ([]domain.SummarySchedule, error) {
	return nil, nil
}
func (f *fakeSchedules) CreateRun(_ context.Context, run domain.ScheduleRun) (*domain.ScheduleRun, error) {
	f.runID++
	run.ID = f.runID
	return &run, nil
}
func (f *fakeSchedules) CompleteRun(_ context.Context, _ int64, status domain.JobStatus, _, summaryID, _ *int64, _ *string) error {
	f.completedStatus = status
	f.summaryID = summaryID
	return nil
}
func (f *fakeSchedules) MarkScheduleRun(_ context.Context, scheduleID int64, _ time.Time) error {
	f.markedScheduleID = scheduleID
	return nil
}

type fakeCollector struct {
	request collection.Request
	result  *collection.Result
}

func (f *fakeCollector) CollectGroup(_ context.Context, req collection.Request) (*collection.Result, error) {
	f.request = req
	return f.result, nil
}

type fakeSummarizer struct {
	request summary.GenerateRequest
	result  *summary.GenerateResult
}

func (f *fakeSummarizer) GenerateFromCollection(_ context.Context, req summary.GenerateRequest) (*summary.GenerateResult, error) {
	f.request = req
	return f.result, nil
}
