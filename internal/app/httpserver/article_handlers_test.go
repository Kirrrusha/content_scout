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

	"github.com/kirilllebedenko/content_scout/internal/article"
	"github.com/kirilllebedenko/content_scout/internal/domain"
)

func TestArticleFromTopicHandler(t *testing.T) {
	articles := &fakeHTTPArticles{
		result: &article.Result{
			Article: domain.Article{ID: 7, Title: "Go Guide", Slug: "go-guide", Type: domain.ArticleTypeGuide, Status: domain.ArticleStatusDraft, Tags: []string{"go"}, ContentMarkdown: "# Go Guide", CreatedAt: time.Now(), UpdatedAt: time.Now()},
			Sources: 2,
		},
	}
	server := NewWithArticle(":0", nil, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), nil, nil, nil, nil, nil, nil, articles)

	req := httptest.NewRequest(http.MethodPost, "/articles/from-summary/10/topics/2", bytes.NewBufferString(`{"telegram_user_id":42,"type":"guide","tags":["go"]}`))
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if articles.topicRequest.SummaryID != 10 || articles.topicRequest.TopicPosition != 2 || articles.topicRequest.Type != domain.ArticleTypeGuide {
		t.Fatalf("request = %+v", articles.topicRequest)
	}
	var response articleConvertResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Article.ID != 7 || response.Sources != 2 || response.Article.ContentMarkdown == "" {
		t.Fatalf("response = %+v", response)
	}
}

func TestArticleUpdateHandler(t *testing.T) {
	articles := &fakeHTTPArticles{
		updated: &domain.Article{ID: 7, Title: "New", Slug: "old", Type: domain.ArticleTypeAnalysis, Status: domain.ArticleStatusDraft, Tags: []string{"ai"}, ContentMarkdown: "# Old", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	server := NewWithArticle(":0", nil, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), nil, nil, nil, nil, nil, nil, articles)

	req := httptest.NewRequest(http.MethodPatch, "/articles/7", bytes.NewBufferString(`{"telegram_user_id":42,"title":"New","tags":["ai"]}`))
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if articles.updateUserID != 42 || articles.updateArticleID != 7 || articles.updateTitle != "New" {
		t.Fatalf("update user=%d article=%d title=%q", articles.updateUserID, articles.updateArticleID, articles.updateTitle)
	}
	var response articleResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Title != "New" || len(response.Tags) != 1 || response.Tags[0] != "ai" {
		t.Fatalf("response = %+v", response)
	}
}

type fakeHTTPArticles struct {
	result          *article.Result
	updated         *domain.Article
	topicRequest    article.ConvertRequest
	summaryRequest  article.ConvertRequest
	updateUserID    int64
	updateArticleID int64
	updateTitle     string
}

func (f *fakeHTTPArticles) ConvertSummary(_ context.Context, req article.ConvertRequest) (*article.Result, error) {
	f.summaryRequest = req
	return f.result, nil
}

func (f *fakeHTTPArticles) ConvertTopic(_ context.Context, req article.ConvertRequest) (*article.Result, error) {
	f.topicRequest = req
	return f.result, nil
}

func (f *fakeHTTPArticles) List(context.Context, int64, int) ([]domain.Article, error) {
	return nil, nil
}

func (f *fakeHTTPArticles) Get(context.Context, int64, int64) (*domain.Article, error) {
	return nil, nil
}

func (f *fakeHTTPArticles) UpdateMetadata(_ context.Context, telegramUserID, articleID int64, title string, tags []string) (*domain.Article, error) {
	f.updateUserID = telegramUserID
	f.updateArticleID = articleID
	f.updateTitle = title
	if f.updated != nil {
		return f.updated, nil
	}
	return &domain.Article{ID: articleID, Title: title, Tags: tags}, nil
}
