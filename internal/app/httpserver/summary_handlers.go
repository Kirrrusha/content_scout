package httpserver

import (
	"net/http"

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
