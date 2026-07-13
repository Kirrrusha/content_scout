package postgres

import (
	"context"
	"os"
	"testing"
)

func TestUserRepositoryUpsertByTelegramID(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	db, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	repo := NewUserRepository(db)
	user, err := repo.UpsertByTelegramID(ctx, 777)
	if err != nil {
		t.Fatalf("UpsertByTelegramID() error = %v", err)
	}
	if user.TelegramUserID != 777 {
		t.Fatalf("TelegramUserID = %d, want 777", user.TelegramUserID)
	}

	found, err := repo.FindByTelegramID(ctx, 777)
	if err != nil {
		t.Fatalf("FindByTelegramID() error = %v", err)
	}
	if found == nil || found.ID != user.ID {
		t.Fatalf("FindByTelegramID() = %+v, want user id %d", found, user.ID)
	}
}
