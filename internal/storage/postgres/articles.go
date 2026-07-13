package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type ArticleRepository struct {
	db *sql.DB
}

func NewArticleRepository(db *sql.DB) *ArticleRepository {
	return &ArticleRepository{db: db}
}

func (r *ArticleRepository) Create(ctx context.Context, article domain.Article, sources []domain.ArticleSource) (*domain.Article, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin create article: %w", err)
	}

	created, err := scanArticle(tx.QueryRowContext(ctx, `
		INSERT INTO articles (user_id, title, slug, type, status, content_markdown)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, user_id, title, slug, type, status, content_markdown, created_at, updated_at
	`, article.UserID, article.Title, article.Slug, article.Type, article.Status, article.ContentMarkdown))
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("insert article: %w", err)
	}

	for _, source := range sources {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO article_sources (article_id, telegram_chat_id, message_id, source_title, source_url, published_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, created.ID, source.TelegramChatID, source.MessageID, source.SourceTitle, source.SourceURL, source.PublishedAt); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("insert article source: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit create article: %w", err)
	}
	return created, nil
}

func (r *ArticleRepository) Find(ctx context.Context, articleID int64) (*domain.Article, error) {
	article, err := scanArticle(r.db.QueryRowContext(ctx, `
		SELECT id, user_id, title, slug, type, status, content_markdown, created_at, updated_at
		FROM articles
		WHERE id = $1
	`, articleID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find article: %w", err)
	}
	return article, nil
}

func (r *ArticleRepository) FindBySlug(ctx context.Context, userID int64, slug string) (*domain.Article, error) {
	article, err := scanArticle(r.db.QueryRowContext(ctx, `
		SELECT id, user_id, title, slug, type, status, content_markdown, created_at, updated_at
		FROM articles
		WHERE user_id = $1 AND slug = $2
	`, userID, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find article by slug: %w", err)
	}
	return article, nil
}

func (r *ArticleRepository) Update(ctx context.Context, article domain.Article) (*domain.Article, error) {
	updated, err := scanArticle(r.db.QueryRowContext(ctx, `
		UPDATE articles
		SET title = $3, type = $4, status = $5, content_markdown = $6, updated_at = now()
		WHERE user_id = $1 AND id = $2
		RETURNING id, user_id, title, slug, type, status, content_markdown, created_at, updated_at
	`, article.UserID, article.ID, article.Title, article.Type, article.Status, article.ContentMarkdown))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update article: %w", err)
	}
	return updated, nil
}

func scanArticle(row interface {
	Scan(dest ...any) error
}) (*domain.Article, error) {
	var article domain.Article
	if err := row.Scan(&article.ID, &article.UserID, &article.Title, &article.Slug, &article.Type, &article.Status, &article.ContentMarkdown, &article.CreatedAt, &article.UpdatedAt); err != nil {
		return nil, err
	}
	return &article, nil
}
