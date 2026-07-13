package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type ReadPositionRepository struct {
	db *sql.DB
}

func NewReadPositionRepository(db *sql.DB) *ReadPositionRepository {
	return &ReadPositionRepository{db: db}
}

func (r *ReadPositionRepository) Upsert(ctx context.Context, position domain.ReadPosition) error {
	const query = `
		INSERT INTO read_positions (user_id, chat_id, last_summarized_message_id, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (user_id, chat_id)
		DO UPDATE SET last_summarized_message_id = EXCLUDED.last_summarized_message_id, updated_at = now()
	`
	if _, err := r.db.ExecContext(ctx, query, position.UserID, position.ChatID, position.LastSummarizedMessageID); err != nil {
		return fmt.Errorf("upsert read position: %w", err)
	}
	return nil
}

func (r *ReadPositionRepository) Find(ctx context.Context, userID, chatID int64) (*domain.ReadPosition, error) {
	const query = `
		SELECT user_id, chat_id, last_summarized_message_id, updated_at
		FROM read_positions
		WHERE user_id = $1 AND chat_id = $2
	`
	var position domain.ReadPosition
	if err := r.db.QueryRowContext(ctx, query, userID, chatID).Scan(&position.UserID, &position.ChatID, &position.LastSummarizedMessageID, &position.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find read position: %w", err)
	}
	return &position, nil
}
