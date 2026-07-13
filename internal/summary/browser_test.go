package summary

import (
	"context"
	"errors"
	"testing"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

func TestBrowserListsOwnerSummaries(t *testing.T) {
	ctx := context.Background()
	users := &fakeUsers{user: &domain.User{ID: 1, TelegramUserID: 42}}
	summaries := &fakeSummaries{
		owned: []domain.Summary{{ID: 10, Title: "Digest"}},
	}
	browser := NewBrowser(42, users, summaries)

	items, err := browser.ListSummaries(ctx, 42, 5)
	if err != nil {
		t.Fatalf("ListSummaries() error = %v", err)
	}
	if summaries.listUserID != 1 || summaries.listLimit != 5 {
		t.Fatalf("list user=%d limit=%d", summaries.listUserID, summaries.listLimit)
	}
	if len(items) != 1 || items[0].Title != "Digest" {
		t.Fatalf("items = %+v", items)
	}
}

func TestBrowserRejectsForeignSummary(t *testing.T) {
	ctx := context.Background()
	users := &fakeUsers{user: &domain.User{ID: 1, TelegramUserID: 42}}
	summaries := &fakeSummaries{
		owned: []domain.Summary{{ID: 10, Title: "Owned"}},
	}
	browser := NewBrowser(42, users, summaries)

	_, err := browser.GetSummary(ctx, 42, 99)
	if !errors.Is(err, ErrSummaryNotFound) {
		t.Fatalf("GetSummary() error = %v, want ErrSummaryNotFound", err)
	}
}

func TestBrowserRejectsNonOwner(t *testing.T) {
	browser := NewBrowser(42, &fakeUsers{user: &domain.User{ID: 1, TelegramUserID: 42}}, &fakeSummaries{})

	_, err := browser.ListSummaries(context.Background(), 99, 10)
	if !errors.Is(err, tdlib.ErrUnauthorizedOwner) {
		t.Fatalf("ListSummaries() error = %v, want unauthorized", err)
	}
}
