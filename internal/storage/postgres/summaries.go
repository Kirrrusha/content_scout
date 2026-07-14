package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
		var topicID int64
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO summary_topics (summary_id, title, short_summary, full_summary, category, importance, confidence, messages_count, sources_count, position)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id
		`, created.ID, topic.Title, topic.ShortSummary, topic.FullSummary, topic.Category, topic.Importance, topic.Confidence, topic.MessagesCount, topic.SourcesCount, topic.Position).Scan(&topicID); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("insert summary topic: %w", err)
		}
		for _, source := range topic.Sources {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO summary_topic_sources (topic_id, chat_id, telegram_chat_id, title, username)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (topic_id, chat_id) DO NOTHING
			`, topicID, source.ChatID, source.TelegramChatID, source.Title, nullString(source.Username)); err != nil {
				_ = tx.Rollback()
				return nil, fmt.Errorf("insert summary topic source: %w", err)
			}
		}
		for _, message := range topic.Messages {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO summary_topic_messages (topic_id, collected_message_id, cluster_index, is_canonical)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (topic_id, collected_message_id) DO NOTHING
			`, topicID, message.CollectedMessageID, message.ClusterIndex, message.IsCanonical); err != nil {
				_ = tx.Rollback()
				return nil, fmt.Errorf("insert summary topic message: %w", err)
			}
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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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
	if err := r.attachTopicSources(ctx, summaryID, topics); err != nil {
		return nil, err
	}
	if err := r.attachTopicMessages(ctx, summaryID, topics); err != nil {
		return nil, err
	}
	return topics, nil
}

func (r *SummaryRepository) DeleteSummariesOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM summaries
		WHERE created_at < $1
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete old summaries: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted summaries: %w", err)
	}
	return deleted, nil
}

func (r *SummaryRepository) attachTopicSources(ctx context.Context, summaryID int64, topics []domain.SummaryTopic) error {
	if len(topics) == 0 {
		return nil
	}
	topicByID := make(map[int64]int, len(topics))
	for i := range topics {
		topicByID[topics[i].ID] = i
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT sts.topic_id, sts.id, sts.chat_id, sts.telegram_chat_id, sts.title, sts.username
		FROM summary_topic_sources sts
		JOIN summary_topics st ON st.id = sts.topic_id
		WHERE st.summary_id = $1
		ORDER BY sts.title, sts.id
	`, summaryID)
	if err != nil {
		return fmt.Errorf("list summary topic sources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var topicID int64
		var source domain.SummaryTopicSource
		var username sql.NullString
		if err := rows.Scan(&topicID, &source.ID, &source.ChatID, &source.TelegramChatID, &source.Title, &username); err != nil {
			return fmt.Errorf("scan summary topic source: %w", err)
		}
		source.TopicID = topicID
		source.Username = stringPtr(username)
		if index, ok := topicByID[topicID]; ok {
			topics[index].Sources = append(topics[index].Sources, source)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate summary topic sources: %w", err)
	}
	return nil
}

func (r *SummaryRepository) attachTopicMessages(ctx context.Context, summaryID int64, topics []domain.SummaryTopic) error {
	if len(topics) == 0 {
		return nil
	}
	topicByID := make(map[int64]int, len(topics))
	for i := range topics {
		topicByID[topics[i].ID] = i
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			stm.topic_id,
			stm.id,
			stm.collected_message_id,
			cm.chat_id,
			cm.telegram_chat_id,
			cm.message_id,
			COALESCE(tc.title, cm.sender_name, 'Telegram') AS source_title,
			tc.username,
			cm.url,
			stm.cluster_index,
			stm.is_canonical
		FROM summary_topic_messages stm
		JOIN summary_topics st ON st.id = stm.topic_id
		JOIN collected_messages cm ON cm.id = stm.collected_message_id
		LEFT JOIN telegram_chats tc ON tc.id = cm.chat_id
		WHERE st.summary_id = $1
		ORDER BY stm.cluster_index, stm.is_canonical DESC, stm.id
	`, summaryID)
	if err != nil {
		return fmt.Errorf("list summary topic messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var topicID int64
		var message domain.SummaryTopicMessage
		var username sql.NullString
		var fallbackURL sql.NullString
		if err := rows.Scan(&topicID, &message.ID, &message.CollectedMessageID, &message.ChatID, &message.TelegramChatID, &message.MessageID, &message.SourceTitle, &username, &fallbackURL, &message.ClusterIndex, &message.IsCanonical); err != nil {
			return fmt.Errorf("scan summary topic message: %w", err)
		}
		message.TopicID = topicID
		message.SourceURL = telegramMessageURL(message.TelegramChatID, message.MessageID, username.String, fallbackURL.String)
		if index, ok := topicByID[topicID]; ok {
			topics[index].Messages = append(topics[index].Messages, message)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate summary topic messages: %w", err)
	}
	return nil
}

func telegramMessageURL(telegramChatID, messageID int64, username, fallback string) string {
	if strings.TrimSpace(username) != "" {
		return fmt.Sprintf("https://t.me/%s/%d", strings.TrimPrefix(strings.TrimSpace(username), "@"), messageID)
	}
	chatID := strconv.FormatInt(telegramChatID, 10)
	if strings.HasPrefix(chatID, "-100") {
		return fmt.Sprintf("https://t.me/c/%s/%d", strings.TrimPrefix(chatID, "-100"), messageID)
	}
	return strings.TrimSpace(fallback)
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
