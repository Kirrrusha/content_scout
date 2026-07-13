package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/obsidian"
)

func TestExportArticleHandler(t *testing.T) {
	articleID := int64(7)
	exports := &fakeHTTPExports{
		result: &obsidian.Result{
			Export: domain.ObsidianExport{
				ID:           1,
				ArticleID:    &articleID,
				FileName:     "Go Guide.md",
				VaultPath:    "Articles/go/Go Guide.md",
				ExportMethod: "telegram_document",
				ContentHash:  "hash",
				ExportedAt:   time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
			},
			Path: "/tmp/Go Guide.md",
		},
	}
	server := NewWithExports(":0", nil, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), nil, nil, nil, nil, nil, nil, nil, exports)

	req := httptest.NewRequest(http.MethodPost, "/exports/articles/7", bytes.NewBufferString(`{"telegram_user_id":42}`))
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if exports.articleUserID != 42 || exports.articleID != 7 {
		t.Fatalf("export user=%d article=%d", exports.articleUserID, exports.articleID)
	}
	var response exportResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.FileName != "Go Guide.md" || response.ContentHash != "hash" || response.Path == "" {
		t.Fatalf("response = %+v", response)
	}
}

type fakeHTTPExports struct {
	result        *obsidian.Result
	articleUserID int64
	articleID     int64
	summaryUserID int64
	summaryID     int64
}

func (f *fakeHTTPExports) ExportArticle(_ context.Context, telegramUserID, articleID int64) (*obsidian.Result, error) {
	f.articleUserID = telegramUserID
	f.articleID = articleID
	return f.result, nil
}

func (f *fakeHTTPExports) ExportSummary(_ context.Context, telegramUserID, summaryID int64) (*obsidian.Result, error) {
	f.summaryUserID = telegramUserID
	f.summaryID = summaryID
	return f.result, nil
}
