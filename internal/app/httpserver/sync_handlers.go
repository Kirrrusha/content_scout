package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

type syncResponse struct {
	UserID       int64     `json:"user_id"`
	FoldersCount int       `json:"folders_count"`
	ChatsCount   int       `json:"chats_count"`
	SyncedAt     time.Time `json:"synced_at"`
}

type folderResponse struct {
	ID         int64     `json:"id"`
	TelegramID int32     `json:"telegram_id"`
	Name       string    `json:"name"`
	SyncedAt   time.Time `json:"synced_at"`
}

type chatResponse struct {
	ID             int64   `json:"id"`
	TelegramChatID int64   `json:"telegram_chat_id"`
	Title          string  `json:"title"`
	Username       *string `json:"username,omitempty"`
	Type           string  `json:"type"`
	IsArchived     bool    `json:"is_archived"`
	IsMuted        bool    `json:"is_muted"`
	UnreadCount    int     `json:"unread_count"`
	LastMessageID  int64   `json:"last_message_id"`
}

func (s *Server) telegramSync(w http.ResponseWriter, r *http.Request) {
	if !s.requireSyncController(w) {
		return
	}
	var req authOwnerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.sync.Sync(r.Context(), req.TelegramUserID)
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, syncResponseFromResult(result))
}

func (s *Server) telegramFolders(w http.ResponseWriter, r *http.Request) {
	if !s.requireSyncController(w) {
		return
	}
	telegramUserID, ok := telegramUserIDFromQuery(w, r)
	if !ok {
		return
	}
	folders, err := s.sync.ListFolders(r.Context(), telegramUserID)
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, folderResponses(folders))
}

func (s *Server) telegramChats(w http.ResponseWriter, r *http.Request) {
	if !s.requireSyncController(w) {
		return
	}
	telegramUserID, ok := telegramUserIDFromQuery(w, r)
	if !ok {
		return
	}
	chats, err := s.sync.ListChats(r.Context(), telegramUserID)
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, chatResponses(chats))
}

func (s *Server) requireSyncController(w http.ResponseWriter) bool {
	if s.sync == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "telegram sync is not configured"})
		return false
	}
	return true
}

func telegramUserIDFromQuery(w http.ResponseWriter, r *http.Request) (int64, bool) {
	telegramUserID, err := strconv.ParseInt(r.URL.Query().Get("telegram_user_id"), 10, 64)
	if err != nil || telegramUserID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "telegram_user_id is required"})
		return 0, false
	}
	return telegramUserID, true
}

func syncResponseFromResult(result *tdlib.SyncResult) syncResponse {
	if result == nil {
		return syncResponse{}
	}
	return syncResponse{
		UserID:       result.UserID,
		FoldersCount: result.FoldersCount,
		ChatsCount:   result.ChatsCount,
		SyncedAt:     result.SyncedAt,
	}
}

func folderResponses(folders []domain.TelegramFolder) []folderResponse {
	responses := make([]folderResponse, 0, len(folders))
	for _, folder := range folders {
		responses = append(responses, folderResponse{
			ID:         folder.ID,
			TelegramID: folder.TelegramID,
			Name:       folder.Name,
			SyncedAt:   folder.SyncedAt,
		})
	}
	return responses
}

func chatResponses(chats []domain.TelegramChat) []chatResponse {
	responses := make([]chatResponse, 0, len(chats))
	for _, chat := range chats {
		responses = append(responses, chatResponse{
			ID:             chat.ID,
			TelegramChatID: chat.TelegramChatID,
			Title:          chat.Title,
			Username:       chat.Username,
			Type:           string(chat.Type),
			IsArchived:     chat.IsArchived,
			IsMuted:        chat.IsMuted,
			UnreadCount:    chat.UnreadCount,
			LastMessageID:  chat.LastMessageID,
		})
	}
	return responses
}
