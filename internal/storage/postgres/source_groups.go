package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type SourceGroupRepository struct {
	db *sql.DB
}

func NewSourceGroupRepository(db *sql.DB) *SourceGroupRepository {
	return &SourceGroupRepository{db: db}
}

func (r *SourceGroupRepository) Create(ctx context.Context, group domain.SourceGroup) (*domain.SourceGroup, error) {
	const query = `
		INSERT INTO source_groups (user_id, name, description)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, name, description, created_at, updated_at
	`
	return scanSourceGroup(r.db.QueryRowContext(ctx, query, group.UserID, group.Name, group.Description))
}

func (r *SourceGroupRepository) Update(ctx context.Context, group domain.SourceGroup) (*domain.SourceGroup, error) {
	const query = `
		UPDATE source_groups
		SET name = $3, description = $4, updated_at = now()
		WHERE user_id = $1 AND id = $2
		RETURNING id, user_id, name, description, created_at, updated_at
	`
	updated, err := scanSourceGroup(r.db.QueryRowContext(ctx, query, group.UserID, group.ID, group.Name, group.Description))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update source group: %w", err)
	}
	return updated, nil
}

func (r *SourceGroupRepository) Delete(ctx context.Context, userID, groupID int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM source_groups WHERE user_id = $1 AND id = $2`, userID, groupID); err != nil {
		return fmt.Errorf("delete source group: %w", err)
	}
	return nil
}

func (r *SourceGroupRepository) ListByUserID(ctx context.Context, userID int64) ([]domain.SourceGroup, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, description, created_at, updated_at
		FROM source_groups
		WHERE user_id = $1
		ORDER BY name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list source groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var groups []domain.SourceGroup
	for rows.Next() {
		group, err := scanSourceGroup(rows)
		if err != nil {
			return nil, fmt.Errorf("scan source group: %w", err)
		}
		groups = append(groups, *group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source groups: %w", err)
	}
	return groups, nil
}

func (r *SourceGroupRepository) AddChat(ctx context.Context, link domain.SourceGroupChat) error {
	const query = `
		INSERT INTO source_group_chats (group_id, chat_id, priority, enabled)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (group_id, chat_id)
		DO UPDATE SET priority = EXCLUDED.priority, enabled = EXCLUDED.enabled
	`
	if _, err := r.db.ExecContext(ctx, query, link.GroupID, link.ChatID, link.Priority, link.Enabled); err != nil {
		return fmt.Errorf("add source group chat: %w", err)
	}
	return nil
}

func (r *SourceGroupRepository) RemoveChat(ctx context.Context, groupID, chatID int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM source_group_chats WHERE group_id = $1 AND chat_id = $2`, groupID, chatID); err != nil {
		return fmt.Errorf("remove source group chat: %w", err)
	}
	return nil
}

func (r *SourceGroupRepository) ListChats(ctx context.Context, groupID int64) ([]domain.SourceGroupChat, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT group_id, chat_id, priority, enabled
		FROM source_group_chats
		WHERE group_id = $1
		ORDER BY priority DESC, chat_id
	`, groupID)
	if err != nil {
		return nil, fmt.Errorf("list source group chats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var links []domain.SourceGroupChat
	for rows.Next() {
		var link domain.SourceGroupChat
		if err := rows.Scan(&link.GroupID, &link.ChatID, &link.Priority, &link.Enabled); err != nil {
			return nil, fmt.Errorf("scan source group chat: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source group chats: %w", err)
	}
	return links, nil
}

func scanSourceGroup(row interface {
	Scan(dest ...any) error
}) (*domain.SourceGroup, error) {
	var group domain.SourceGroup
	if err := row.Scan(&group.ID, &group.UserID, &group.Name, &group.Description, &group.CreatedAt, &group.UpdatedAt); err != nil {
		return nil, err
	}
	return &group, nil
}
