package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/sourcegroups"
)

type groupRequest struct {
	TelegramUserID int64  `json:"telegram_user_id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
}

type groupChatRequest struct {
	TelegramUserID int64 `json:"telegram_user_id"`
	ChatID         int64 `json:"chat_id"`
	Priority       int   `json:"priority"`
	Enabled        *bool `json:"enabled,omitempty"`
}

type groupResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type groupWithChatsResponse struct {
	Group groupResponse  `json:"group"`
	Chats []chatResponse `json:"chats"`
}

func (s *Server) groupsList(w http.ResponseWriter, r *http.Request) {
	if !s.requireGroupController(w) {
		return
	}
	telegramUserID, ok := telegramUserIDFromQuery(w, r)
	if !ok {
		return
	}
	groups, err := s.groups.List(r.Context(), telegramUserID)
	if err != nil {
		s.writeGroupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, groupResponses(groups))
}

func (s *Server) groupsCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requireGroupController(w) {
		return
	}
	var req groupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	group, err := s.groups.Create(r.Context(), req.TelegramUserID, req.Name, req.Description)
	if err != nil {
		s.writeGroupError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, groupResponseFromDomain(*group))
}

func (s *Server) groupsUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requireGroupController(w) {
		return
	}
	groupID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	var req groupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	group, err := s.groups.Update(r.Context(), req.TelegramUserID, groupID, req.Name, req.Description)
	if err != nil {
		s.writeGroupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, groupResponseFromDomain(*group))
}

func (s *Server) groupsDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireGroupController(w) {
		return
	}
	groupID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	var req authOwnerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.groups.Delete(r.Context(), req.TelegramUserID, groupID); err != nil {
		s.writeGroupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) groupChatsList(w http.ResponseWriter, r *http.Request) {
	if !s.requireGroupController(w) {
		return
	}
	groupID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	telegramUserID, ok := telegramUserIDFromQuery(w, r)
	if !ok {
		return
	}
	group, err := s.groups.ListChats(r.Context(), telegramUserID, groupID)
	if err != nil {
		s.writeGroupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, groupWithChatsResponse{
		Group: groupResponseFromDomain(group.Group),
		Chats: chatResponses(group.Chats),
	})
}

func (s *Server) groupChatsAdd(w http.ResponseWriter, r *http.Request) {
	if !s.requireGroupController(w) {
		return
	}
	groupID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	var req groupChatRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if err := s.groups.AddChat(r.Context(), req.TelegramUserID, groupID, req.ChatID, req.Priority, enabled); err != nil {
		s.writeGroupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

func (s *Server) groupChatsRemove(w http.ResponseWriter, r *http.Request) {
	if !s.requireGroupController(w) {
		return
	}
	groupID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	chatID, ok := pathInt64(w, r, "chatId")
	if !ok {
		return
	}
	var req authOwnerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.groups.RemoveChat(r.Context(), req.TelegramUserID, groupID, chatID); err != nil {
		s.writeGroupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (s *Server) requireGroupController(w http.ResponseWriter) bool {
	if s.groups == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "source groups are not configured"})
		return false
	}
	return true
}

func (s *Server) writeGroupError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sourcegroups.ErrGroupNotFound), errors.Is(err, sourcegroups.ErrChatNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrBadRequest):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		s.writeAuthError(w, err)
	}
}

var ErrBadRequest = errors.New("bad request")

func pathInt64(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	value, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || value <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid " + name})
		return 0, false
	}
	return value, true
}

func pathInt(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	value, err := strconv.Atoi(r.PathValue(name))
	if err != nil || value <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid " + name})
		return 0, false
	}
	return value, true
}

func queryInt64(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	value, err := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	if err != nil || value <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid " + name})
		return 0, false
	}
	return value, true
}

func groupResponses(groups []domain.SourceGroup) []groupResponse {
	responses := make([]groupResponse, 0, len(groups))
	for _, group := range groups {
		responses = append(responses, groupResponseFromDomain(group))
	}
	return responses
}

func groupResponseFromDomain(group domain.SourceGroup) groupResponse {
	return groupResponse{ID: group.ID, Name: group.Name, Description: group.Description}
}
