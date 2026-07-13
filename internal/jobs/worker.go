package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/storage"
)

type Handler interface {
	HandleJob(ctx context.Context, job domain.Job) error
}

type HandlerFunc func(ctx context.Context, job domain.Job) error

func (f HandlerFunc) HandleJob(ctx context.Context, job domain.Job) error {
	return f(ctx, job)
}

type Worker struct {
	repo    storage.JobRepository
	handler Handler
	logger  *slog.Logger
	id      string
	lease   time.Duration
	now     func() time.Time
}

func NewWorker(repo storage.JobRepository, handler Handler, logger *slog.Logger, id string) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	if id == "" {
		id = "worker"
	}
	return &Worker{repo: repo, handler: handler, logger: logger, id: id, lease: 5 * time.Minute, now: time.Now}
}

func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	if w.repo == nil || w.handler == nil {
		return false, errors.New("job worker is not configured")
	}
	if _, err := w.repo.RecoverExpiredLeases(ctx); err != nil {
		return false, err
	}
	job, err := w.repo.ClaimNext(ctx, w.id, w.lease)
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}

	start := w.now()
	logger := w.logger.With("job_id", job.ID, "job_type", job.Type, "attempt", job.Attempt, "worker_id", w.id)
	logger.Info("job started")
	err = w.handler.HandleJob(ctx, *job)
	duration := w.now().Sub(start)
	if err == nil {
		if completeErr := w.repo.Complete(ctx, job.ID); completeErr != nil {
			return true, completeErr
		}
		logger.Info("job completed", "duration_ms", duration.Milliseconds(), "result", "completed")
		return true, nil
	}

	if permanentError(err) {
		if deadErr := w.repo.Dead(ctx, job.ID, err.Error()); deadErr != nil {
			return true, deadErr
		}
		logger.Error("job dead", "duration_ms", duration.Milliseconds(), "result", "dead", "error", err)
		return true, nil
	}
	availableAt := w.now().Add(backoff(job.Attempt))
	if retryErr := w.repo.Retry(ctx, job.ID, availableAt, err.Error()); retryErr != nil {
		return true, retryErr
	}
	logger.Warn("job retry scheduled", "duration_ms", duration.Milliseconds(), "result", "retry", "available_at", availableAt, "error", err)
	return true, nil
}

func backoff(attempt int) time.Duration {
	steps := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}
	if attempt <= 0 {
		attempt = 1
	}
	index := attempt - 1
	if index >= len(steps) {
		index = len(steps) - 1
	}
	base := steps[index]
	jitter := time.Duration(rand.Int63n(int64(base / 5)))
	return base + jitter
}

func permanentError(err error) bool {
	text := strings.ToLower(err.Error())
	permanent := []string{
		"unknown chat",
		"deleted source group",
		"invalid configuration",
		"telegram session is not connected",
		"telegram session is not started",
		"invalid obsidian api key",
		"invalid user input",
	}
	for _, marker := range permanent {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

type ScheduledPipelineHandler struct {
	Scheduler interface {
		RunSchedule(ctx context.Context, schedule domain.SummarySchedule) error
	}
}

func (h ScheduledPipelineHandler) HandleJob(ctx context.Context, job domain.Job) error {
	if job.Type != domain.JobTypeScheduledPipeline {
		return fmt.Errorf("unsupported job type: %s", job.Type)
	}
	if h.Scheduler == nil {
		return errors.New("scheduler runtime is not configured")
	}
	var payload domain.JobPayloadScheduledPipeline
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("invalid user input: decode scheduled pipeline payload: %w", err)
	}
	if payload.Schedule.ID == 0 {
		return errors.New("invalid user input: schedule id is required")
	}
	return h.Scheduler.RunSchedule(ctx, payload.Schedule)
}
