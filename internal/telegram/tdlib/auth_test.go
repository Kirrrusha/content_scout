package tdlib

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

func TestAuthServiceAuthorizationFlow(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserRepo()
	sessions := newMemorySessionRepo()
	client := &fakeClient{state: AuthorizationStateWaitPhoneNumber}
	service := NewAuthService(AuthServiceConfig{
		OwnerTelegramID: 42,
		TelegramAPIID:   100,
		TelegramAPIHash: "hash",
		StorageBaseDir:  t.TempDir(),
	}, users, sessions, fakeFactory{client: client})
	service.now = func() time.Time { return time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC) }

	status, err := service.Start(ctx, 42)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if status.AuthState != AuthorizationStateWaitPhoneNumber {
		t.Fatalf("AuthState = %s, want wait phone", status.AuthState)
	}
	if !client.started {
		t.Fatal("client was not started")
	}

	status, err = service.SubmitPhoneNumber(ctx, 42, "+15550001111")
	if err != nil {
		t.Fatalf("SubmitPhoneNumber() error = %v", err)
	}
	if status.AuthState != AuthorizationStateWaitCode {
		t.Fatalf("AuthState = %s, want wait code", status.AuthState)
	}
	if client.phone != "+15550001111" {
		t.Fatalf("phone was not submitted")
	}

	status, err = service.SubmitCode(ctx, 42, "12345")
	if err != nil {
		t.Fatalf("SubmitCode() error = %v", err)
	}
	if status.AuthState != AuthorizationStateReady {
		t.Fatalf("AuthState = %s, want ready", status.AuthState)
	}
	if status.SessionState != domain.SessionStatusConnected {
		t.Fatalf("SessionState = %s, want connected", status.SessionState)
	}

	session, err := sessions.FindByUserID(ctx, status.UserID)
	if err != nil {
		t.Fatalf("FindByUserID() error = %v", err)
	}
	if session == nil || session.LastConnected == nil {
		t.Fatalf("session = %+v, want persisted connected session", session)
	}
}

func TestAuthServicePasswordFlow(t *testing.T) {
	ctx := context.Background()
	service, client := newTestAuthService(t, AuthorizationStateWaitPassword)

	status, err := service.Start(ctx, 42)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if status.AuthState != AuthorizationStateWaitPassword {
		t.Fatalf("AuthState = %s, want wait password", status.AuthState)
	}

	status, err = service.SubmitPassword(ctx, 42, "secret-password")
	if err != nil {
		t.Fatalf("SubmitPassword() error = %v", err)
	}
	if status.AuthState != AuthorizationStateReady {
		t.Fatalf("AuthState = %s, want ready", status.AuthState)
	}
	if client.password != "secret-password" {
		t.Fatal("password was not submitted to client")
	}
}

func TestAuthServiceRejectsNonOwner(t *testing.T) {
	service, _ := newTestAuthService(t, AuthorizationStateWaitPhoneNumber)

	_, err := service.Start(context.Background(), 99)
	if !errors.Is(err, ErrUnauthorizedOwner) {
		t.Fatalf("Start() error = %v, want ErrUnauthorizedOwner", err)
	}
}

func TestAuthServiceDeleteSession(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserRepo()
	sessions := newMemorySessionRepo()
	baseDir := t.TempDir()
	client := &fakeClient{state: AuthorizationStateReady}
	service := NewAuthService(AuthServiceConfig{
		OwnerTelegramID: 42,
		TelegramAPIID:   100,
		TelegramAPIHash: "hash",
		StorageBaseDir:  baseDir,
	}, users, sessions, fakeFactory{client: client})

	status, err := service.Start(ctx, 42)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if filepath.Dir(status.SessionPath) != baseDir {
		t.Fatalf("SessionPath = %q, want under %q", status.SessionPath, baseDir)
	}

	if err := service.DeleteSession(ctx, 42); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if !client.stopped {
		t.Fatal("client was not stopped")
	}
	session, err := sessions.FindByUserID(ctx, status.UserID)
	if err != nil {
		t.Fatalf("FindByUserID() error = %v", err)
	}
	if session != nil {
		t.Fatalf("session = %+v, want nil", session)
	}
}

func newTestAuthService(t *testing.T, state AuthorizationState) (*AuthService, *fakeClient) {
	t.Helper()
	client := &fakeClient{state: state}
	service := NewAuthService(AuthServiceConfig{
		OwnerTelegramID: 42,
		TelegramAPIID:   100,
		TelegramAPIHash: "hash",
		StorageBaseDir:  t.TempDir(),
	}, newMemoryUserRepo(), newMemorySessionRepo(), fakeFactory{client: client})
	return service, client
}

type fakeFactory struct {
	client *fakeClient
}

func (f fakeFactory) NewClient(string) (TelegramClient, error) {
	return f.client, nil
}

type fakeClient struct {
	state    AuthorizationState
	started  bool
	stopped  bool
	phone    string
	code     string
	password string
}

func (c *fakeClient) Start(context.Context) error {
	c.started = true
	return nil
}

func (c *fakeClient) Stop(context.Context) error {
	c.stopped = true
	c.state = AuthorizationStateClosed
	return nil
}

func (c *fakeClient) AuthorizationState(context.Context) (AuthorizationState, error) {
	return c.state, nil
}

func (c *fakeClient) SubmitPhoneNumber(_ context.Context, phone string) error {
	c.phone = phone
	c.state = AuthorizationStateWaitCode
	return nil
}

func (c *fakeClient) SubmitCode(_ context.Context, code string) error {
	c.code = code
	c.state = AuthorizationStateReady
	return nil
}

func (c *fakeClient) SubmitPassword(_ context.Context, password string) error {
	c.password = password
	c.state = AuthorizationStateReady
	return nil
}

func (c *fakeClient) ListFolders(context.Context) ([]domain.TelegramFolder, error) {
	return nil, nil
}

func (c *fakeClient) ListChats(context.Context, ChatList) ([]domain.TelegramChat, error) {
	return nil, nil
}

func (c *fakeClient) GetChatHistory(context.Context, int64, int64, int) ([]domain.TelegramMessage, error) {
	return nil, nil
}

type memoryUserRepo struct {
	nextID int64
	users  map[int64]domain.User
}

func newMemoryUserRepo() *memoryUserRepo {
	return &memoryUserRepo{nextID: 1, users: make(map[int64]domain.User)}
}

func (r *memoryUserRepo) UpsertByTelegramID(_ context.Context, telegramUserID int64) (*domain.User, error) {
	if user, ok := r.users[telegramUserID]; ok {
		return &user, nil
	}
	user := domain.User{ID: r.nextID, TelegramUserID: telegramUserID}
	r.nextID++
	r.users[telegramUserID] = user
	return &user, nil
}

func (r *memoryUserRepo) FindByTelegramID(_ context.Context, telegramUserID int64) (*domain.User, error) {
	user, ok := r.users[telegramUserID]
	if !ok {
		return nil, nil
	}
	return &user, nil
}

type memorySessionRepo struct {
	nextID   int64
	byUserID map[int64]domain.TelegramSession
}

func newMemorySessionRepo() *memorySessionRepo {
	return &memorySessionRepo{nextID: 1, byUserID: make(map[int64]domain.TelegramSession)}
}

func (r *memorySessionRepo) Upsert(_ context.Context, session domain.TelegramSession) (*domain.TelegramSession, error) {
	if existing, ok := r.byUserID[session.UserID]; ok {
		session.ID = existing.ID
		session.CreatedAt = existing.CreatedAt
	} else {
		session.ID = r.nextID
		r.nextID++
		session.CreatedAt = time.Now()
	}
	session.UpdatedAt = time.Now()
	r.byUserID[session.UserID] = session
	return &session, nil
}

func (r *memorySessionRepo) FindByUserID(_ context.Context, userID int64) (*domain.TelegramSession, error) {
	session, ok := r.byUserID[userID]
	if !ok {
		return nil, nil
	}
	return &session, nil
}

func (r *memorySessionRepo) DeleteByUserID(_ context.Context, userID int64) error {
	delete(r.byUserID, userID)
	return nil
}
