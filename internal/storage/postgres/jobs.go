package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type JobRepository struct {
	db *sql.DB
}

func NewJobRepository(db *sql.DB) *JobRepository {
	return &JobRepository{db: db}
}

func (r *JobRepository) Enqueue(ctx context.Context, job domain.Job) (*domain.Job, error) {
	if job.Status == "" {
		job.Status = domain.JobStatusPending
	}
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = 4
	}
	if job.AvailableAt.IsZero() {
		job.AvailableAt = time.Now()
	}
	if len(job.Payload) == 0 {
		job.Payload = []byte(`{}`)
	}
	const query = `
		INSERT INTO jobs (type, status, payload, max_attempts, available_at, deduplication_key)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (deduplication_key) WHERE deduplication_key IS NOT NULL AND status IN ('pending', 'running', 'retry_wait')
		DO UPDATE SET deduplication_key = EXCLUDED.deduplication_key
		RETURNING id, type, status, payload, attempt, max_attempts, available_at, locked_at, locked_by,
			lease_expires_at, last_error, created_at, started_at, finished_at, deduplication_key, result
	`
	return scanJob(r.db.QueryRowContext(ctx, query, job.Type, job.Status, job.Payload, job.MaxAttempts, job.AvailableAt, nullString(job.DeduplicationKey)))
}

func (r *JobRepository) Find(ctx context.Context, jobID int64) (*domain.Job, error) {
	job, err := scanJob(r.db.QueryRowContext(ctx, `
		SELECT id, type, status, payload, attempt, max_attempts, available_at, locked_at, locked_by,
			lease_expires_at, last_error, created_at, started_at, finished_at, deduplication_key, result
		FROM jobs
		WHERE id = $1
	`, jobID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find job: %w", err)
	}
	return job, nil
}

func (r *JobRepository) ClaimNext(ctx context.Context, workerID string, lease time.Duration) (*domain.Job, error) {
	if lease <= 0 {
		lease = 5 * time.Minute
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin claim job: %w", err)
	}
	defer tx.Rollback()

	var id int64
	err = tx.QueryRowContext(ctx, `
		SELECT id
		FROM jobs
		WHERE status IN ('pending', 'retry_wait')
		  AND available_at <= now()
		ORDER BY available_at, created_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select claimable job: %w", err)
	}

	job, err := scanJob(tx.QueryRowContext(ctx, `
		UPDATE jobs
		SET status = 'running',
			attempt = attempt + 1,
			locked_at = now(),
			locked_by = $1,
			lease_expires_at = now() + $2::interval,
			started_at = COALESCE(started_at, now()),
			last_error = NULL
		WHERE id = $3
		RETURNING id, type, status, payload, attempt, max_attempts, available_at, locked_at, locked_by,
			lease_expires_at, last_error, created_at, started_at, finished_at, deduplication_key, result
	`, workerID, intervalString(lease), id))
	if err != nil {
		return nil, fmt.Errorf("update claimed job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim job: %w", err)
	}
	return job, nil
}

func (r *JobRepository) Complete(ctx context.Context, jobID int64) error {
	return r.CompleteWithResult(ctx, jobID, nil)
}

func (r *JobRepository) CompleteWithResult(ctx context.Context, jobID int64, result []byte) error {
	if len(result) == 0 {
		result = []byte(`{}`)
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'completed',
			locked_at = NULL,
			locked_by = NULL,
			lease_expires_at = NULL,
			finished_at = now(),
			last_error = NULL,
			result = $2
		WHERE id = $1
	`, jobID, result)
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	return nil
}

func (r *JobRepository) Retry(ctx context.Context, jobID int64, availableAt time.Time, message string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = CASE WHEN attempt >= max_attempts THEN 'dead' ELSE 'retry_wait' END,
			available_at = CASE WHEN attempt >= max_attempts THEN available_at ELSE $2 END,
			locked_at = NULL,
			locked_by = NULL,
			lease_expires_at = NULL,
			finished_at = CASE WHEN attempt >= max_attempts THEN now() ELSE NULL END,
			last_error = $3
		WHERE id = $1
	`, jobID, availableAt, message)
	if err != nil {
		return fmt.Errorf("retry job: %w", err)
	}
	return nil
}

func (r *JobRepository) Dead(ctx context.Context, jobID int64, message string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'dead',
			locked_at = NULL,
			locked_by = NULL,
			lease_expires_at = NULL,
			finished_at = now(),
			last_error = $2
		WHERE id = $1
	`, jobID, message)
	if err != nil {
		return fmt.Errorf("dead job: %w", err)
	}
	return nil
}

func (r *JobRepository) RecoverExpiredLeases(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'retry_wait',
			available_at = now(),
			locked_at = NULL,
			locked_by = NULL,
			lease_expires_at = NULL,
			last_error = 'lease expired'
		WHERE status = 'running'
		  AND lease_expires_at < now()
	`)
	if err != nil {
		return 0, fmt.Errorf("recover expired job leases: %w", err)
	}
	return result.RowsAffected()
}

func (r *JobRepository) ExtendLease(ctx context.Context, jobID int64, workerID string, lease time.Duration) error {
	if lease <= 0 {
		lease = 5 * time.Minute
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE jobs
		SET lease_expires_at = now() + $3::interval
		WHERE id = $1 AND locked_by = $2 AND status = 'running'
	`, jobID, workerID, intervalString(lease))
	if err != nil {
		return fmt.Errorf("extend job lease: %w", err)
	}
	return nil
}

func scanJob(row interface{ Scan(dest ...any) error }) (*domain.Job, error) {
	var job domain.Job
	var lockedAt sql.NullTime
	var lockedBy sql.NullString
	var leaseExpiresAt sql.NullTime
	var lastError sql.NullString
	var startedAt sql.NullTime
	var finishedAt sql.NullTime
	var deduplicationKey sql.NullString
	var result []byte
	if err := row.Scan(
		&job.ID,
		&job.Type,
		&job.Status,
		&job.Payload,
		&job.Attempt,
		&job.MaxAttempts,
		&job.AvailableAt,
		&lockedAt,
		&lockedBy,
		&leaseExpiresAt,
		&lastError,
		&job.CreatedAt,
		&startedAt,
		&finishedAt,
		&deduplicationKey,
		&result,
	); err != nil {
		return nil, err
	}
	job.LockedAt = timePtr(lockedAt)
	job.LockedBy = stringPtr(lockedBy)
	job.LeaseExpiresAt = timePtr(leaseExpiresAt)
	job.LastError = stringPtr(lastError)
	job.StartedAt = timePtr(startedAt)
	job.FinishedAt = timePtr(finishedAt)
	job.DeduplicationKey = stringPtr(deduplicationKey)
	job.Result = result
	if len(job.Result) == 0 {
		job.Result = []byte(`{}`)
	}
	return &job, nil
}

func intervalString(value time.Duration) string {
	return fmt.Sprintf("%f seconds", value.Seconds())
}
