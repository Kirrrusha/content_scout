package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type SummaryScheduleRepository struct {
	db *sql.DB
}

func NewSummaryScheduleRepository(db *sql.DB) *SummaryScheduleRepository {
	return &SummaryScheduleRepository{db: db}
}

func (r *SummaryScheduleRepository) Create(ctx context.Context, schedule domain.SummarySchedule) (*domain.SummarySchedule, error) {
	return scanSchedule(r.db.QueryRowContext(ctx, `
		INSERT INTO summary_schedules (user_id, group_id, cron, timezone, enabled, summary_type, quiet_hours_start, quiet_hours_end, export_to_obsidian)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, user_id, group_id, cron, timezone, enabled, summary_type, quiet_hours_start, quiet_hours_end, export_to_obsidian, last_run_at, created_at, updated_at
	`, schedule.UserID, schedule.GroupID, schedule.Cron, schedule.Timezone, schedule.Enabled, schedule.SummaryType, schedule.QuietHoursStart, schedule.QuietHoursEnd, schedule.ExportToObsidian))
}

func (r *SummaryScheduleRepository) Update(ctx context.Context, schedule domain.SummarySchedule) (*domain.SummarySchedule, error) {
	return scanSchedule(r.db.QueryRowContext(ctx, `
		UPDATE summary_schedules
		SET group_id = $3, cron = $4, timezone = $5, enabled = $6, summary_type = $7,
			quiet_hours_start = $8, quiet_hours_end = $9, export_to_obsidian = $10, updated_at = now()
		WHERE user_id = $1 AND id = $2
		RETURNING id, user_id, group_id, cron, timezone, enabled, summary_type, quiet_hours_start, quiet_hours_end, export_to_obsidian, last_run_at, created_at, updated_at
	`, schedule.UserID, schedule.ID, schedule.GroupID, schedule.Cron, schedule.Timezone, schedule.Enabled, schedule.SummaryType, schedule.QuietHoursStart, schedule.QuietHoursEnd, schedule.ExportToObsidian))
}

func (r *SummaryScheduleRepository) ListByUser(ctx context.Context, userID int64) ([]domain.SummarySchedule, error) {
	return r.list(ctx, `WHERE user_id = $1 ORDER BY id`, userID)
}

func (r *SummaryScheduleRepository) FindByUser(ctx context.Context, userID, scheduleID int64) (*domain.SummarySchedule, error) {
	schedule, err := scanSchedule(r.db.QueryRowContext(ctx, `
		SELECT id, user_id, group_id, cron, timezone, enabled, summary_type, quiet_hours_start, quiet_hours_end, export_to_obsidian, last_run_at, created_at, updated_at
		FROM summary_schedules
		WHERE user_id = $1 AND id = $2
	`, userID, scheduleID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find summary schedule: %w", err)
	}
	return schedule, nil
}

func (r *SummaryScheduleRepository) Delete(ctx context.Context, userID, scheduleID int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM summary_schedules WHERE user_id = $1 AND id = $2`, userID, scheduleID); err != nil {
		return fmt.Errorf("delete summary schedule: %w", err)
	}
	return nil
}

func (r *SummaryScheduleRepository) ListEnabled(ctx context.Context) ([]domain.SummarySchedule, error) {
	return r.list(ctx, `WHERE enabled = true ORDER BY id`)
}

func (r *SummaryScheduleRepository) list(ctx context.Context, clause string, args ...any) ([]domain.SummarySchedule, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, group_id, cron, timezone, enabled, summary_type, quiet_hours_start, quiet_hours_end, export_to_obsidian, last_run_at, created_at, updated_at
		FROM summary_schedules
		`+clause, args...)
	if err != nil {
		return nil, fmt.Errorf("list summary schedules: %w", err)
	}
	defer rows.Close()
	var schedules []domain.SummarySchedule
	for rows.Next() {
		schedule, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan summary schedule: %w", err)
		}
		schedules = append(schedules, *schedule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate summary schedules: %w", err)
	}
	return schedules, nil
}

func (r *SummaryScheduleRepository) ListRuns(ctx context.Context, scheduleID int64, limit int) ([]domain.ScheduleRun, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, schedule_id, collection_job_id, summary_id, export_id, status, error, started_at, completed_at
		FROM schedule_runs
		WHERE schedule_id = $1
		ORDER BY started_at DESC, id DESC
		LIMIT $2
	`, scheduleID, limit)
	if err != nil {
		return nil, fmt.Errorf("list schedule runs: %w", err)
	}
	defer rows.Close()
	var runs []domain.ScheduleRun
	for rows.Next() {
		run, err := scanScheduleRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan schedule run: %w", err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schedule runs: %w", err)
	}
	return runs, nil
}

func (r *SummaryScheduleRepository) CreateRun(ctx context.Context, run domain.ScheduleRun) (*domain.ScheduleRun, error) {
	return scanScheduleRun(r.db.QueryRowContext(ctx, `
		INSERT INTO schedule_runs (schedule_id, status, error)
		VALUES ($1, $2, $3)
		RETURNING id, schedule_id, collection_job_id, summary_id, export_id, status, error, started_at, completed_at
	`, run.ScheduleID, run.Status, nullString(run.Error)))
}

func (r *SummaryScheduleRepository) CompleteRun(ctx context.Context, runID int64, status domain.JobStatus, collectionJobID, summaryID, exportID *int64, message *string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE schedule_runs
		SET status = $2, collection_job_id = $3, summary_id = $4, export_id = $5, error = $6, completed_at = now()
		WHERE id = $1
	`, runID, status, nullInt64(collectionJobID), nullInt64(summaryID), nullInt64(exportID), nullString(message))
	if err != nil {
		return fmt.Errorf("complete schedule run: %w", err)
	}
	return nil
}

func (r *SummaryScheduleRepository) MarkScheduleRun(ctx context.Context, scheduleID int64, runAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE summary_schedules SET last_run_at = $2, updated_at = now() WHERE id = $1`, scheduleID, runAt)
	if err != nil {
		return fmt.Errorf("mark summary schedule run: %w", err)
	}
	return nil
}

func scanSchedule(row interface{ Scan(dest ...any) error }) (*domain.SummarySchedule, error) {
	var schedule domain.SummarySchedule
	var lastRunAt sql.NullTime
	if err := row.Scan(&schedule.ID, &schedule.UserID, &schedule.GroupID, &schedule.Cron, &schedule.Timezone, &schedule.Enabled, &schedule.SummaryType, &schedule.QuietHoursStart, &schedule.QuietHoursEnd, &schedule.ExportToObsidian, &lastRunAt, &schedule.CreatedAt, &schedule.UpdatedAt); err != nil {
		return nil, err
	}
	schedule.LastRunAt = timePtr(lastRunAt)
	return &schedule, nil
}

func scanScheduleRun(row interface{ Scan(dest ...any) error }) (*domain.ScheduleRun, error) {
	var run domain.ScheduleRun
	var collectionJobID sql.NullInt64
	var summaryID sql.NullInt64
	var exportID sql.NullInt64
	var message sql.NullString
	var completedAt sql.NullTime
	if err := row.Scan(&run.ID, &run.ScheduleID, &collectionJobID, &summaryID, &exportID, &run.Status, &message, &run.StartedAt, &completedAt); err != nil {
		return nil, err
	}
	run.CollectionJobID = int64Ptr(collectionJobID)
	run.SummaryID = int64Ptr(summaryID)
	run.ExportID = int64Ptr(exportID)
	run.Error = stringPtr(message)
	run.CompletedAt = timePtr(completedAt)
	return &run, nil
}
