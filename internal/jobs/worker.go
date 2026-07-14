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
	"github.com/kirilllebedenko/content_scout/internal/summary"
)

type Handler interface {
	HandleJob(ctx context.Context, job domain.Job) error
}

type ResultHandler interface {
	HandleJobWithResult(ctx context.Context, job domain.Job) ([]byte, error)
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
	var result []byte
	if resultHandler, ok := w.handler.(ResultHandler); ok {
		result, err = resultHandler.HandleJobWithResult(ctx, *job)
	} else {
		err = w.handler.HandleJob(ctx, *job)
	}
	duration := w.now().Sub(start)
	if err == nil {
		if completeErr := w.repo.CompleteWithResult(ctx, job.ID, result); completeErr != nil {
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

type SummaryGenerationHandler struct {
	Summarizer interface {
		GenerateFromCollection(ctx context.Context, req summary.GenerateRequest) (*summary.GenerateResult, error)
	}
}

func (h SummaryGenerationHandler) HandleJob(ctx context.Context, job domain.Job) error {
	_, err := h.HandleJobWithResult(ctx, job)
	return err
}

func (h SummaryGenerationHandler) HandleJobWithResult(ctx context.Context, job domain.Job) ([]byte, error) {
	if job.Type != domain.JobTypeSummaryGeneration {
		return nil, fmt.Errorf("unsupported job type: %s", job.Type)
	}
	if h.Summarizer == nil {
		return nil, errors.New("summary generation runtime is not configured")
	}
	var payload domain.JobPayloadSummaryGeneration
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, fmt.Errorf("invalid user input: decode summary generation payload: %w", err)
	}
	if payload.TelegramUserID == 0 || payload.CollectionJobID == 0 {
		return nil, errors.New("invalid user input: telegram_user_id and collection_job_id are required")
	}
	result, err := h.Summarizer.GenerateFromCollection(ctx, summary.GenerateRequest{
		TelegramUserID:  payload.TelegramUserID,
		CollectionJobID: payload.CollectionJobID,
		Format:          payload.Format,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(domain.JobResultSummaryGeneration{
		SummaryID:      result.SummaryID,
		SummaryJobID:   result.SummaryJobID,
		TopicsCount:    result.TopicsCount,
		MessagesCount:  result.MessagesCount,
		DuplicateCount: result.DuplicateCount,
	})
}

type MultiHandler []Handler

func (h MultiHandler) HandleJob(ctx context.Context, job domain.Job) error {
	_, err := h.HandleJobWithResult(ctx, job)
	return err
}

func (h MultiHandler) HandleJobWithResult(ctx context.Context, job domain.Job) ([]byte, error) {
	for _, handler := range h {
		if resultHandler, ok := handler.(ResultHandler); ok {
			result, err := resultHandler.HandleJobWithResult(ctx, job)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unsupported job type") {
				return result, err
			}
			continue
		}
		err := handler.HandleJob(ctx, job)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unsupported job type") {
			return nil, err
		}
	}
	return nil, fmt.Errorf("unsupported job type: %s", job.Type)
}
