package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/kirilllebedenko/content_scout/internal/article"
	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/summary"
)

type articleConvertRequest struct {
	TelegramUserID int64    `json:"telegram_user_id"`
	Type           string   `json:"type"`
	Title          string   `json:"title"`
	Tags           []string `json:"tags"`
}

type articleUpdateRequest struct {
	TelegramUserID int64    `json:"telegram_user_id"`
	Title          string   `json:"title"`
	Tags           []string `json:"tags"`
}

type articleResponse struct {
	ID              int64    `json:"id"`
	Title           string   `json:"title"`
	Slug            string   `json:"slug"`
	Type            string   `json:"type"`
	Status          string   `json:"status"`
	Tags            []string `json:"tags"`
	ContentMarkdown string   `json:"content_markdown,omitempty"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

type articleConvertResponse struct {
	Article articleResponse `json:"article"`
	Sources int             `json:"sources"`
}

func (s *Server) articleFromSummary(w http.ResponseWriter, r *http.Request) {
	if s.articles == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "article conversion is not configured"})
		return
	}
	summaryID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	var req articleConvertRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.articles.ConvertSummary(r.Context(), article.ConvertRequest{
		TelegramUserID: req.TelegramUserID,
		SummaryID:      summaryID,
		Type:           domain.ArticleType(req.Type),
		Title:          req.Title,
		Tags:           req.Tags,
	})
	if s.writeArticleError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, articleConvertResponse{Article: articleResponseFromDomain(result.Article, true), Sources: result.Sources})
}

func (s *Server) articleFromTopic(w http.ResponseWriter, r *http.Request) {
	if s.articles == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "article conversion is not configured"})
		return
	}
	summaryID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	position, ok := pathInt(w, r, "position")
	if !ok {
		return
	}
	var req articleConvertRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.articles.ConvertTopic(r.Context(), article.ConvertRequest{
		TelegramUserID: req.TelegramUserID,
		SummaryID:      summaryID,
		TopicPosition:  position,
		Type:           domain.ArticleType(req.Type),
		Title:          req.Title,
		Tags:           req.Tags,
	})
	if s.writeArticleError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, articleConvertResponse{Article: articleResponseFromDomain(result.Article, true), Sources: result.Sources})
}

func (s *Server) articlesList(w http.ResponseWriter, r *http.Request) {
	if s.articles == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "article conversion is not configured"})
		return
	}
	telegramUserID, ok := queryInt64(w, r, "telegram_user_id")
	if !ok {
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a non-negative integer"})
			return
		}
		limit = parsed
	}
	articles, err := s.articles.List(r.Context(), telegramUserID, limit)
	if s.writeArticleError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, articleResponses(articles, false))
}

func (s *Server) articleGet(w http.ResponseWriter, r *http.Request) {
	if s.articles == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "article conversion is not configured"})
		return
	}
	telegramUserID, ok := queryInt64(w, r, "telegram_user_id")
	if !ok {
		return
	}
	articleID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	item, err := s.articles.Get(r.Context(), telegramUserID, articleID)
	if s.writeArticleError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, articleResponseFromDomain(*item, true))
}

func (s *Server) articleUpdate(w http.ResponseWriter, r *http.Request) {
	if s.articles == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "article conversion is not configured"})
		return
	}
	articleID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	var req articleUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	item, err := s.articles.UpdateMetadata(r.Context(), req.TelegramUserID, articleID, req.Title, req.Tags)
	if s.writeArticleError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, articleResponseFromDomain(*item, true))
}

func (s *Server) writeArticleError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, article.ErrArticleNotFound), errors.Is(err, summary.ErrSummaryNotFound), errors.Is(err, summary.ErrTopicNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		s.writeAuthError(w, err)
	}
	return true
}

func articleResponses(articles []domain.Article, includeMarkdown bool) []articleResponse {
	responses := make([]articleResponse, 0, len(articles))
	for _, item := range articles {
		responses = append(responses, articleResponseFromDomain(item, includeMarkdown))
	}
	return responses
}

func articleResponseFromDomain(item domain.Article, includeMarkdown bool) articleResponse {
	response := articleResponse{
		ID:        item.ID,
		Title:     item.Title,
		Slug:      item.Slug,
		Type:      string(item.Type),
		Status:    string(item.Status),
		Tags:      item.Tags,
		CreatedAt: item.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: item.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if includeMarkdown {
		response.ContentMarkdown = item.ContentMarkdown
	}
	return response
}
