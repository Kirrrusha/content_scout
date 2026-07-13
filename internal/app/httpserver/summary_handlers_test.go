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
)

func TestSummariesListHandler(t *testing.T) {
	browser := &fakeHTTPSummaryBrowser{
		summaries: []domain.Summary{{
			ID:            10,
			Title:         "Digest",
			Overview:      "Weekly overview",
			MessagesCount: 12,
			SourcesCount:  3,
			TopicsCount:   2,
			CreatedAt:     time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
		}},
	}
	server := NewWithBrowser(":0", nil, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), nil, nil, nil, nil, nil, browser)

	req := httptest.NewRequest(http.MethodGet, "/summaries?telegram_user_id=42&limit=5", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if browser.listUserID != 42 || browser.listLimit != 5 {
		t.Fatalf("list user=%d limit=%d", browser.listUserID, browser.listLimit)
	}
	var response []summaryItemResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response) != 1 || response[0].ID != 10 || response[0].Markdown != "" {
		t.Fatalf("response = %+v", response)
	}
}

func TestSummaryTopicsHandler(t *testing.T) {
	browser := &fakeHTTPSummaryBrowser{
		topics: []domain.SummaryTopic{{
			ID:           1,
			SummaryID:    10,
			Title:        "Go",
			ShortSummary: "Short",
			FullSummary:  "Full",
			Category:     "Tech",
			Importance:   8,
			Confidence:   domain.ConfidenceHigh,
			Position:     1,
		}},
	}
	server := NewWithBrowser(":0", nil, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), nil, nil, nil, nil, nil, browser)

	req := httptest.NewRequest(http.MethodGet, "/summaries/10/topics?telegram_user_id=42", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if browser.topicsUserID != 42 || browser.topicsSummaryID != 10 {
		t.Fatalf("topics user=%d summary=%d", browser.topicsUserID, browser.topicsSummaryID)
	}
	var response []summaryTopicResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response) != 1 || response[0].Title != "Go" || response[0].Confidence != "high" {
		t.Fatalf("response = %+v", response)
	}
}

type fakeHTTPSummaryBrowser struct {
	summaries       []domain.Summary
	summary         *domain.Summary
	topics          []domain.SummaryTopic
	listUserID      int64
	listLimit       int
	getUserID       int64
	getSummaryID    int64
	topicsUserID    int64
	topicsSummaryID int64
}

func (f *fakeHTTPSummaryBrowser) ListSummaries(_ context.Context, telegramUserID int64, limit int) ([]domain.Summary, error) {
	f.listUserID = telegramUserID
	f.listLimit = limit
	return f.summaries, nil
}

func (f *fakeHTTPSummaryBrowser) GetSummary(_ context.Context, telegramUserID, summaryID int64) (*domain.Summary, error) {
	f.getUserID = telegramUserID
	f.getSummaryID = summaryID
	if f.summary != nil {
		return f.summary, nil
	}
	return &domain.Summary{ID: summaryID, Title: "Digest"}, nil
}

func (f *fakeHTTPSummaryBrowser) ListTopics(_ context.Context, telegramUserID, summaryID int64) ([]domain.SummaryTopic, error) {
	f.topicsUserID = telegramUserID
	f.topicsSummaryID = summaryID
	return f.topics, nil
}
