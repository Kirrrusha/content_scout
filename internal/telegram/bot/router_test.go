package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

func TestRouterStartShowsMainMenu(t *testing.T) {
	ctx := context.Background()
	states := NewMemoryStateStore()
	router := NewRouter(42, states)

	out, err := router.Handle(ctx, Incoming{
		Kind:    IncomingMessage,
		UserID:  42,
		ChatID:  100,
		Command: "start",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if out.ChatID != 100 {
		t.Fatalf("ChatID = %d, want 100", out.ChatID)
	}
	if !strings.Contains(out.Text, "Telegram Summary Bot") {
		t.Fatalf("Text = %q", out.Text)
	}
	if len(out.Menu) != 5 {
		t.Fatalf("menu rows = %d, want 5", len(out.Menu))
	}

	state, ok, err := states.Get(ctx, 42)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || state.View != ViewStart {
		t.Fatalf("state = %+v ok=%v, want start", state, ok)
	}
}

func TestRouterRejectsNonOwner(t *testing.T) {
	router := NewRouter(42, NewMemoryStateStore())

	out, err := router.Handle(context.Background(), Incoming{
		Kind:    IncomingMessage,
		UserID:  99,
		ChatID:  100,
		Command: "start",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if out.Text != "Доступ запрещен." {
		t.Fatalf("Text = %q, want access denied", out.Text)
	}
	if len(out.Menu) != 0 {
		t.Fatalf("menu rows = %d, want 0", len(out.Menu))
	}
}

func TestRouterCallbackRoutesAndStoresState(t *testing.T) {
	ctx := context.Background()
	states := NewMemoryStateStore()
	sync := &fakeSyncController{
		folders: []domain.TelegramFolder{{Name: "Golang"}},
	}
	router := NewRouterWithControllers(42, states, nil, sync)

	out, err := router.Handle(ctx, Incoming{
		Kind:            IncomingCallback,
		UserID:          42,
		ChatID:          100,
		CallbackID:      "callback-1",
		CallbackData:    ActionFolders,
		CallbackMessage: 7,
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if out.EditMessageID != 7 {
		t.Fatalf("EditMessageID = %d, want 7", out.EditMessageID)
	}
	if out.CallbackID != "callback-1" {
		t.Fatalf("CallbackID = %q", out.CallbackID)
	}
	if out.AnswerCallback == "" {
		t.Fatal("AnswerCallback is empty")
	}
	if !strings.Contains(out.Text, "Golang") {
		t.Fatalf("Text = %q, want folder name", out.Text)
	}

	state, ok, err := states.Get(ctx, 42)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || state.View != ViewFolders {
		t.Fatalf("state = %+v ok=%v, want folders", state, ok)
	}
}

func TestRouterSyncAndChats(t *testing.T) {
	ctx := context.Background()
	states := NewMemoryStateStore()
	sync := &fakeSyncController{
		result: &tdlib.SyncResult{FoldersCount: 1, ChatsCount: 2},
		chats: []domain.TelegramChat{{
			Title:       "Backend",
			Type:        domain.ChatTypeChannel,
			UnreadCount: 7,
		}},
	}
	router := NewRouterWithControllers(42, states, nil, sync)

	out, err := router.Handle(ctx, Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "/sync",
	})
	if err != nil {
		t.Fatalf("Handle(sync) error = %v", err)
	}
	if !sync.synced || !strings.Contains(out.Text, "Чатов: 2") {
		t.Fatalf("sync output = %q synced=%v", out.Text, sync.synced)
	}

	out, err = router.Handle(ctx, Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "/chats",
	})
	if err != nil {
		t.Fatalf("Handle(chats) error = %v", err)
	}
	if !strings.Contains(out.Text, "Backend") || !strings.Contains(out.Text, "unread: 7") {
		t.Fatalf("chats output = %q", out.Text)
	}
}

func TestRouterRequiresConfiguredOwner(t *testing.T) {
	router := NewRouter(0, NewMemoryStateStore())

	_, err := router.Handle(context.Background(), Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "/start",
	})
	if err == nil {
		t.Fatal("Handle() error = nil, want owner config error")
	}
}

func TestRouterConnectStartsAuthorization(t *testing.T) {
	ctx := context.Background()
	states := NewMemoryStateStore()
	auth := &fakeAuthController{
		startStatus: &tdlib.AuthStatus{
			SessionState: domain.SessionStatusAuthorizing,
			AuthState:    tdlib.AuthorizationStateWaitPhoneNumber,
		},
	}
	router := NewRouterWithAuth(42, states, auth)

	out, err := router.Handle(ctx, Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "/connect",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !auth.started {
		t.Fatal("auth was not started")
	}
	if !strings.Contains(out.Text, "номер телефона") {
		t.Fatalf("Text = %q", out.Text)
	}
	state, ok, err := states.Get(ctx, 42)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || state.View != ViewAuthPhone {
		t.Fatalf("state = %+v ok=%v, want auth phone", state, ok)
	}
}

func TestRouterDialogSubmitsCode(t *testing.T) {
	ctx := context.Background()
	states := NewMemoryStateStore()
	if err := states.Set(ctx, 42, DialogState{View: ViewAuthCode}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	auth := &fakeAuthController{
		codeStatus: &tdlib.AuthStatus{
			SessionState: domain.SessionStatusConnected,
			AuthState:    tdlib.AuthorizationStateReady,
		},
	}
	router := NewRouterWithAuth(42, states, auth)

	out, err := router.Handle(ctx, Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "12345",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if auth.code != "12345" {
		t.Fatalf("code = %q, want 12345", auth.code)
	}
	if !strings.Contains(out.Text, "подключен") {
		t.Fatalf("Text = %q", out.Text)
	}
}

type fakeAuthController struct {
	started     bool
	deleted     bool
	phone       string
	code        string
	password    string
	startStatus *tdlib.AuthStatus
	phoneStatus *tdlib.AuthStatus
	codeStatus  *tdlib.AuthStatus
}

func (f *fakeAuthController) Start(context.Context, int64) (*tdlib.AuthStatus, error) {
	f.started = true
	return f.startStatus, nil
}

func (f *fakeAuthController) SubmitPhoneNumber(_ context.Context, _ int64, phone string) (*tdlib.AuthStatus, error) {
	f.phone = phone
	return f.phoneStatus, nil
}

func (f *fakeAuthController) SubmitCode(_ context.Context, _ int64, code string) (*tdlib.AuthStatus, error) {
	f.code = code
	return f.codeStatus, nil
}

func (f *fakeAuthController) SubmitPassword(_ context.Context, _ int64, password string) (*tdlib.AuthStatus, error) {
	f.password = password
	return &tdlib.AuthStatus{SessionState: domain.SessionStatusConnected, AuthState: tdlib.AuthorizationStateReady}, nil
}

func (f *fakeAuthController) Status(context.Context, int64) (*tdlib.AuthStatus, error) {
	return &tdlib.AuthStatus{SessionState: domain.SessionStatusConnected, AuthState: tdlib.AuthorizationStateReady}, nil
}

func (f *fakeAuthController) DeleteSession(context.Context, int64) error {
	f.deleted = true
	return nil
}

type fakeSyncController struct {
	synced  bool
	result  *tdlib.SyncResult
	folders []domain.TelegramFolder
	chats   []domain.TelegramChat
}

func (f *fakeSyncController) Sync(context.Context, int64) (*tdlib.SyncResult, error) {
	f.synced = true
	return f.result, nil
}

func (f *fakeSyncController) ListFolders(context.Context, int64) ([]domain.TelegramFolder, error) {
	return f.folders, nil
}

func (f *fakeSyncController) ListChats(context.Context, int64) ([]domain.TelegramChat, error) {
	return f.chats, nil
}
