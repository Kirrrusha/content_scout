package scheduler

import (
	"context"
	"encoding/json"
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

func TestEnqueueDueCreatesScheduledPipelineJob(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 13, 9, 30, 0, 0, time.UTC)
	repo := &fakeSchedules{enabled: []domain.SummarySchedule{{
		ID:          5,
		GroupID:     7,
		Enabled:     true,
		Cron:        "09:00",
		Timezone:    "UTC",
		SummaryType: "standard",
	}}}
	queue := &fakeQueue{}
	service := NewService(42, repo, nil, nil, nil, nil)
	service.now = func() time.Time { return now }

	count, err := service.EnqueueDue(ctx, queue)
	if err != nil {
		t.Fatalf("EnqueueDue() error = %v", err)
	}
	if count != 1 || len(queue.jobs) != 1 {
		t.Fatalf("count=%d jobs=%d", count, len(queue.jobs))
	}
	job := queue.jobs[0]
	if job.Type != domain.JobTypeScheduledPipeline {
		t.Fatalf("job type = %s", job.Type)
	}
	if job.DeduplicationKey == nil || *job.DeduplicationKey != "scheduled_pipeline:5:2026-07-13" {
		t.Fatalf("dedup key = %v", job.DeduplicationKey)
	}
	var payload domain.JobPayloadScheduledPipeline
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatalf("payload decode: %v", err)
	}
	if payload.Schedule.ID != 5 || payload.Schedule.GroupID != 7 {
		t.Fatalf("payload = %+v", payload)
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
	enabled          []domain.SummarySchedule
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
	return f.enabled, nil
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

type fakeQueue struct {
	jobs []domain.Job
}

func (f *fakeQueue) Enqueue(_ context.Context, job domain.Job) (*domain.Job, error) {
	f.jobs = append(f.jobs, job)
	return &f.jobs[len(f.jobs)-1], nil
}

func (f *fakeSummarizer) GenerateFromCollection(_ context.Context, req summary.GenerateRequest) (*summary.GenerateResult, error) {
	f.request = req
	return f.result, nil
}
