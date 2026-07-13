package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

type authOwnerRequest struct {
	TelegramUserID int64 `json:"telegram_user_id"`
}

type authPhoneRequest struct {
	TelegramUserID int64  `json:"telegram_user_id"`
	Phone          string `json:"phone"`
}

type authCodeRequest struct {
	TelegramUserID int64  `json:"telegram_user_id"`
	Code           string `json:"code"`
}

type authPasswordRequest struct {
	TelegramUserID int64  `json:"telegram_user_id"`
	Password       string `json:"password"`
}

type authStatusResponse struct {
	UserID       int64  `json:"user_id,omitempty"`
	SessionID    int64  `json:"session_id,omitempty"`
	SessionState string `json:"session_state"`
	AuthState    string `json:"auth_state"`
}

func (s *Server) authStart(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuthController(w) {
		return
	}
	var req authOwnerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	status, err := s.auth.Start(r.Context(), req.TelegramUserID)
	s.writeAuthStatus(w, status, err)
}

func (s *Server) authPhone(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuthController(w) {
		return
	}
	var req authPhoneRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	status, err := s.auth.SubmitPhoneNumber(r.Context(), req.TelegramUserID, req.Phone)
	s.writeAuthStatus(w, status, err)
}

func (s *Server) authCode(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuthController(w) {
		return
	}
	var req authCodeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	status, err := s.auth.SubmitCode(r.Context(), req.TelegramUserID, req.Code)
	s.writeAuthStatus(w, status, err)
}

func (s *Server) authPassword(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuthController(w) {
		return
	}
	var req authPasswordRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	status, err := s.auth.SubmitPassword(r.Context(), req.TelegramUserID, req.Password)
	s.writeAuthStatus(w, status, err)
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuthController(w) {
		return
	}
	telegramUserID, err := strconv.ParseInt(r.URL.Query().Get("telegram_user_id"), 10, 64)
	if err != nil || telegramUserID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "telegram_user_id is required"})
		return
	}
	status, err := s.auth.Status(r.Context(), telegramUserID)
	s.writeAuthStatus(w, status, err)
}

func (s *Server) authDeleteSession(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuthController(w) {
		return
	}
	var req authOwnerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.auth.DeleteSession(r.Context(), req.TelegramUserID); err != nil {
		s.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) requireAuthController(w http.ResponseWriter) bool {
	if s.auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "telegram authorization is not configured"})
		return false
	}
	return true
}

func (s *Server) writeAuthStatus(w http.ResponseWriter, status *tdlib.AuthStatus, err error) {
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, authStatusResponse{
		UserID:       status.UserID,
		SessionID:    status.SessionID,
		SessionState: string(status.SessionState),
		AuthState:    string(status.AuthState),
	})
}

func (s *Server) writeAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, tdlib.ErrUnauthorizedOwner) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return false
	}
	return true
}
