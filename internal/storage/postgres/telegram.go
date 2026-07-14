package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type TelegramSessionRepository struct {
	db *sql.DB
}

func NewTelegramSessionRepository(db *sql.DB) *TelegramSessionRepository {
	return &TelegramSessionRepository{db: db}
}

func (r *TelegramSessionRepository) Upsert(ctx context.Context, session domain.TelegramSession) (*domain.TelegramSession, error) {
	const query = `
		INSERT INTO telegram_sessions (user_id, storage_path, status, last_connected)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id)
		DO UPDATE SET storage_path = EXCLUDED.storage_path, status = EXCLUDED.status, last_connected = EXCLUDED.last_connected, updated_at = now()
		RETURNING id, user_id, storage_path, status, last_connected, created_at, updated_at
	`
	return scanTelegramSession(r.db.QueryRowContext(ctx, query, session.UserID, session.StoragePath, session.Status, nullTime(session.LastConnected)))
}

func (r *TelegramSessionRepository) FindByUserID(ctx context.Context, userID int64) (*domain.TelegramSession, error) {
	const query = `
		SELECT id, user_id, storage_path, status, last_connected, created_at, updated_at
		FROM telegram_sessions
		WHERE user_id = $1
	`
	session, err := scanTelegramSession(r.db.QueryRowContext(ctx, query, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find telegram session by user id: %w", err)
	}
	return session, nil
}

func (r *TelegramSessionRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM telegram_sessions WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete telegram session by user id: %w", err)
	}
	return nil
}

func scanTelegramSession(row interface {
	Scan(dest ...any) error
}) (*domain.TelegramSession, error) {
	var session domain.TelegramSession
	var lastConnected sql.NullTime
	if err := row.Scan(&session.ID, &session.UserID, &session.StoragePath, &session.Status, &lastConnected, &session.CreatedAt, &session.UpdatedAt); err != nil {
		return nil, err
	}
	session.LastConnected = timePtr(lastConnected)
	return &session, nil
}

type TelegramFolderRepository struct {
	db *sql.DB
}

func NewTelegramFolderRepository(db *sql.DB) *TelegramFolderRepository {
	return &TelegramFolderRepository{db: db}
}

func (r *TelegramFolderRepository) UpsertMany(ctx context.Context, folders []domain.TelegramFolder) error {
	const query = `
		INSERT INTO telegram_folders (user_id, telegram_id, name, synced_at)
		VALUES ($1, $2, $3, COALESCE(NULLIF($4::timestamptz, '0001-01-01'::timestamptz), now()))
		ON CONFLICT (user_id, telegram_id)
		DO UPDATE SET name = EXCLUDED.name, synced_at = EXCLUDED.synced_at
	`
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert telegram folders: %w", err)
	}
	for _, folder := range folders {
		if _, err := tx.ExecContext(ctx, query, folder.UserID, folder.TelegramID, folder.Name, folder.SyncedAt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("upsert telegram folder: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert telegram folders: %w", err)
	}
	return nil
}

func (r *TelegramFolderRepository) ListByUserID(ctx context.Context, userID int64) ([]domain.TelegramFolder, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, telegram_id, name, synced_at
		FROM telegram_folders
		WHERE user_id = $1
		ORDER BY telegram_id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list telegram folders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var folders []domain.TelegramFolder
	for rows.Next() {
		var folder domain.TelegramFolder
		if err := rows.Scan(&folder.ID, &folder.UserID, &folder.TelegramID, &folder.Name, &folder.SyncedAt); err != nil {
			return nil, fmt.Errorf("scan telegram folder: %w", err)
		}
		folders = append(folders, folder)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate telegram folders: %w", err)
	}
	return folders, nil
}

type TelegramChatRepository struct {
	db *sql.DB
}

func NewTelegramChatRepository(db *sql.DB) *TelegramChatRepository {
	return &TelegramChatRepository{db: db}
}

func (r *TelegramChatRepository) UpsertMany(ctx context.Context, chats []domain.TelegramChat) error {
	const query = `
		INSERT INTO telegram_chats (user_id, telegram_chat_id, title, username, type, is_archived, is_muted, unread_count, last_message_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		ON CONFLICT (user_id, telegram_chat_id)
		DO UPDATE SET title = EXCLUDED.title, username = EXCLUDED.username, type = EXCLUDED.type,
			is_archived = EXCLUDED.is_archived, is_muted = EXCLUDED.is_muted,
			unread_count = EXCLUDED.unread_count, last_message_id = EXCLUDED.last_message_id, updated_at = now()
	`
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert telegram chats: %w", err)
	}
	for _, chat := range chats {
		if _, err := tx.ExecContext(ctx, query, chat.UserID, chat.TelegramChatID, chat.Title, nullString(chat.Username), chat.Type, chat.IsArchived, chat.IsMuted, chat.UnreadCount, chat.LastMessageID); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("upsert telegram chat: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert telegram chats: %w", err)
	}
	return nil
}

func (r *TelegramChatRepository) ListByUserID(ctx context.Context, userID int64) ([]domain.TelegramChat, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, telegram_chat_id, title, username, type, is_archived, is_muted, unread_count, last_message_id, updated_at
		FROM telegram_chats
		WHERE user_id = $1
		ORDER BY title
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list telegram chats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var chats []domain.TelegramChat
	for rows.Next() {
		chat, err := scanTelegramChat(rows)
		if err != nil {
			return nil, fmt.Errorf("scan telegram chat: %w", err)
		}
		chats = append(chats, *chat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate telegram chats: %w", err)
	}
	return chats, nil
}

func (r *TelegramChatRepository) FindByTelegramChatID(ctx context.Context, userID, telegramChatID int64) (*domain.TelegramChat, error) {
	chat, err := scanTelegramChat(r.db.QueryRowContext(ctx, `
		SELECT id, user_id, telegram_chat_id, title, username, type, is_archived, is_muted, unread_count, last_message_id, updated_at
		FROM telegram_chats
		WHERE user_id = $1 AND telegram_chat_id = $2
	`, userID, telegramChatID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find telegram chat: %w", err)
	}
	return chat, nil
}

func scanTelegramChat(row interface {
	Scan(dest ...any) error
}) (*domain.TelegramChat, error) {
	var chat domain.TelegramChat
	var username sql.NullString
	if err := row.Scan(&chat.ID, &chat.UserID, &chat.TelegramChatID, &chat.Title, &username, &chat.Type, &chat.IsArchived, &chat.IsMuted, &chat.UnreadCount, &chat.LastMessageID, &chat.UpdatedAt); err != nil {
		return nil, err
	}
	chat.Username = stringPtr(username)
	return &chat, nil
}
