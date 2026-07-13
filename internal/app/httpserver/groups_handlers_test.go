package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/sourcegroups"
)

func TestGroupsCreateHandler(t *testing.T) {
	groups := &fakeHTTPGroups{created: &domain.SourceGroup{ID: 1, Name: "Golang"}}
	server := NewWithAllControllers(":0", nil, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), nil, nil, groups)

	req := httptest.NewRequest(http.MethodPost, "/groups", bytes.NewBufferString(`{"telegram_user_id":42,"name":"Golang"}`))
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if groups.createdName != "Golang" {
		t.Fatalf("createdName = %q", groups.createdName)
	}
	var response groupResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ID != 1 || response.Name != "Golang" {
		t.Fatalf("response = %+v", response)
	}
}

func TestGroupChatsAddHandler(t *testing.T) {
	groups := &fakeHTTPGroups{}
	server := NewWithAllControllers(":0", nil, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), nil, nil, groups)

	req := httptest.NewRequest(http.MethodPost, "/groups/1/chats", bytes.NewBufferString(`{"telegram_user_id":42,"chat_id":10,"priority":4}`))
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if groups.addedGroupID != 1 || groups.addedChatID != 10 || groups.addedPriority != 4 {
		t.Fatalf("added group=%d chat=%d priority=%d", groups.addedGroupID, groups.addedChatID, groups.addedPriority)
	}
}

func TestCollectionGroupCreateHandler(t *testing.T) {
	collector := &fakeHTTPCollector{
		result: &collection.Result{JobID: 7, UserID: 1, GroupID: 2, ChatsCount: 3, MessagesCount: 4},
	}
	server := NewWithRuntime(":0", nil, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), nil, nil, nil, collector)

	req := httptest.NewRequest(http.MethodPost, "/collections/group/2", bytes.NewBufferString(`{"telegram_user_id":42,"mode":"new","limit":50}`))
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if collector.request.GroupID != 2 || collector.request.Mode != domain.CollectionModeNewOnly || collector.request.Limit != 50 {
		t.Fatalf("request = %+v", collector.request)
	}
	var response collectionResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.JobID != 7 || response.MessagesCount != 4 {
		t.Fatalf("response = %+v", response)
	}
}

type fakeHTTPGroups struct {
	created       *domain.SourceGroup
	groups        []domain.SourceGroup
	withChats     *sourcegroups.GroupWithChats
	createdName   string
	addedGroupID  int64
	addedChatID   int64
	addedPriority int
}

func (f *fakeHTTPGroups) Create(_ context.Context, _ int64, name, description string) (*domain.SourceGroup, error) {
	f.createdName = name
	if f.created != nil {
		return f.created, nil
	}
	return &domain.SourceGroup{ID: 1, Name: name, Description: description}, nil
}

func (f *fakeHTTPGroups) Update(_ context.Context, _ int64, groupID int64, name, description string) (*domain.SourceGroup, error) {
	return &domain.SourceGroup{ID: groupID, Name: name, Description: description}, nil
}

func (f *fakeHTTPGroups) Delete(context.Context, int64, int64) error {
	return nil
}

func (f *fakeHTTPGroups) List(context.Context, int64) ([]domain.SourceGroup, error) {
	return f.groups, nil
}

func (f *fakeHTTPGroups) AddChat(_ context.Context, _ int64, groupID, chatID int64, priority int, _ bool) error {
	f.addedGroupID = groupID
	f.addedChatID = chatID
	f.addedPriority = priority
	return nil
}

func (f *fakeHTTPGroups) RemoveChat(context.Context, int64, int64, int64) error {
	return nil
}

func (f *fakeHTTPGroups) ListChats(context.Context, int64, int64) (*sourcegroups.GroupWithChats, error) {
	return f.withChats, nil
}

type fakeHTTPCollector struct {
	request collection.Request
	result  *collection.Result
}

func (f *fakeHTTPCollector) CollectGroup(_ context.Context, req collection.Request) (*collection.Result, error) {
	f.request = req
	return f.result, nil
}
