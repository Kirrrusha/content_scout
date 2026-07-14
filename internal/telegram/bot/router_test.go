package bot

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/kirilllebedenko/content_scout/internal/article"
	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/obsidian"
	"github.com/kirilllebedenko/content_scout/internal/schedules"
	"github.com/kirilllebedenko/content_scout/internal/sourcegroups"
	"github.com/kirilllebedenko/content_scout/internal/summary"
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
	if len(out.Menu) != 6 {
		t.Fatalf("menu rows = %d, want 6", len(out.Menu))
	}

	state, ok, err := states.Get(ctx, 42)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || state.View != ViewStart {
		t.Fatalf("state = %+v ok=%v, want start", state, ok)
	}
}

func TestTelegramTextFitsTelegramMessageLimit(t *testing.T) {
	got := telegramText(strings.Repeat("a", 5000))
	if len([]rune(got)) > 4096 {
		t.Fatalf("telegramText length = %d", len([]rune(got)))
	}
	if !strings.Contains(got, "Текст сокращен") {
		t.Fatalf("telegramText() = %q", got)
	}
}

func TestSafeTelegramTextForLogRedactsSensitiveCommands(t *testing.T) {
	for _, text := range []string{"/phone +79990000000", "/code 12345", "/password secret"} {
		got := safeTelegramTextForLog(text)
		if got != "<redacted>" {
			t.Fatalf("safeTelegramTextForLog(%q) = %q", text, got)
		}
	}
	if got := safeTelegramTextForLog("/groups"); got != "/groups" {
		t.Fatalf("safeTelegramTextForLog(/groups) = %q", got)
	}
}

func TestAsyncArticleConversionCallbackClassifier(t *testing.T) {
	for _, data := range []string{"art:summary:10", "art:topic:10:2"} {
		if !isAsyncArticleConversionCallback(Incoming{Kind: IncomingCallback, CallbackData: data}) {
			t.Fatalf("%q should be async", data)
		}
	}
	for _, data := range []string{"art:list", "art:open:1", "sum:open:1"} {
		if isAsyncArticleConversionCallback(Incoming{Kind: IncomingCallback, CallbackData: data}) {
			t.Fatalf("%q should not be async", data)
		}
	}
}

func TestAsyncSummaryGenerationCallbackClassifier(t *testing.T) {
	if !isAsyncSummaryGenerationCallback(Incoming{Kind: IncomingCallback, CallbackData: "newsum:generate:9"}) {
		t.Fatal("newsum:generate should be async")
	}
	for _, data := range []string{"newsum:group:7", "newsum:mode:7:24h", "sum:open:1"} {
		if isAsyncSummaryGenerationCallback(Incoming{Kind: IncomingCallback, CallbackData: data}) {
			t.Fatalf("%q should not be async", data)
		}
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

func TestRouterChatsPagination(t *testing.T) {
	ctx := context.Background()
	states := NewMemoryStateStore()
	chats := make([]domain.TelegramChat, 35)
	for i := range chats {
		chats[i] = domain.TelegramChat{
			ID:          int64(100 + i),
			Title:       "Chat " + strconv.Itoa(i+1),
			Type:        domain.ChatTypeChannel,
			UnreadCount: i,
		}
	}
	sync := &fakeSyncController{chats: chats}
	router := NewRouterWithControllers(42, states, nil, sync)

	out, err := router.Handle(ctx, Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "/chats",
	})
	if err != nil {
		t.Fatalf("Handle(chats) error = %v", err)
	}
	if !strings.Contains(out.Text, "Каналы и группы: 1-30 из 35") || !strings.Contains(out.Text, "ID: 100") {
		t.Fatalf("first page text = %q", out.Text)
	}
	if len(out.Menu) != 2 || out.Menu[0][0].Data != "chats:page:1" {
		t.Fatalf("first page menu = %+v", out.Menu)
	}

	out, err = router.Handle(ctx, Incoming{
		Kind:            IncomingCallback,
		UserID:          42,
		ChatID:          100,
		CallbackID:      "callback-1",
		CallbackData:    "chats:page:1",
		CallbackMessage: 7,
	})
	if err != nil {
		t.Fatalf("Handle(chats page callback) error = %v", err)
	}
	if out.EditMessageID != 7 || out.CallbackID != "callback-1" {
		t.Fatalf("out = %+v", out)
	}
	if !strings.Contains(out.Text, "Каналы и группы: 31-35 из 35") || !strings.Contains(out.Text, "31. Chat 31 [ID: 130") {
		t.Fatalf("second page text = %q", out.Text)
	}
	if len(out.Menu) != 2 || out.Menu[0][0].Data != "chats:page:0" {
		t.Fatalf("second page menu = %+v", out.Menu)
	}
	if !strings.Contains(out.Text, "/group_add_chat <group_id> <chat_id>") {
		t.Fatalf("second page lacks selection help: %q", out.Text)
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

func TestPublicAuthErrorExplainsTDLibConfiguration(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "api id",
			err:  errors.New("start tdlib client: TELEGRAM_API_ID is not configured"),
			want: "TELEGRAM_API_ID не настроен",
		},
		{
			name: "api hash",
			err:  errors.New("start tdlib client: TELEGRAM_API_HASH is not configured"),
			want: "TELEGRAM_API_HASH не настроен",
		},
		{
			name: "database dir",
			err:  errors.New("TDLIB_DATABASE_DIR is not configured"),
			want: "TDLIB_DATABASE_DIR не настроен",
		},
		{
			name: "session path",
			err:  errors.New("start tdlib client: TDLib session path is not configured"),
			want: "TDLIB_DATABASE_DIR не настроен",
		},
		{
			name: "unavailable adapter",
			err:  errors.New("start tdlib client: native TDLib adapter is not connected yet"),
			want: "Native TDLib adapter не подключен",
		},
		{
			name: "internal api timeout",
			err:  errors.New("Post \"http://127.0.0.1:8080/telegram/auth/start\": context deadline exceeded (Client.Timeout exceeded while awaiting headers)"),
			want: "Внутренний API не ответил вовремя",
		},
		{
			name: "llm summary timeout",
			err:  errors.New("summarize with llm: Post \"https://openrouter.ai/api/v1/chat/completions\": context deadline exceeded (Client.Timeout exceeded while awaiting headers)"),
			want: "LLM не успела создать сводку вовремя",
		},
		{
			name: "internal api unavailable",
			err:  errors.New("dial tcp 127.0.0.1:8080: connect: connection refused"),
			want: "Внутренний API недоступен",
		},
		{
			name: "invalid phone",
			err:  errors.New("submit tdlib authorization input: tdlib error 400: PHONE_NUMBER_INVALID"),
			want: "международном формате с плюсом",
		},
		{
			name: "session not started",
			err:  errors.New("telegram session is not started"),
			want: "/connect",
		},
		{
			name: "session not connected",
			err:  errors.New("telegram session is not connected"),
			want: "Завершите авторизацию",
		},
		{
			name: "authorization not ready",
			err:  errors.New("telegram authorization is not ready: authorizationStateWaitCode"),
			want: "/session",
		},
		{
			name: "unknown telegram error includes detail",
			err:  errors.New("list main telegram chats: load telegram chats: tdlib error 400: FLOOD_WAIT_3"),
			want: "FLOOD_WAIT_3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publicAuthError(tt.err); !strings.Contains(got, tt.want) {
				t.Fatalf("publicAuthError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRouterDialogRequiresPhoneWithPlus(t *testing.T) {
	ctx := context.Background()
	states := NewMemoryStateStore()
	if err := states.Set(ctx, 42, DialogState{View: ViewAuthPhone}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	auth := &fakeAuthController{}
	router := NewRouterWithAuth(42, states, auth)

	out, err := router.Handle(ctx, Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "79204675533",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if auth.phone != "" {
		t.Fatalf("phone = %q, want no submitted phone", auth.phone)
	}
	if !strings.Contains(out.Text, "с плюсом") {
		t.Fatalf("Text = %q", out.Text)
	}
}

func TestRouterDialogRefusesTelegramCode(t *testing.T) {
	ctx := context.Background()
	states := NewMemoryStateStore()
	if err := states.Set(ctx, 42, DialogState{View: ViewAuthCode}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	auth := &fakeAuthController{}
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
	if auth.code != "" {
		t.Fatalf("code = %q, want no bot-submitted code", auth.code)
	}
	if !strings.Contains(out.Text, "Не отправляйте код") {
		t.Fatalf("Text = %q", out.Text)
	}
}

func TestRouterSourceGroupCommands(t *testing.T) {
	ctx := context.Background()
	groups := &fakeGroupController{
		created: &domain.SourceGroup{ID: 1, Name: "Golang"},
		withChats: &sourcegroups.GroupWithChats{
			Group: domain.SourceGroup{ID: 1, Name: "Golang"},
			Links: []domain.SourceGroupChat{{GroupID: 1, ChatID: 10, Priority: 3, Enabled: true}},
			Chats: []domain.TelegramChat{{ID: 10, Title: "Backend", Type: domain.ChatTypeChannel}},
		},
	}
	router := NewRouterWithAllControllers(42, NewMemoryStateStore(), nil, nil, groups)

	out, err := router.Handle(ctx, Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "/group_create Golang",
	})
	if err != nil {
		t.Fatalf("Handle(group_create) error = %v", err)
	}
	if groups.createdName != "Golang" || !strings.Contains(out.Text, "Группа создана") {
		t.Fatalf("createdName=%q output=%q", groups.createdName, out.Text)
	}

	out, err = router.Handle(ctx, Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "/group_add_chat 1 10 3",
	})
	if err != nil {
		t.Fatalf("Handle(group_add_chat) error = %v", err)
	}
	if groups.addedGroupID != 1 || groups.addedChatID != 10 || groups.addedPriority != 3 {
		t.Fatalf("added group=%d chat=%d priority=%d", groups.addedGroupID, groups.addedChatID, groups.addedPriority)
	}

	out, err = router.Handle(ctx, Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "/group_chats 1",
	})
	if err != nil {
		t.Fatalf("Handle(group_chats) error = %v", err)
	}
	if !strings.Contains(out.Text, "Backend") {
		t.Fatalf("group chats output = %q", out.Text)
	}
}

func TestRouterGroupsListCallback(t *testing.T) {
	ctx := context.Background()
	groups := &fakeGroupController{
		groups: []domain.SourceGroup{{ID: 7, Name: "AI"}},
	}
	router := NewRouterWithAllControllers(42, NewMemoryStateStore(), nil, nil, groups)

	out, err := router.Handle(ctx, Incoming{
		Kind:            IncomingCallback,
		UserID:          42,
		ChatID:          100,
		CallbackID:      "callback-1",
		CallbackData:    ActionGroups,
		CallbackMessage: 5,
	})
	if err != nil {
		t.Fatalf("Handle(groups list callback) error = %v", err)
	}
	if out.EditMessageID != 5 || out.CallbackID != "callback-1" {
		t.Fatalf("out = %+v", out)
	}
	if !strings.Contains(out.Text, "Мои группы") || out.Menu[0][0].Data != "groups:open:7" {
		t.Fatalf("output = %q menu=%+v", out.Text, out.Menu)
	}
}

func TestPublicGroupErrorExplainsEmptyGroupFromAPI(t *testing.T) {
	got := publicGroupError(errors.New("source group not found"))
	if !strings.Contains(got, "/group_add_chat") {
		t.Fatalf("publicGroupError() = %q", got)
	}
}

func TestRouterCollectGroupCommand(t *testing.T) {
	ctx := context.Background()
	collector := &fakeCollectionController{
		result: &collection.Result{JobID: 9, ChatsCount: 2, MessagesCount: 5},
	}
	router := NewRouterWithRuntime(42, NewMemoryStateStore(), nil, nil, nil, collector)

	out, err := router.Handle(ctx, Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "/collect_group 7 24h 25",
	})
	if err != nil {
		t.Fatalf("Handle(collect_group) error = %v", err)
	}
	if collector.request.GroupID != 7 || collector.request.Mode != domain.CollectionModeLast24H || collector.request.Limit != 25 {
		t.Fatalf("request = %+v", collector.request)
	}
	if !strings.Contains(out.Text, "Сообщений: 5") {
		t.Fatalf("output = %q", out.Text)
	}
}

func TestRouterSummarizeCollectionCommand(t *testing.T) {
	ctx := context.Background()
	summary := &fakeSummaryController{
		result: &summaryResultFixture,
	}
	router := NewRouterWithServices(42, NewMemoryStateStore(), nil, nil, nil, nil, summary)

	out, err := router.Handle(ctx, Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "/summarize_collection 9 detailed",
	})
	if err != nil {
		t.Fatalf("Handle(summarize_collection) error = %v", err)
	}
	if summary.request.CollectionJobID != 9 || summary.request.Format != "detailed" {
		t.Fatalf("request = %+v", summary.request)
	}
	if !strings.Contains(out.Text, "Сводка готова") {
		t.Fatalf("output = %q", out.Text)
	}
}

func TestRouterSummarizeCollectionTimeoutIsLLMError(t *testing.T) {
	ctx := context.Background()
	summary := &fakeSummaryController{
		err: errors.New("Post \"http://127.0.0.1:8080/summaries/from-collection/9\": context deadline exceeded (Client.Timeout exceeded while awaiting headers)"),
	}
	router := NewRouterWithServices(42, NewMemoryStateStore(), nil, nil, nil, nil, summary)

	out, err := router.Handle(ctx, Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "/summarize_collection 9 detailed",
	})
	if err != nil {
		t.Fatalf("Handle(summarize_collection) error = %v", err)
	}
	if !strings.Contains(out.Text, "LLM не успела создать сводку вовремя") {
		t.Fatalf("output = %q", out.Text)
	}
}

func TestRouterNewSummaryButtonFlow(t *testing.T) {
	ctx := context.Background()
	groups := &fakeGroupController{
		groups: []domain.SourceGroup{{ID: 7, Name: "AI"}},
	}
	collector := &fakeCollectionController{
		result: &collection.Result{JobID: 9, ChatsCount: 2, MessagesCount: 5},
	}
	summary := &fakeSummaryController{
		result: &summaryResultFixture,
	}
	router := NewRouterWithServices(42, NewMemoryStateStore(), nil, nil, groups, collector, summary)

	out, err := router.Handle(ctx, Incoming{
		Kind:            IncomingCallback,
		UserID:          42,
		ChatID:          100,
		CallbackID:      "callback-1",
		CallbackData:    ActionNewSummary,
		CallbackMessage: 5,
	})
	if err != nil {
		t.Fatalf("Handle(new summary) error = %v", err)
	}
	if out.Menu[0][0].Data != "newsum:group:7" {
		t.Fatalf("menu = %+v", out.Menu)
	}

	out, err = router.Handle(ctx, Incoming{
		Kind:            IncomingCallback,
		UserID:          42,
		ChatID:          100,
		CallbackID:      "callback-2",
		CallbackData:    "newsum:mode:7:24h",
		CallbackMessage: 5,
	})
	if err != nil {
		t.Fatalf("Handle(new summary mode) error = %v", err)
	}
	if collector.request.GroupID != 7 || collector.request.Mode != domain.CollectionModeLast24H || collector.request.Limit != 100 {
		t.Fatalf("collect request = %+v", collector.request)
	}
	if out.Menu[0][0].Data != "newsum:generate:9" {
		t.Fatalf("menu = %+v", out.Menu)
	}

	out, err = router.Handle(ctx, Incoming{
		Kind:            IncomingCallback,
		UserID:          42,
		ChatID:          100,
		CallbackID:      "callback-3",
		CallbackData:    "newsum:generate:9",
		CallbackMessage: 5,
	})
	if err != nil {
		t.Fatalf("Handle(new summary generate) error = %v", err)
	}
	if summary.request.CollectionJobID != 9 || summary.request.Format != "standard" {
		t.Fatalf("summary request = %+v", summary.request)
	}
	if !strings.Contains(out.Text, "Сводка готова") || out.Menu[0][0].Data != "sum:open:1" {
		t.Fatalf("output = %q menu=%+v", out.Text, out.Menu)
	}
}

func TestRouterSummariesCommand(t *testing.T) {
	ctx := context.Background()
	browser := &fakeSummaryBrowserController{
		summaries: []domain.Summary{{ID: 10, Title: "Digest", TopicsCount: 2, MessagesCount: 12, SourcesCount: 3}},
	}
	router := NewRouterWithBrowser(42, NewMemoryStateStore(), nil, nil, nil, nil, nil, browser)

	out, err := router.Handle(ctx, Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "/summaries",
	})
	if err != nil {
		t.Fatalf("Handle(summaries) error = %v", err)
	}
	if browser.listUserID != 42 || browser.listLimit != 10 {
		t.Fatalf("list user=%d limit=%d", browser.listUserID, browser.listLimit)
	}
	if !strings.Contains(out.Text, "Digest") || len(out.Menu) < 2 || out.Menu[0][0].Data != "sum:open:10" {
		t.Fatalf("output = %q menu=%+v", out.Text, out.Menu)
	}
}

func TestRouterSummaryOpenCallback(t *testing.T) {
	ctx := context.Background()
	browser := &fakeSummaryBrowserController{
		summary: &domain.Summary{ID: 10, Title: "Digest", Overview: "Overview", TopicsCount: 2, MessagesCount: 12, SourcesCount: 3},
	}
	router := NewRouterWithBrowser(42, NewMemoryStateStore(), nil, nil, nil, nil, nil, browser)

	out, err := router.Handle(ctx, Incoming{
		Kind:            IncomingCallback,
		UserID:          42,
		ChatID:          100,
		CallbackID:      "callback-1",
		CallbackData:    "sum:open:10",
		CallbackMessage: 7,
	})
	if err != nil {
		t.Fatalf("Handle(summary open callback) error = %v", err)
	}
	if browser.getSummaryID != 10 {
		t.Fatalf("summary id = %d", browser.getSummaryID)
	}
	if out.EditMessageID != 7 || out.CallbackID != "callback-1" {
		t.Fatalf("out = %+v", out)
	}
	if !strings.Contains(out.Text, "Сводка #10") || out.Menu[0][0].Data != "sum:topic:10:1" {
		t.Fatalf("output = %q menu=%+v", out.Text, out.Menu)
	}
}

func TestRouterSummaryTopicCallback(t *testing.T) {
	ctx := context.Background()
	username := "golang"
	browser := &fakeSummaryBrowserController{
		card: &summary.TopicCard{
			Summary: domain.Summary{ID: 10, Title: "Digest"},
			Topic: domain.SummaryTopic{
				Title:         "Go",
				ShortSummary:  "Short",
				FullSummary:   "Full",
				Importance:    8,
				Confidence:    domain.ConfidenceHigh,
				MessagesCount: 4,
				Sources: []domain.SummaryTopicSource{{
					Title:    "Go Channel",
					Username: &username,
				}},
				Messages: []domain.SummaryTopicMessage{{
					SourceTitle: "Go Channel",
					SourceURL:   "https://t.me/golang/101",
				}},
			},
			Index: 1,
			Total: 2,
		},
	}
	router := NewRouterWithBrowser(42, NewMemoryStateStore(), nil, nil, nil, nil, nil, browser)

	out, err := router.Handle(ctx, Incoming{
		Kind:            IncomingCallback,
		UserID:          42,
		ChatID:          100,
		CallbackID:      "callback-1",
		CallbackData:    "sum:topic:10:1",
		CallbackMessage: 7,
	})
	if err != nil {
		t.Fatalf("Handle(topic callback) error = %v", err)
	}
	if browser.cardSummaryID != 10 || browser.cardPosition != 1 {
		t.Fatalf("card summary=%d position=%d", browser.cardSummaryID, browser.cardPosition)
	}
	if out.EditMessageID != 7 || out.CallbackID != "callback-1" {
		t.Fatalf("out = %+v", out)
	}
	if !strings.Contains(out.Text, "Go") || !strings.Contains(out.Text, "https://t.me/golang/101") || out.Menu[0][1].Data != "sum:topic:10:2" {
		t.Fatalf("output = %q menu=%+v", out.Text, out.Menu)
	}
}

func TestRouterArticleFromTopicCallback(t *testing.T) {
	ctx := context.Background()
	articles := &fakeArticleController{
		result: &article.Result{Article: domain.Article{ID: 7, Title: "Go Guide", Slug: "go-guide", Type: domain.ArticleTypeGuide, Status: domain.ArticleStatusDraft, Tags: []string{"go"}}, Sources: 2},
	}
	router := NewRouterWithArticle(42, NewMemoryStateStore(), nil, nil, nil, nil, nil, nil, articles)

	out, err := router.Handle(ctx, Incoming{
		Kind:            IncomingCallback,
		UserID:          42,
		ChatID:          100,
		CallbackID:      "callback-1",
		CallbackData:    "art:topic:10:2",
		CallbackMessage: 7,
	})
	if err != nil {
		t.Fatalf("Handle(article topic callback) error = %v", err)
	}
	if articles.topicRequest.SummaryID != 10 || articles.topicRequest.TopicPosition != 2 {
		t.Fatalf("request = %+v", articles.topicRequest)
	}
	if out.EditMessageID != 7 || out.CallbackID != "callback-1" {
		t.Fatalf("out = %+v", out)
	}
	if !strings.Contains(out.Text, "Черновик статьи создан") || !strings.Contains(out.Text, "Go Guide") {
		t.Fatalf("output = %q", out.Text)
	}
}

func TestRouterArticleTitleCommand(t *testing.T) {
	ctx := context.Background()
	articles := &fakeArticleController{
		updated: &domain.Article{ID: 7, Title: "New title", Slug: "old", Type: domain.ArticleTypeAnalysis, Status: domain.ArticleStatusDraft, ContentMarkdown: "# Old"},
	}
	router := NewRouterWithArticle(42, NewMemoryStateStore(), nil, nil, nil, nil, nil, nil, articles)

	out, err := router.Handle(ctx, Incoming{
		Kind:   IncomingMessage,
		UserID: 42,
		ChatID: 100,
		Text:   "/article_title 7 New title",
	})
	if err != nil {
		t.Fatalf("Handle(article_title) error = %v", err)
	}
	if articles.updateArticleID != 7 || articles.updateTitle != "New title" {
		t.Fatalf("update article=%d title=%q", articles.updateArticleID, articles.updateTitle)
	}
	if !strings.Contains(out.Text, "New title") {
		t.Fatalf("output = %q", out.Text)
	}
}

func TestRouterExportArticleCallbackSendsDocument(t *testing.T) {
	ctx := context.Background()
	articleID := int64(7)
	exports := &fakeExportController{
		result: &obsidian.Result{
			Export: domain.ObsidianExport{ID: 1, ArticleID: &articleID, FileName: "Go Guide.md", VaultPath: "Articles/go/Go Guide.md"},
			Path:   "/tmp/Go Guide.md",
		},
	}
	router := NewRouterWithExports(42, NewMemoryStateStore(), nil, nil, nil, nil, nil, nil, nil, exports)

	out, err := router.Handle(ctx, Incoming{
		Kind:            IncomingCallback,
		UserID:          42,
		ChatID:          100,
		CallbackID:      "callback-1",
		CallbackData:    "exp:article:7",
		CallbackMessage: 9,
	})
	if err != nil {
		t.Fatalf("Handle(export article callback) error = %v", err)
	}
	if exports.articleID != 7 || exports.articleUserID != 42 {
		t.Fatalf("export user=%d article=%d", exports.articleUserID, exports.articleID)
	}
	if out.DocumentPath != "/tmp/Go Guide.md" || !strings.Contains(out.Text, "Markdown") {
		t.Fatalf("out = %+v", out)
	}
	if out.CallbackID != "callback-1" {
		t.Fatalf("callback id = %q", out.CallbackID)
	}
}

func TestRouterScheduleButtonFlow(t *testing.T) {
	ctx := context.Background()
	groups := &fakeGroupController{
		groups: []domain.SourceGroup{{ID: 7, Name: "AI"}},
	}
	schedule := &fakeScheduleController{
		created: &domain.SummarySchedule{ID: 3, GroupID: 7, Cron: "09:00", Timezone: "Europe/Moscow", Enabled: true, SummaryType: "standard"},
		job:     &domain.Job{ID: 11, Status: domain.JobStatusPending},
	}
	router := NewRouterWithAllControllers(42, NewMemoryStateStore(), nil, nil, groups)
	router.SetSchedules(schedule)

	out, err := router.Handle(ctx, Incoming{
		Kind:            IncomingCallback,
		UserID:          42,
		ChatID:          100,
		CallbackID:      "callback-1",
		CallbackData:    ActionScheduleNew,
		CallbackMessage: 5,
	})
	if err != nil {
		t.Fatalf("Handle(schedule new) error = %v", err)
	}
	if out.Menu[0][0].Data != "sched:group:7" {
		t.Fatalf("menu = %+v", out.Menu)
	}

	out, err = router.Handle(ctx, Incoming{
		Kind:            IncomingCallback,
		UserID:          42,
		ChatID:          100,
		CallbackID:      "callback-2",
		CallbackData:    "sched:create:7:0900",
		CallbackMessage: 5,
	})
	if err != nil {
		t.Fatalf("Handle(schedule create) error = %v", err)
	}
	if schedule.createRequest.GroupID != 7 || schedule.createRequest.Time != "09:00" || schedule.createRequest.Timezone != "Europe/Moscow" {
		t.Fatalf("create request = %+v", schedule.createRequest)
	}
	if !strings.Contains(out.Text, "Расписание создано") || out.Menu[0][0].Data != "sched:run:3" {
		t.Fatalf("output = %q menu=%+v", out.Text, out.Menu)
	}

	out, err = router.Handle(ctx, Incoming{
		Kind:            IncomingCallback,
		UserID:          42,
		ChatID:          100,
		CallbackID:      "callback-3",
		CallbackData:    "sched:run:3",
		CallbackMessage: 5,
	})
	if err != nil {
		t.Fatalf("Handle(schedule run) error = %v", err)
	}
	if schedule.runID != 3 || !strings.Contains(out.Text, "job_id=11") {
		t.Fatalf("runID=%d output=%q", schedule.runID, out.Text)
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

type fakeGroupController struct {
	created       *domain.SourceGroup
	groups        []domain.SourceGroup
	withChats     *sourcegroups.GroupWithChats
	createdName   string
	addedGroupID  int64
	addedChatID   int64
	addedPriority int
}

func (f *fakeGroupController) Create(_ context.Context, _ int64, name, _ string) (*domain.SourceGroup, error) {
	f.createdName = name
	if f.created != nil {
		return f.created, nil
	}
	return &domain.SourceGroup{ID: 1, Name: name}, nil
}

func (f *fakeGroupController) Update(_ context.Context, _ int64, groupID int64, name, _ string) (*domain.SourceGroup, error) {
	return &domain.SourceGroup{ID: groupID, Name: name}, nil
}

func (f *fakeGroupController) Delete(context.Context, int64, int64) error {
	return nil
}

func (f *fakeGroupController) List(context.Context, int64) ([]domain.SourceGroup, error) {
	return f.groups, nil
}

func (f *fakeGroupController) AddChat(_ context.Context, _ int64, groupID, chatID int64, priority int, _ bool) error {
	f.addedGroupID = groupID
	f.addedChatID = chatID
	f.addedPriority = priority
	return nil
}

func (f *fakeGroupController) RemoveChat(context.Context, int64, int64, int64) error {
	return nil
}

func (f *fakeGroupController) ListChats(context.Context, int64, int64) (*sourcegroups.GroupWithChats, error) {
	return f.withChats, nil
}

type fakeCollectionController struct {
	request collection.Request
	result  *collection.Result
}

func (f *fakeCollectionController) CollectGroup(_ context.Context, req collection.Request) (*collection.Result, error) {
	f.request = req
	return f.result, nil
}

var summaryResultFixture = summary.GenerateResult{SummaryID: 1, SummaryJobID: 2, TopicsCount: 3, MessagesCount: 4, DuplicateCount: 5}

type fakeSummaryController struct {
	request summary.GenerateRequest
	result  *summary.GenerateResult
	err     error
}

func (f *fakeSummaryController) GenerateFromCollection(_ context.Context, req summary.GenerateRequest) (*summary.GenerateResult, error) {
	f.request = req
	return f.result, f.err
}

type fakeSummaryBrowserController struct {
	summaries     []domain.Summary
	summary       *domain.Summary
	topics        []domain.SummaryTopic
	card          *summary.TopicCard
	listUserID    int64
	listLimit     int
	getSummaryID  int64
	cardSummaryID int64
	cardPosition  int
}

func (f *fakeSummaryBrowserController) ListSummaries(_ context.Context, telegramUserID int64, limit int) ([]domain.Summary, error) {
	f.listUserID = telegramUserID
	f.listLimit = limit
	return f.summaries, nil
}

func (f *fakeSummaryBrowserController) GetSummary(_ context.Context, _ int64, summaryID int64) (*domain.Summary, error) {
	f.getSummaryID = summaryID
	if f.summary != nil {
		return f.summary, nil
	}
	return &domain.Summary{ID: summaryID, Title: "Digest", Overview: "Overview", TopicsCount: 2, MessagesCount: 12, SourcesCount: 3}, nil
}

func (f *fakeSummaryBrowserController) ListTopics(context.Context, int64, int64) ([]domain.SummaryTopic, error) {
	return f.topics, nil
}

func (f *fakeSummaryBrowserController) TopicCard(_ context.Context, _ int64, summaryID int64, position int) (*summary.TopicCard, error) {
	f.cardSummaryID = summaryID
	f.cardPosition = position
	return f.card, nil
}

type fakeScheduleController struct {
	items         []domain.SummarySchedule
	created       *domain.SummarySchedule
	updated       *domain.SummarySchedule
	job           *domain.Job
	createRequest schedules.Request
	runID         int64
	enabledID     int64
	enabled       bool
	deletedID     int64
}

func (f *fakeScheduleController) List(context.Context, int64) ([]domain.SummarySchedule, error) {
	return f.items, nil
}

func (f *fakeScheduleController) Get(_ context.Context, _ int64, scheduleID int64) (*domain.SummarySchedule, error) {
	for _, item := range f.items {
		if item.ID == scheduleID {
			return &item, nil
		}
	}
	if f.created != nil && f.created.ID == scheduleID {
		return f.created, nil
	}
	return &domain.SummarySchedule{ID: scheduleID, GroupID: 7, Cron: "09:00", Timezone: "Europe/Moscow", Enabled: true, SummaryType: "standard"}, nil
}

func (f *fakeScheduleController) Create(_ context.Context, req schedules.Request) (*domain.SummarySchedule, error) {
	f.createRequest = req
	if f.created != nil {
		return f.created, nil
	}
	return &domain.SummarySchedule{ID: 1, GroupID: req.GroupID, Cron: req.Time, Timezone: req.Timezone, Enabled: req.Enabled, SummaryType: req.SummaryType}, nil
}

func (f *fakeScheduleController) Delete(_ context.Context, _ int64, scheduleID int64) error {
	f.deletedID = scheduleID
	return nil
}

func (f *fakeScheduleController) SetEnabled(_ context.Context, _ int64, scheduleID int64, enabled bool) (*domain.SummarySchedule, error) {
	f.enabledID = scheduleID
	f.enabled = enabled
	if f.updated != nil {
		return f.updated, nil
	}
	return &domain.SummarySchedule{ID: scheduleID, GroupID: 7, Cron: "09:00", Timezone: "Europe/Moscow", Enabled: enabled, SummaryType: "standard"}, nil
}

func (f *fakeScheduleController) Run(_ context.Context, _ int64, scheduleID int64) (*domain.Job, error) {
	f.runID = scheduleID
	if f.job != nil {
		return f.job, nil
	}
	return &domain.Job{ID: 1, Status: domain.JobStatusPending}, nil
}

func (f *fakeScheduleController) ListRuns(context.Context, int64, int64, int) ([]domain.ScheduleRun, error) {
	return nil, nil
}

type fakeArticleController struct {
	result          *article.Result
	updated         *domain.Article
	summaryRequest  article.ConvertRequest
	topicRequest    article.ConvertRequest
	updateArticleID int64
	updateTitle     string
	updateTags      []string
}

func (f *fakeArticleController) ConvertSummary(_ context.Context, req article.ConvertRequest) (*article.Result, error) {
	f.summaryRequest = req
	return f.result, nil
}

func (f *fakeArticleController) ConvertTopic(_ context.Context, req article.ConvertRequest) (*article.Result, error) {
	f.topicRequest = req
	return f.result, nil
}

func (f *fakeArticleController) List(context.Context, int64, int) ([]domain.Article, error) {
	return nil, nil
}

func (f *fakeArticleController) Get(context.Context, int64, int64) (*domain.Article, error) {
	return nil, nil
}

func (f *fakeArticleController) UpdateMetadata(_ context.Context, _ int64, articleID int64, title string, tags []string) (*domain.Article, error) {
	f.updateArticleID = articleID
	f.updateTitle = title
	f.updateTags = tags
	if f.updated != nil {
		return f.updated, nil
	}
	return &domain.Article{ID: articleID, Title: title, Tags: tags}, nil
}

type fakeExportController struct {
	result        *obsidian.Result
	articleUserID int64
	articleID     int64
	summaryUserID int64
	summaryID     int64
}

func (f *fakeExportController) ExportArticle(_ context.Context, telegramUserID, articleID int64) (*obsidian.Result, error) {
	f.articleUserID = telegramUserID
	f.articleID = articleID
	return f.result, nil
}

func (f *fakeExportController) ExportSummary(_ context.Context, telegramUserID, summaryID int64) (*obsidian.Result, error) {
	f.summaryUserID = telegramUserID
	f.summaryID = summaryID
	return f.result, nil
}
