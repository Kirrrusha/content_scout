package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

func TestAuthPhoneHandler(t *testing.T) {
	auth := &fakeHTTPAuth{
		phoneStatus: &tdlib.AuthStatus{
			UserID:       1,
			SessionID:    2,
			SessionState: domain.SessionStatusAuthorizing,
			AuthState:    tdlib.AuthorizationStateWaitCode,
		},
	}
	server := NewWithAuth(":0", nil, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), auth)

	body := bytes.NewBufferString(`{"telegram_user_id":42,"phone":"+15550001111"}`)
	req := httptest.NewRequest(http.MethodPost, "/telegram/auth/phone", body)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if auth.phone != "+15550001111" {
		t.Fatalf("phone = %q", auth.phone)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("+15550001111")) {
		t.Fatalf("response leaked phone: %s", rec.Body.String())
	}

	var response authStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.AuthState != string(tdlib.AuthorizationStateWaitCode) {
		t.Fatalf("auth_state = %q", response.AuthState)
	}
}

func TestAuthHandlerRejectsForbiddenOwner(t *testing.T) {
	auth := &fakeHTTPAuth{err: tdlib.ErrUnauthorizedOwner}
	server := NewWithAuth(":0", nil, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), auth)

	req := httptest.NewRequest(http.MethodPost, "/telegram/auth/start", bytes.NewBufferString(`{"telegram_user_id":99}`))
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

type fakeHTTPAuth struct {
	err         error
	phone       string
	phoneStatus *tdlib.AuthStatus
}

func (f *fakeHTTPAuth) Start(context.Context, int64) (*tdlib.AuthStatus, error) {
	return &tdlib.AuthStatus{SessionState: domain.SessionStatusAuthorizing, AuthState: tdlib.AuthorizationStateWaitPhoneNumber}, f.err
}

func (f *fakeHTTPAuth) SubmitPhoneNumber(_ context.Context, _ int64, phone string) (*tdlib.AuthStatus, error) {
	f.phone = phone
	return f.phoneStatus, f.err
}

func (f *fakeHTTPAuth) SubmitCode(context.Context, int64, string) (*tdlib.AuthStatus, error) {
	return &tdlib.AuthStatus{SessionState: domain.SessionStatusConnected, AuthState: tdlib.AuthorizationStateReady}, f.err
}

func (f *fakeHTTPAuth) SubmitPassword(context.Context, int64, string) (*tdlib.AuthStatus, error) {
	return &tdlib.AuthStatus{SessionState: domain.SessionStatusConnected, AuthState: tdlib.AuthorizationStateReady}, f.err
}

func (f *fakeHTTPAuth) Status(context.Context, int64) (*tdlib.AuthStatus, error) {
	return &tdlib.AuthStatus{SessionState: domain.SessionStatusConnected, AuthState: tdlib.AuthorizationStateReady}, f.err
}

func (f *fakeHTTPAuth) DeleteSession(context.Context, int64) error {
	return f.err
}
