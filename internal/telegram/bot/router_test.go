package bot

import (
	"context"
	"strings"
	"testing"
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
	router := NewRouter(42, states)

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

	state, ok, err := states.Get(ctx, 42)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || state.View != ViewFolders {
		t.Fatalf("state = %+v ok=%v, want folders", state, ok)
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
