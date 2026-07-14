package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/summary"
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

func TestWorkerStoresHandlerResult(t *testing.T) {
	repo := &fakeJobRepo{job: &domain.Job{ID: 4, Type: domain.JobTypeSummaryGeneration, Attempt: 1, Payload: []byte(`{"telegram_user_id":42,"collection_job_id":10}`)}}
	handler := SummaryGenerationHandler{Summarizer: &fakeSummaryGenerator{result: &summaryResultFixture}}
	worker := NewWorker(repo, handler, discardLogger(), "worker-1")

	claimed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !claimed || repo.completedID != 4 {
		t.Fatalf("claimed=%v completed=%d", claimed, repo.completedID)
	}
	var result domain.JobResultSummaryGeneration
	if err := json.Unmarshal(repo.completedResult, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.SummaryID != 20 || result.TopicsCount != 3 {
		t.Fatalf("result = %+v", result)
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
	job             *domain.Job
	completedID     int64
	completedResult []byte
	retryID         int64
	retryAt         time.Time
	deadID          int64
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func (f *fakeJobRepo) Enqueue(context.Context, domain.Job) (*domain.Job, error) {
	return nil, nil
}

func (f *fakeJobRepo) Find(context.Context, int64) (*domain.Job, error) {
	return nil, nil
}

func (f *fakeJobRepo) ClaimNext(context.Context, string, time.Duration) (*domain.Job, error) {
	job := f.job
	f.job = nil
	return job, nil
}

func (f *fakeJobRepo) Complete(_ context.Context, jobID int64) error {
	return f.CompleteWithResult(context.Background(), jobID, nil)
}

func (f *fakeJobRepo) CompleteWithResult(_ context.Context, jobID int64, result []byte) error {
	f.completedID = jobID
	f.completedResult = append([]byte(nil), result...)
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

var summaryResultFixture = summary.GenerateResult{
	SummaryID:      20,
	SummaryJobID:   30,
	TopicsCount:    3,
	MessagesCount:  12,
	DuplicateCount: 2,
}

type fakeSummaryGenerator struct {
	request summary.GenerateRequest
	result  *summary.GenerateResult
}

func (f *fakeSummaryGenerator) GenerateFromCollection(_ context.Context, req summary.GenerateRequest) (*summary.GenerateResult, error) {
	f.request = req
	return f.result, nil
}
