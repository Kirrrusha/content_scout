package httpserver

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

func TestSecurityMiddlewareAllowsPublicHealthWithoutToken(t *testing.T) {
	server := NewWithOptions(":0", nil, testLogger(), Options{ServiceToken: "secret", RequireAuth: true}, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing security header: %+v", rec.Header())
	}
}

func TestSecurityMiddlewareRejectsInternalEndpointWithoutBearerToken(t *testing.T) {
	server := NewWithOptions(":0", nil, testLogger(), Options{ServiceToken: "secret", RequireAuth: true}, &securityFakeAuth{}, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/telegram/auth/start", bytes.NewBufferString(`{"telegram_user_id":42}`))
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestSecurityMiddlewareAllowsValidBearerToken(t *testing.T) {
	auth := &securityFakeAuth{}
	server := NewWithOptions(":0", nil, testLogger(), Options{ServiceToken: "secret", RequireAuth: true}, auth, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/telegram/auth/start", bytes.NewBufferString(`{"telegram_user_id":42}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !auth.started {
		t.Fatal("auth controller was not called")
	}
}

func TestSecurityMiddlewareLimitsRequestBody(t *testing.T) {
	server := NewWithOptions(":0", nil, testLogger(), Options{ServiceToken: "secret", RequireAuth: true, MaxRequestBytes: 8}, &securityFakeAuth{}, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/telegram/auth/start", bytes.NewBufferString(`{"telegram_user_id":42}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
}

type securityFakeAuth struct {
	started bool
}

func (f *securityFakeAuth) Start(context.Context, int64) (*tdlib.AuthStatus, error) {
	f.started = true
	return &tdlib.AuthStatus{SessionState: domain.SessionStatusAuthorizing, AuthState: tdlib.AuthorizationStateWaitPhoneNumber}, nil
}

func (f *securityFakeAuth) SubmitPhoneNumber(context.Context, int64, string) (*tdlib.AuthStatus, error) {
	return nil, nil
}

func (f *securityFakeAuth) SubmitCode(context.Context, int64, string) (*tdlib.AuthStatus, error) {
	return nil, nil
}

func (f *securityFakeAuth) SubmitPassword(context.Context, int64, string) (*tdlib.AuthStatus, error) {
	return nil, nil
}

func (f *securityFakeAuth) Status(context.Context, int64) (*tdlib.AuthStatus, error) {
	return nil, nil
}

func (f *securityFakeAuth) DeleteSession(context.Context, int64) error {
	return nil
}
