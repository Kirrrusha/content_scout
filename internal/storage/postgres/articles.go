package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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
		INSERT INTO articles (user_id, title, slug, type, status, tags, content_markdown)
		VALUES ($1, $2, $3, $4, $5, $6::text[], $7)
		RETURNING id, user_id, title, slug, type, status, tags::text, content_markdown, created_at, updated_at
	`, article.UserID, article.Title, article.Slug, article.Type, article.Status, pgTextArray(article.Tags), article.ContentMarkdown))
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
		SELECT id, user_id, title, slug, type, status, tags::text, content_markdown, created_at, updated_at
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

func (r *ArticleRepository) FindByUser(ctx context.Context, userID, articleID int64) (*domain.Article, error) {
	article, err := scanArticle(r.db.QueryRowContext(ctx, `
		SELECT id, user_id, title, slug, type, status, tags::text, content_markdown, created_at, updated_at
		FROM articles
		WHERE user_id = $1 AND id = $2
	`, userID, articleID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find article by user: %w", err)
	}
	return article, nil
}

func (r *ArticleRepository) FindBySlug(ctx context.Context, userID int64, slug string) (*domain.Article, error) {
	article, err := scanArticle(r.db.QueryRowContext(ctx, `
		SELECT id, user_id, title, slug, type, status, tags::text, content_markdown, created_at, updated_at
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

func (r *ArticleRepository) ListByUser(ctx context.Context, userID int64, limit int) ([]domain.Article, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, title, slug, type, status, tags::text, content_markdown, created_at, updated_at
		FROM articles
		WHERE user_id = $1
		ORDER BY updated_at DESC, id DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list articles by user: %w", err)
	}
	defer rows.Close()

	var articles []domain.Article
	for rows.Next() {
		article, err := scanArticle(rows)
		if err != nil {
			return nil, fmt.Errorf("scan article: %w", err)
		}
		articles = append(articles, *article)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate articles: %w", err)
	}
	return articles, nil
}

func (r *ArticleRepository) ListSources(ctx context.Context, articleID int64) ([]domain.ArticleSource, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, article_id, telegram_chat_id, message_id, source_title, source_url, published_at
		FROM article_sources
		WHERE article_id = $1
		ORDER BY published_at, id
	`, articleID)
	if err != nil {
		return nil, fmt.Errorf("list article sources: %w", err)
	}
	defer rows.Close()

	var sources []domain.ArticleSource
	for rows.Next() {
		var source domain.ArticleSource
		if err := rows.Scan(&source.ID, &source.ArticleID, &source.TelegramChatID, &source.MessageID, &source.SourceTitle, &source.SourceURL, &source.PublishedAt); err != nil {
			return nil, fmt.Errorf("scan article source: %w", err)
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate article sources: %w", err)
	}
	return sources, nil
}

func (r *ArticleRepository) Update(ctx context.Context, article domain.Article) (*domain.Article, error) {
	updated, err := scanArticle(r.db.QueryRowContext(ctx, `
		UPDATE articles
		SET title = $3, type = $4, status = $5, tags = $6::text[], content_markdown = $7, updated_at = now()
		WHERE user_id = $1 AND id = $2
		RETURNING id, user_id, title, slug, type, status, tags::text, content_markdown, created_at, updated_at
	`, article.UserID, article.ID, article.Title, article.Type, article.Status, pgTextArray(article.Tags), article.ContentMarkdown))
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
	var tags string
	if err := row.Scan(&article.ID, &article.UserID, &article.Title, &article.Slug, &article.Type, &article.Status, &tags, &article.ContentMarkdown, &article.CreatedAt, &article.UpdatedAt); err != nil {
		return nil, err
	}
	article.Tags = parsePGTextArray(tags)
	return &article, nil
}

func pgTextArray(values []string) string {
	if len(values) == 0 {
		return "{}"
	}
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		value = strings.ReplaceAll(value, `\`, `\\`)
		value = strings.ReplaceAll(value, `"`, `\"`)
		items = append(items, `"`+value+`"`)
	}
	if len(items) == 0 {
		return "{}"
	}
	return "{" + strings.Join(items, ",") + "}"
}

func parsePGTextArray(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "{}" {
		return nil
	}
	value = strings.TrimPrefix(strings.TrimSuffix(value, "}"), "{")
	var out []string
	var b strings.Builder
	inQuote := false
	escaped := false
	for _, r := range value {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inQuote = !inQuote
		case r == ',' && !inQuote:
			if item := strings.TrimSpace(b.String()); item != "" {
				out = append(out, item)
			}
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if item := strings.TrimSpace(b.String()); item != "" {
		out = append(out, item)
	}
	return out
}
