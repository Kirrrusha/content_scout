package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) UpsertByTelegramID(ctx context.Context, telegramUserID int64) (*domain.User, error) {
	const query = `
		INSERT INTO users (telegram_user_id)
		VALUES ($1)
		ON CONFLICT (telegram_user_id)
		DO UPDATE SET updated_at = now()
		RETURNING id, telegram_user_id, created_at, updated_at
	`

	var user domain.User
	if err := r.db.QueryRowContext(ctx, query, telegramUserID).Scan(
		&user.ID,
		&user.TelegramUserID,
		&user.CreatedAt,
		&user.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("upsert user by telegram id: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) FindByTelegramID(ctx context.Context, telegramUserID int64) (*domain.User, error) {
	const query = `
		SELECT id, telegram_user_id, created_at, updated_at
		FROM users
		WHERE telegram_user_id = $1
	`

	var user domain.User
	if err := r.db.QueryRowContext(ctx, query, telegramUserID).Scan(
		&user.ID,
		&user.TelegramUserID,
		&user.CreatedAt,
		&user.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find user by telegram id: %w", err)
	}
	return &user, nil
}
