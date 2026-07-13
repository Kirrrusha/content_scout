package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type ObsidianExportRepository struct {
	db *sql.DB
}

func NewObsidianExportRepository(db *sql.DB) *ObsidianExportRepository {
	return &ObsidianExportRepository{db: db}
}

func (r *ObsidianExportRepository) Create(ctx context.Context, export domain.ObsidianExport) (*domain.ObsidianExport, error) {
	const query = `
		INSERT INTO obsidian_exports (article_id, summary_id, file_name, vault_path, export_method, content_hash)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, article_id, summary_id, file_name, vault_path, export_method, content_hash, exported_at
	`
	return scanObsidianExport(r.db.QueryRowContext(ctx, query, nullInt64(export.ArticleID), nullInt64(export.SummaryID), export.FileName, export.VaultPath, export.ExportMethod, export.ContentHash))
}

func (r *ObsidianExportRepository) FindByContentHash(ctx context.Context, hash string) (*domain.ObsidianExport, error) {
	export, err := scanObsidianExport(r.db.QueryRowContext(ctx, `
		SELECT id, article_id, summary_id, file_name, vault_path, export_method, content_hash, exported_at
		FROM obsidian_exports
		WHERE content_hash = $1
	`, hash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find obsidian export by content hash: %w", err)
	}
	return export, nil
}

func scanObsidianExport(row interface {
	Scan(dest ...any) error
}) (*domain.ObsidianExport, error) {
	var export domain.ObsidianExport
	var articleID sql.NullInt64
	var summaryID sql.NullInt64
	if err := row.Scan(&export.ID, &articleID, &summaryID, &export.FileName, &export.VaultPath, &export.ExportMethod, &export.ContentHash, &export.ExportedAt); err != nil {
		return nil, err
	}
	export.ArticleID = int64Ptr(articleID)
	export.SummaryID = int64Ptr(summaryID)
	return &export, nil
}
