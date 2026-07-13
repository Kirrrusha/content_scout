package httpserver

import (
	"errors"
	"net/http"

	"github.com/kirilllebedenko/content_scout/internal/article"
	"github.com/kirilllebedenko/content_scout/internal/obsidian"
	"github.com/kirilllebedenko/content_scout/internal/summary"
)

type exportRequest struct {
	TelegramUserID int64 `json:"telegram_user_id"`
}

type exportResponse struct {
	ID           int64  `json:"id"`
	ArticleID    *int64 `json:"article_id,omitempty"`
	SummaryID    *int64 `json:"summary_id,omitempty"`
	FileName     string `json:"file_name"`
	VaultPath    string `json:"vault_path"`
	Path         string `json:"path"`
	ExportMethod string `json:"export_method"`
	ContentHash  string `json:"content_hash"`
	Reused       bool   `json:"reused"`
	ExportedAt   string `json:"exported_at"`
}

func (s *Server) exportArticle(w http.ResponseWriter, r *http.Request) {
	if s.exports == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "obsidian export is not configured"})
		return
	}
	articleID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	var req exportRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.exports.ExportArticle(r.Context(), req.TelegramUserID, articleID)
	if s.writeExportError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, exportResponseFromResult(result))
}

func (s *Server) exportSummary(w http.ResponseWriter, r *http.Request) {
	if s.exports == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "obsidian export is not configured"})
		return
	}
	summaryID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	var req exportRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.exports.ExportSummary(r.Context(), req.TelegramUserID, summaryID)
	if s.writeExportError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, exportResponseFromResult(result))
}

func (s *Server) writeExportError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, article.ErrArticleNotFound), errors.Is(err, summary.ErrSummaryNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		s.writeAuthError(w, err)
	}
	return true
}

func exportResponseFromResult(result *obsidian.Result) exportResponse {
	export := result.Export
	return exportResponse{
		ID:           export.ID,
		ArticleID:    export.ArticleID,
		SummaryID:    export.SummaryID,
		FileName:     export.FileName,
		VaultPath:    export.VaultPath,
		Path:         result.Path,
		ExportMethod: export.ExportMethod,
		ContentHash:  export.ContentHash,
		Reused:       result.Reused,
		ExportedAt:   export.ExportedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
