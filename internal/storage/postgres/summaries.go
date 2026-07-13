package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type SummaryRepository struct {
	db *sql.DB
}

func NewSummaryRepository(db *sql.DB) *SummaryRepository {
	return &SummaryRepository{db: db}
}

func (r *SummaryRepository) CreateJob(ctx context.Context, job domain.SummaryJob) (*domain.SummaryJob, error) {
	const query = `
		INSERT INTO summary_jobs (user_id, source_type, source_id, status, started_at, completed_at, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, source_type, source_id, status, started_at, completed_at, error, created_at
	`
	return scanSummaryJob(r.db.QueryRowContext(ctx, query, job.UserID, job.SourceType, job.SourceID, job.Status, nullTime(job.StartedAt), nullTime(job.CompletedAt), nullString(job.Error)))
}

func (r *SummaryRepository) FindJob(ctx context.Context, jobID int64) (*domain.SummaryJob, error) {
	job, err := scanSummaryJob(r.db.QueryRowContext(ctx, `
		SELECT id, user_id, source_type, source_id, status, started_at, completed_at, error, created_at
		FROM summary_jobs
		WHERE id = $1
	`, jobID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find summary job: %w", err)
	}
	return job, nil
}

func (r *SummaryRepository) UpdateJobStatus(ctx context.Context, jobID int64, status domain.JobStatus, message *string) error {
	var completedAt sql.NullTime
	if status == domain.JobStatusCompleted || status == domain.JobStatusFailed {
		completedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}
	const query = `
		UPDATE summary_jobs
		SET status = $2, error = $3, completed_at = COALESCE($4::timestamptz, completed_at)
		WHERE id = $1
	`
	if _, err := r.db.ExecContext(ctx, query, jobID, status, nullString(message), completedAt); err != nil {
		return fmt.Errorf("update summary job status: %w", err)
	}
	return nil
}

func (r *SummaryRepository) CreateSummary(ctx context.Context, summary domain.Summary, topics []domain.SummaryTopic) (*domain.Summary, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin create summary: %w", err)
	}

	created, err := scanSummary(tx.QueryRowContext(ctx, `
		INSERT INTO summaries (job_id, title, overview, messages_count, sources_count, topics_count, markdown)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, job_id, title, overview, messages_count, sources_count, topics_count, markdown, created_at
	`, summary.JobID, summary.Title, summary.Overview, summary.MessagesCount, summary.SourcesCount, len(topics), summary.Markdown))
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("insert summary: %w", err)
	}

	for _, topic := range topics {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO summary_topics (summary_id, title, short_summary, full_summary, category, importance, confidence, messages_count, sources_count, position)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, created.ID, topic.Title, topic.ShortSummary, topic.FullSummary, topic.Category, topic.Importance, topic.Confidence, topic.MessagesCount, topic.SourcesCount, topic.Position); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("insert summary topic: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit create summary: %w", err)
	}
	return created, nil
}

func (r *SummaryRepository) FindSummary(ctx context.Context, summaryID int64) (*domain.Summary, error) {
	summary, err := scanSummary(r.db.QueryRowContext(ctx, `
		SELECT id, job_id, title, overview, messages_count, sources_count, topics_count, markdown, created_at
		FROM summaries
		WHERE id = $1
	`, summaryID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find summary: %w", err)
	}
	return summary, nil
}

func (r *SummaryRepository) FindSummaryByUser(ctx context.Context, userID, summaryID int64) (*domain.Summary, error) {
	summary, err := scanSummary(r.db.QueryRowContext(ctx, `
		SELECT s.id, s.job_id, s.title, s.overview, s.messages_count, s.sources_count, s.topics_count, s.markdown, s.created_at
		FROM summaries s
		JOIN summary_jobs j ON j.id = s.job_id
		WHERE j.user_id = $1 AND s.id = $2
	`, userID, summaryID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find summary by user: %w", err)
	}
	return summary, nil
}

func (r *SummaryRepository) ListSummariesByUser(ctx context.Context, userID int64, limit int) ([]domain.Summary, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.job_id, s.title, s.overview, s.messages_count, s.sources_count, s.topics_count, s.markdown, s.created_at
		FROM summaries s
		JOIN summary_jobs j ON j.id = s.job_id
		WHERE j.user_id = $1
		ORDER BY s.created_at DESC, s.id DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list summaries by user: %w", err)
	}
	defer rows.Close()

	var summaries []domain.Summary
	for rows.Next() {
		summary, err := scanSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		summaries = append(summaries, *summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate summaries: %w", err)
	}
	return summaries, nil
}

func (r *SummaryRepository) ListTopics(ctx context.Context, summaryID int64) ([]domain.SummaryTopic, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, summary_id, title, short_summary, full_summary, category, importance, confidence, messages_count, sources_count, position
		FROM summary_topics
		WHERE summary_id = $1
		ORDER BY position, importance DESC
	`, summaryID)
	if err != nil {
		return nil, fmt.Errorf("list summary topics: %w", err)
	}
	defer rows.Close()

	var topics []domain.SummaryTopic
	for rows.Next() {
		var topic domain.SummaryTopic
		if err := rows.Scan(&topic.ID, &topic.SummaryID, &topic.Title, &topic.ShortSummary, &topic.FullSummary, &topic.Category, &topic.Importance, &topic.Confidence, &topic.MessagesCount, &topic.SourcesCount, &topic.Position); err != nil {
			return nil, fmt.Errorf("scan summary topic: %w", err)
		}
		topics = append(topics, topic)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate summary topics: %w", err)
	}
	return topics, nil
}

func scanSummaryJob(row interface {
	Scan(dest ...any) error
}) (*domain.SummaryJob, error) {
	var job domain.SummaryJob
	var startedAt sql.NullTime
	var completedAt sql.NullTime
	var message sql.NullString
	if err := row.Scan(&job.ID, &job.UserID, &job.SourceType, &job.SourceID, &job.Status, &startedAt, &completedAt, &message, &job.CreatedAt); err != nil {
		return nil, err
	}
	job.StartedAt = timePtr(startedAt)
	job.CompletedAt = timePtr(completedAt)
	job.Error = stringPtr(message)
	return &job, nil
}

func scanSummary(row interface {
	Scan(dest ...any) error
}) (*domain.Summary, error) {
	var summary domain.Summary
	if err := row.Scan(&summary.ID, &summary.JobID, &summary.Title, &summary.Overview, &summary.MessagesCount, &summary.SourcesCount, &summary.TopicsCount, &summary.Markdown, &summary.CreatedAt); err != nil {
		return nil, err
	}
	return &summary, nil
}
