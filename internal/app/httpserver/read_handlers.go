package httpserver

import (
	"context"
	"net/http"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

type TelegramReadController interface {
	MarkCollectedMessagesRead(ctx context.Context, telegramUserID int64, messages []domain.CollectedMessage) error
}

func (s *Server) telegramMessagesRead(w http.ResponseWriter, r *http.Request) {
	if s.telegramRead == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "telegram read service is not configured"})
		return
	}
	var req tdlib.ReadMessagesRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.telegramRead.MarkCollectedMessagesRead(r.Context(), req.TelegramUserID, tdlib.CollectedMessagesFromReadRequest(req)); err != nil {
		s.logger.Error("mark telegram messages read failed", "telegram_user_id", req.TelegramUserID, "error", err)
		s.writeAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
