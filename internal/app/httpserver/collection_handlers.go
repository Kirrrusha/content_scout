package httpserver

import (
	"net/http"

	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type collectionRequest struct {
	TelegramUserID int64  `json:"telegram_user_id"`
	Mode           string `json:"mode"`
	Limit          int    `json:"limit"`
}

type collectionResponse struct {
	JobID         int64 `json:"job_id"`
	UserID        int64 `json:"user_id"`
	GroupID       int64 `json:"group_id"`
	ChatsCount    int   `json:"chats_count"`
	MessagesCount int   `json:"messages_count"`
}

func (s *Server) collectionGroupCreate(w http.ResponseWriter, r *http.Request) {
	if s.collector == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "message collection is not configured"})
		return
	}
	groupID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	var req collectionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.collector.CollectGroup(r.Context(), collection.Request{
		TelegramUserID: req.TelegramUserID,
		GroupID:        groupID,
		Mode:           domain.CollectionMode(req.Mode),
		Limit:          req.Limit,
	})
	if err != nil {
		s.writeGroupError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, collectionResponse{
		JobID:         result.JobID,
		UserID:        result.UserID,
		GroupID:       result.GroupID,
		ChatsCount:    result.ChatsCount,
		MessagesCount: result.MessagesCount,
	})
}
