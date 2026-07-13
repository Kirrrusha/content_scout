package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type MessageCollectionRepository struct {
	db *sql.DB
}

func NewMessageCollectionRepository(db *sql.DB) *MessageCollectionRepository {
	return &MessageCollectionRepository{db: db}
}

func (r *MessageCollectionRepository) CreateJob(ctx context.Context, job domain.MessageCollectionJob) (*domain.MessageCollectionJob, error) {
	const query = `
		INSERT INTO message_collection_jobs (user_id, group_id, mode, limit_count, status, error)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, user_id, group_id, mode, limit_count, status, error, created_at, updated_at
	`
	return scanCollectionJob(r.db.QueryRowContext(ctx, query, job.UserID, job.GroupID, job.Mode, job.Limit, job.Status, nullString(job.Error)))
}

func (r *MessageCollectionRepository) UpdateJobStatus(ctx context.Context, jobID int64, status domain.JobStatus, message *string) error {
	const query = `
		UPDATE message_collection_jobs
		SET status = $2, error = $3, updated_at = now()
		WHERE id = $1
	`
	if _, err := r.db.ExecContext(ctx, query, jobID, status, nullString(message)); err != nil {
		return fmt.Errorf("update message collection job status: %w", err)
	}
	return nil
}

func (r *MessageCollectionRepository) AddMessages(ctx context.Context, messages []domain.CollectedMessage) error {
	if len(messages) == 0 {
		return nil
	}
	const query = `
		INSERT INTO collected_messages (
			job_id, user_id, chat_id, telegram_chat_id, message_id, date, edit_date,
			sender_id, sender_name, text, caption, url, reply_to_id, forwarded, has_media, media_type
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (job_id, chat_id, message_id) DO NOTHING
	`
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin add collected messages: %w", err)
	}
	for _, message := range messages {
		if _, err := tx.ExecContext(ctx, query,
			message.JobID,
			message.UserID,
			message.ChatID,
			message.TelegramChatID,
			message.MessageID,
			message.Date,
			nullTime(message.EditDate),
			message.SenderID,
			message.SenderName,
			message.Text,
			message.Caption,
			message.URL,
			nullInt64(message.ReplyToID),
			message.Forwarded,
			message.HasMedia,
			message.MediaType,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert collected message: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit add collected messages: %w", err)
	}
	return nil
}

func (r *MessageCollectionRepository) ListMessages(ctx context.Context, jobID int64) ([]domain.CollectedMessage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, job_id, user_id, chat_id, telegram_chat_id, message_id, date, edit_date,
			sender_id, sender_name, text, caption, url, reply_to_id, forwarded, has_media, media_type, created_at
		FROM collected_messages
		WHERE job_id = $1
		ORDER BY date, id
	`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list collected messages: %w", err)
	}
	defer rows.Close()

	var messages []domain.CollectedMessage
	for rows.Next() {
		message, err := scanCollectedMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan collected message: %w", err)
		}
		messages = append(messages, *message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate collected messages: %w", err)
	}
	return messages, nil
}

func scanCollectionJob(row interface {
	Scan(dest ...any) error
}) (*domain.MessageCollectionJob, error) {
	var job domain.MessageCollectionJob
	var message sql.NullString
	if err := row.Scan(&job.ID, &job.UserID, &job.GroupID, &job.Mode, &job.Limit, &job.Status, &message, &job.CreatedAt, &job.UpdatedAt); err != nil {
		return nil, err
	}
	job.Error = stringPtr(message)
	return &job, nil
}

func scanCollectedMessage(row interface {
	Scan(dest ...any) error
}) (*domain.CollectedMessage, error) {
	var message domain.CollectedMessage
	var editDate sql.NullTime
	var replyToID sql.NullInt64
	if err := row.Scan(
		&message.ID,
		&message.JobID,
		&message.UserID,
		&message.ChatID,
		&message.TelegramChatID,
		&message.MessageID,
		&message.Date,
		&editDate,
		&message.SenderID,
		&message.SenderName,
		&message.Text,
		&message.Caption,
		&message.URL,
		&replyToID,
		&message.Forwarded,
		&message.HasMedia,
		&message.MediaType,
		&message.CreatedAt,
	); err != nil {
		return nil, err
	}
	message.EditDate = timePtr(editDate)
	message.ReplyToID = int64Ptr(replyToID)
	return &message, nil
}
