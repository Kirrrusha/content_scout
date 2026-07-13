package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

func TestWorkerCompletesClaimedJob(t *testing.T) {
	repo := &fakeJobRepo{job: &domain.Job{ID: 1, Type: domain.JobTypeScheduledPipeline, Attempt: 1}}
	worker := NewWorker(repo, HandlerFunc(func(context.Context, domain.Job) error { return nil }), discardLogger(), "worker-1")
	worker.now = func() time.Time { return time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC) }

	claimed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !claimed || repo.completedID != 1 {
		t.Fatalf("claimed=%v completed=%d", claimed, repo.completedID)
	}
}

func TestWorkerRetriesTemporaryError(t *testing.T) {
	repo := &fakeJobRepo{job: &domain.Job{ID: 2, Type: domain.JobTypeScheduledPipeline, Attempt: 1}}
	worker := NewWorker(repo, HandlerFunc(func(context.Context, domain.Job) error { return errors.New("network timeout") }), discardLogger(), "worker-1")
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	worker.now = func() time.Time { return now }

	claimed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !claimed || repo.retryID != 2 || repo.retryAt.Before(now.Add(time.Minute)) {
		t.Fatalf("retry id=%d at=%s", repo.retryID, repo.retryAt)
	}
}

func TestWorkerMarksPermanentErrorDead(t *testing.T) {
	repo := &fakeJobRepo{job: &domain.Job{ID: 3, Type: domain.JobTypeScheduledPipeline, Attempt: 1}}
	worker := NewWorker(repo, HandlerFunc(func(context.Context, domain.Job) error { return errors.New("invalid user input") }), discardLogger(), "worker-1")

	claimed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !claimed || repo.deadID != 3 {
		t.Fatalf("dead id=%d", repo.deadID)
	}
}

func TestScheduledPipelineHandlerRunsSchedule(t *testing.T) {
	payload := []byte(`{"schedule":{"id":7,"group_id":9,"summary_type":"standard"}}`)
	scheduler := &fakePipelineScheduler{}
	handler := ScheduledPipelineHandler{Scheduler: scheduler}

	err := handler.HandleJob(context.Background(), domain.Job{Type: domain.JobTypeScheduledPipeline, Payload: payload})
	if err != nil {
		t.Fatalf("HandleJob() error = %v", err)
	}
	if scheduler.schedule.ID != 7 || scheduler.schedule.GroupID != 9 {
		t.Fatalf("schedule = %+v", scheduler.schedule)
	}
}

type fakeJobRepo struct {
	job         *domain.Job
	completedID int64
	retryID     int64
	retryAt     time.Time
	deadID      int64
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func (f *fakeJobRepo) Enqueue(context.Context, domain.Job) (*domain.Job, error) {
	return nil, nil
}

func (f *fakeJobRepo) ClaimNext(context.Context, string, time.Duration) (*domain.Job, error) {
	job := f.job
	f.job = nil
	return job, nil
}

func (f *fakeJobRepo) Complete(_ context.Context, jobID int64) error {
	f.completedID = jobID
	return nil
}

func (f *fakeJobRepo) Retry(_ context.Context, jobID int64, availableAt time.Time, _ string) error {
	f.retryID = jobID
	f.retryAt = availableAt
	return nil
}

func (f *fakeJobRepo) Dead(_ context.Context, jobID int64, _ string) error {
	f.deadID = jobID
	return nil
}

func (f *fakeJobRepo) RecoverExpiredLeases(context.Context) (int64, error) {
	return 0, nil
}

func (f *fakeJobRepo) ExtendLease(context.Context, int64, string, time.Duration) error {
	return nil
}

type fakePipelineScheduler struct {
	schedule domain.SummarySchedule
}

func (f *fakePipelineScheduler) RunSchedule(_ context.Context, schedule domain.SummarySchedule) error {
	f.schedule = schedule
	return nil
}
