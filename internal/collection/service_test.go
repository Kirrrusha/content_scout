package collection

import (
	"context"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

func TestCollectGroupCollectsNewMessagesWithoutUpdatingReadPosition(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserRepo()
	user, err := users.UpsertByTelegramID(ctx, 42)
	if err != nil {
		t.Fatalf("UpsertByTelegramID() error = %v", err)
	}
	sessions := newMemorySessionRepo()
	_, err = sessions.Upsert(ctx, domain.TelegramSession{UserID: user.ID, StoragePath: "/tmp/tdlib", Status: domain.SessionStatusConnected})
	if err != nil {
		t.Fatalf("session Upsert() error = %v", err)
	}
	groups := newMemoryGroupRepo()
	group, err := groups.Create(ctx, domain.SourceGroup{UserID: user.ID, Name: "Golang"})
	if err != nil {
		t.Fatalf("group Create() error = %v", err)
	}
	_ = groups.AddChat(ctx, domain.SourceGroupChat{GroupID: group.ID, ChatID: 10, Enabled: true})
	chats := newMemoryChatRepo([]domain.TelegramChat{{
		ID:             10,
		UserID:         user.ID,
		TelegramChatID: -100,
		Title:          "Backend",
		Type:           domain.ChatTypeChannel,
	}})
	positions := newMemoryReadPositionRepo()
	err = positions.Upsert(ctx, domain.ReadPosition{UserID: user.ID, ChatID: 10, LastSummarizedMessageID: 100})
	if err != nil {
		t.Fatalf("position Upsert() error = %v", err)
	}
	collections := newMemoryCollectionRepo()
	client := &fakeClient{
		state: tdlib.AuthorizationStateReady,
		history: []domain.TelegramMessage{
			{ChatID: -100, MessageID: 99, Date: time.Now(), Text: "old"},
			{ChatID: -100, MessageID: 101, Date: time.Now(), Text: "new"},
			{ChatID: -100, MessageID: 102, Date: time.Now()},
		},
	}
	service := NewService(42, users, sessions, groups, chats, positions, collections, fakeFactory{client: client})

	result, err := service.CollectGroup(ctx, Request{
		TelegramUserID: 42,
		GroupID:        group.ID,
		Mode:           domain.CollectionModeNewOnly,
		Limit:          50,
	})
	if err != nil {
		t.Fatalf("CollectGroup() error = %v", err)
	}
	if result.MessagesCount != 1 {
		t.Fatalf("MessagesCount = %d, want 1", result.MessagesCount)
	}
	if client.fromMessageID != 100 {
		t.Fatalf("fromMessageID = %d, want 100", client.fromMessageID)
	}
	position, err := positions.Find(ctx, user.ID, 10)
	if err != nil {
		t.Fatalf("position Find() error = %v", err)
	}
	if position.LastSummarizedMessageID != 100 {
		t.Fatalf("read position advanced to %d, want 100", position.LastSummarizedMessageID)
	}
	messages, err := collections.ListMessages(ctx, result.JobID)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(messages) != 1 || messages[0].MessageID != 101 {
		t.Fatalf("messages = %+v", messages)
	}
}

type fakeFactory struct {
	client *fakeClient
}

func (f fakeFactory) NewClient(string) (tdlib.TelegramClient, error) {
	return f.client, nil
}

type fakeClient struct {
	state         tdlib.AuthorizationState
	history       []domain.TelegramMessage
	fromMessageID int64
}

func (c *fakeClient) Start(context.Context) error { return nil }
func (c *fakeClient) Stop(context.Context) error  { return nil }
func (c *fakeClient) AuthorizationState(context.Context) (tdlib.AuthorizationState, error) {
	return c.state, nil
}
func (c *fakeClient) SubmitPhoneNumber(context.Context, string) error { return nil }
func (c *fakeClient) SubmitCode(context.Context, string) error        { return nil }
func (c *fakeClient) SubmitPassword(context.Context, string) error    { return nil }
func (c *fakeClient) ListFolders(context.Context) ([]domain.TelegramFolder, error) {
	return nil, nil
}
func (c *fakeClient) ListChats(context.Context, tdlib.ChatList) ([]domain.TelegramChat, error) {
	return nil, nil
}
func (c *fakeClient) ListFolderChats(context.Context, int32) ([]domain.TelegramChat, error) {
	return nil, nil
}
func (c *fakeClient) GetChatHistory(_ context.Context, _ int64, fromMessageID int64, _ int) ([]domain.TelegramMessage, error) {
	c.fromMessageID = fromMessageID
	return c.history, nil
}
func (c *fakeClient) MarkMessagesRead(context.Context, int64, []int64) error { return nil }

type memoryUserRepo struct {
	nextID int64
	users  map[int64]domain.User
}

func newMemoryUserRepo() *memoryUserRepo {
	return &memoryUserRepo{nextID: 1, users: make(map[int64]domain.User)}
}

func (r *memoryUserRepo) UpsertByTelegramID(_ context.Context, telegramUserID int64) (*domain.User, error) {
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
	byUserID map[int64]domain.TelegramSession
}

func newMemorySessionRepo() *memorySessionRepo {
	return &memorySessionRepo{byUserID: make(map[int64]domain.TelegramSession)}
}

func (r *memorySessionRepo) Upsert(_ context.Context, session domain.TelegramSession) (*domain.TelegramSession, error) {
	if session.ID == 0 {
		session.ID = int64(len(r.byUserID) + 1)
	}
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

type memoryGroupRepo struct {
	nextID int64
	groups map[int64]domain.SourceGroup
	links  []domain.SourceGroupChat
}

func newMemoryGroupRepo() *memoryGroupRepo {
	return &memoryGroupRepo{nextID: 1, groups: make(map[int64]domain.SourceGroup)}
}

func (r *memoryGroupRepo) Create(_ context.Context, group domain.SourceGroup) (*domain.SourceGroup, error) {
	group.ID = r.nextID
	r.nextID++
	r.groups[group.ID] = group
	return &group, nil
}
func (r *memoryGroupRepo) Update(_ context.Context, group domain.SourceGroup) (*domain.SourceGroup, error) {
	r.groups[group.ID] = group
	return &group, nil
}
func (r *memoryGroupRepo) Delete(_ context.Context, _, groupID int64) error {
	delete(r.groups, groupID)
	return nil
}
func (r *memoryGroupRepo) ListByUserID(_ context.Context, userID int64) ([]domain.SourceGroup, error) {
	var out []domain.SourceGroup
	for _, group := range r.groups {
		if group.UserID == userID {
			out = append(out, group)
		}
	}
	return out, nil
}
func (r *memoryGroupRepo) AddChat(_ context.Context, link domain.SourceGroupChat) error {
	r.links = append(r.links, link)
	return nil
}
func (r *memoryGroupRepo) RemoveChat(context.Context, int64, int64) error { return nil }
func (r *memoryGroupRepo) ListChats(_ context.Context, groupID int64) ([]domain.SourceGroupChat, error) {
	var out []domain.SourceGroupChat
	for _, link := range r.links {
		if link.GroupID == groupID {
			out = append(out, link)
		}
	}
	return out, nil
}

type memoryChatRepo struct{ chats []domain.TelegramChat }

func newMemoryChatRepo(chats []domain.TelegramChat) *memoryChatRepo {
	return &memoryChatRepo{chats: chats}
}
func (r *memoryChatRepo) UpsertMany(context.Context, []domain.TelegramChat) error { return nil }
func (r *memoryChatRepo) ListByUserID(_ context.Context, userID int64) ([]domain.TelegramChat, error) {
	var out []domain.TelegramChat
	for _, chat := range r.chats {
		if chat.UserID == userID {
			out = append(out, chat)
		}
	}
	return out, nil
}
func (r *memoryChatRepo) FindByTelegramChatID(context.Context, int64, int64) (*domain.TelegramChat, error) {
	return nil, nil
}

type memoryReadPositionRepo struct {
	positions map[[2]int64]domain.ReadPosition
}

func newMemoryReadPositionRepo() *memoryReadPositionRepo {
	return &memoryReadPositionRepo{positions: make(map[[2]int64]domain.ReadPosition)}
}
func (r *memoryReadPositionRepo) Upsert(_ context.Context, position domain.ReadPosition) error {
	r.positions[[2]int64{position.UserID, position.ChatID}] = position
	return nil
}
func (r *memoryReadPositionRepo) Find(_ context.Context, userID, chatID int64) (*domain.ReadPosition, error) {
	position, ok := r.positions[[2]int64{userID, chatID}]
	if !ok {
		return nil, nil
	}
	return &position, nil
}

type memoryCollectionRepo struct {
	nextID   int64
	jobs     map[int64]domain.MessageCollectionJob
	messages []domain.CollectedMessage
}

func newMemoryCollectionRepo() *memoryCollectionRepo {
	return &memoryCollectionRepo{nextID: 1, jobs: make(map[int64]domain.MessageCollectionJob)}
}
func (r *memoryCollectionRepo) CreateJob(_ context.Context, job domain.MessageCollectionJob) (*domain.MessageCollectionJob, error) {
	job.ID = r.nextID
	r.nextID++
	r.jobs[job.ID] = job
	return &job, nil
}
func (r *memoryCollectionRepo) FindJob(_ context.Context, jobID int64) (*domain.MessageCollectionJob, error) {
	job, ok := r.jobs[jobID]
	if !ok {
		return nil, nil
	}
	return &job, nil
}
func (r *memoryCollectionRepo) UpdateJobStatus(_ context.Context, jobID int64, status domain.JobStatus, message *string) error {
	job := r.jobs[jobID]
	job.Status = status
	job.Error = message
	r.jobs[jobID] = job
	return nil
}
func (r *memoryCollectionRepo) AddMessages(_ context.Context, messages []domain.CollectedMessage) error {
	r.messages = append(r.messages, messages...)
	return nil
}
func (r *memoryCollectionRepo) ListMessages(_ context.Context, jobID int64) ([]domain.CollectedMessage, error) {
	var out []domain.CollectedMessage
	for _, message := range r.messages {
		if message.JobID == jobID {
			out = append(out, message)
		}
	}
	return out, nil
}
