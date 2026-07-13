package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/summary"
)

type summaryFromCollectionRequest struct {
	TelegramUserID int64  `json:"telegram_user_id"`
	Format         string `json:"format"`
}

type summaryResponse struct {
	SummaryID      int64 `json:"summary_id"`
	SummaryJobID   int64 `json:"summary_job_id"`
	TopicsCount    int   `json:"topics_count"`
	MessagesCount  int   `json:"messages_count"`
	DuplicateCount int   `json:"duplicate_count"`
}

type summaryItemResponse struct {
	ID            int64  `json:"id"`
	JobID         int64  `json:"job_id"`
	Title         string `json:"title"`
	Overview      string `json:"overview"`
	MessagesCount int    `json:"messages_count"`
	SourcesCount  int    `json:"sources_count"`
	TopicsCount   int    `json:"topics_count"`
	Markdown      string `json:"markdown,omitempty"`
	CreatedAt     string `json:"created_at"`
}

type summaryTopicResponse struct {
	ID            int64  `json:"id"`
	SummaryID     int64  `json:"summary_id"`
	Title         string `json:"title"`
	ShortSummary  string `json:"short_summary"`
	FullSummary   string `json:"full_summary"`
	Category      string `json:"category"`
	Importance    int    `json:"importance"`
	Confidence    string `json:"confidence"`
	MessagesCount int    `json:"messages_count"`
	SourcesCount  int    `json:"sources_count"`
	Position      int    `json:"position"`
}

func (s *Server) summaryFromCollection(w http.ResponseWriter, r *http.Request) {
	if s.summary == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summary generation is not configured"})
		return
	}
	collectionJobID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	var req summaryFromCollectionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.summary.GenerateFromCollection(r.Context(), summary.GenerateRequest{
		TelegramUserID:  req.TelegramUserID,
		CollectionJobID: collectionJobID,
		Format:          req.Format,
	})
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, summaryResponse{
		SummaryID:      result.SummaryID,
		SummaryJobID:   result.SummaryJobID,
		TopicsCount:    result.TopicsCount,
		MessagesCount:  result.MessagesCount,
		DuplicateCount: result.DuplicateCount,
	})
}

func (s *Server) summariesList(w http.ResponseWriter, r *http.Request) {
	if s.browser == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summary browser is not configured"})
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
	summaries, err := s.browser.ListSummaries(r.Context(), telegramUserID, limit)
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summaryItemResponses(summaries, false))
}

func (s *Server) summaryGet(w http.ResponseWriter, r *http.Request) {
	if s.browser == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summary browser is not configured"})
		return
	}
	telegramUserID, ok := queryInt64(w, r, "telegram_user_id")
	if !ok {
		return
	}
	summaryID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	item, err := s.browser.GetSummary(r.Context(), telegramUserID, summaryID)
	if errors.Is(err, summary.ErrSummaryNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "summary not found"})
		return
	}
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summaryItemResponseFromDomain(*item, true))
}

func (s *Server) summaryTopics(w http.ResponseWriter, r *http.Request) {
	if s.browser == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summary browser is not configured"})
		return
	}
	telegramUserID, ok := queryInt64(w, r, "telegram_user_id")
	if !ok {
		return
	}
	summaryID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	topics, err := s.browser.ListTopics(r.Context(), telegramUserID, summaryID)
	if errors.Is(err, summary.ErrSummaryNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "summary not found"})
		return
	}
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summaryTopicResponses(topics))
}

func summaryItemResponses(summaries []domain.Summary, includeMarkdown bool) []summaryItemResponse {
	responses := make([]summaryItemResponse, 0, len(summaries))
	for _, item := range summaries {
		responses = append(responses, summaryItemResponseFromDomain(item, includeMarkdown))
	}
	return responses
}

func summaryItemResponseFromDomain(item domain.Summary, includeMarkdown bool) summaryItemResponse {
	response := summaryItemResponse{
		ID:            item.ID,
		JobID:         item.JobID,
		Title:         item.Title,
		Overview:      item.Overview,
		MessagesCount: item.MessagesCount,
		SourcesCount:  item.SourcesCount,
		TopicsCount:   item.TopicsCount,
		CreatedAt:     item.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if includeMarkdown {
		response.Markdown = item.Markdown
	}
	return response
}

func summaryTopicResponses(topics []domain.SummaryTopic) []summaryTopicResponse {
	responses := make([]summaryTopicResponse, 0, len(topics))
	for _, topic := range topics {
		responses = append(responses, summaryTopicResponse{
			ID:            topic.ID,
			SummaryID:     topic.SummaryID,
			Title:         topic.Title,
			ShortSummary:  topic.ShortSummary,
			FullSummary:   topic.FullSummary,
			Category:      topic.Category,
			Importance:    topic.Importance,
			Confidence:    string(topic.Confidence),
			MessagesCount: topic.MessagesCount,
			SourcesCount:  topic.SourcesCount,
			Position:      topic.Position,
		})
	}
	return responses
}
