package postgres

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

func TestUserRepositoryUpsertByTelegramID(t *testing.T) {
	ctx, db := openIntegrationDB(t)

	repo := NewUserRepository(db)
	user, err := repo.UpsertByTelegramID(ctx, time.Now().UnixNano())
	if err != nil {
		t.Fatalf("UpsertByTelegramID() error = %v", err)
	}

	found, err := repo.FindByTelegramID(ctx, user.TelegramUserID)
	if err != nil {
		t.Fatalf("FindByTelegramID() error = %v", err)
	}
	if found == nil || found.ID != user.ID {
		t.Fatalf("FindByTelegramID() = %+v, want user id %d", found, user.ID)
	}
}

func TestDomainRepositoriesIntegration(t *testing.T) {
	ctx, db := openIntegrationDB(t)

	userRepo := NewUserRepository(db)
	user, err := userRepo.UpsertByTelegramID(ctx, time.Now().UnixNano())
	if err != nil {
		t.Fatalf("UpsertByTelegramID() error = %v", err)
	}

	sessionRepo := NewTelegramSessionRepository(db)
	now := time.Now().UTC()
	session, err := sessionRepo.Upsert(ctx, domain.TelegramSession{
		UserID:        user.ID,
		StoragePath:   "/tmp/tdlib-test",
		Status:        domain.SessionStatusConnected,
		LastConnected: &now,
	})
	if err != nil {
		t.Fatalf("session Upsert() error = %v", err)
	}
	if session.Status != domain.SessionStatusConnected {
		t.Fatalf("session status = %q", session.Status)
	}

	folderRepo := NewTelegramFolderRepository(db)
	if err := folderRepo.UpsertMany(ctx, []domain.TelegramFolder{{
		UserID:     user.ID,
		TelegramID: 1,
		Name:       "Golang",
		SyncedAt:   now,
	}}); err != nil {
		t.Fatalf("folder UpsertMany() error = %v", err)
	}
	folders, err := folderRepo.ListByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("folder ListByUserID() error = %v", err)
	}
	if len(folders) != 1 {
		t.Fatalf("folders len = %d, want 1", len(folders))
	}

	username := "golang_digest"
	chatRepo := NewTelegramChatRepository(db)
	if err := chatRepo.UpsertMany(ctx, []domain.TelegramChat{{
		UserID:         user.ID,
		TelegramChatID: -100123,
		Title:          "Golang Digest",
		Username:       &username,
		Type:           domain.ChatTypeChannel,
		UnreadCount:    4,
		LastMessageID:  900,
	}}); err != nil {
		t.Fatalf("chat UpsertMany() error = %v", err)
	}
	chat, err := chatRepo.FindByTelegramChatID(ctx, user.ID, -100123)
	if err != nil {
		t.Fatalf("FindByTelegramChatID() error = %v", err)
	}
	if chat == nil || chat.Username == nil || *chat.Username != username {
		t.Fatalf("chat = %+v, want username %q", chat, username)
	}

	groupRepo := NewSourceGroupRepository(db)
	group, err := groupRepo.Create(ctx, domain.SourceGroup{
		UserID:      user.ID,
		Name:        "Backend",
		Description: "Backend channels",
	})
	if err != nil {
		t.Fatalf("group Create() error = %v", err)
	}
	if err := groupRepo.AddChat(ctx, domain.SourceGroupChat{
		GroupID:  group.ID,
		ChatID:   chat.ID,
		Priority: 10,
		Enabled:  true,
	}); err != nil {
		t.Fatalf("AddChat() error = %v", err)
	}
	groupChats, err := groupRepo.ListChats(ctx, group.ID)
	if err != nil {
		t.Fatalf("ListChats() error = %v", err)
	}
	if len(groupChats) != 1 || groupChats[0].ChatID != chat.ID {
		t.Fatalf("group chats = %+v, want chat id %d", groupChats, chat.ID)
	}

	positionRepo := NewReadPositionRepository(db)
	if err := positionRepo.Upsert(ctx, domain.ReadPosition{
		UserID:                  user.ID,
		ChatID:                  chat.ID,
		LastSummarizedMessageID: 555,
	}); err != nil {
		t.Fatalf("position Upsert() error = %v", err)
	}
	position, err := positionRepo.Find(ctx, user.ID, chat.ID)
	if err != nil {
		t.Fatalf("position Find() error = %v", err)
	}
	if position == nil || position.LastSummarizedMessageID != 555 {
		t.Fatalf("position = %+v, want message 555", position)
	}

	summaryRepo := NewSummaryRepository(db)
	job, err := summaryRepo.CreateJob(ctx, domain.SummaryJob{
		UserID:     user.ID,
		SourceType: domain.SummarySourceTypeGroup,
		SourceID:   group.ID,
		Status:     domain.JobStatusPending,
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if err := summaryRepo.UpdateJobStatus(ctx, job.ID, domain.JobStatusProcessing, nil); err != nil {
		t.Fatalf("UpdateJobStatus() error = %v", err)
	}
	summary, err := summaryRepo.CreateSummary(ctx, domain.Summary{
		JobID:         job.ID,
		Title:         "Digest",
		Overview:      "Overview",
		MessagesCount: 10,
		SourcesCount:  1,
		Markdown:      "# Digest",
	}, []domain.SummaryTopic{{
		Title:         "Topic",
		ShortSummary:  "Short",
		FullSummary:   "Full",
		Category:      "Golang",
		Importance:    9,
		Confidence:    domain.ConfidenceHigh,
		MessagesCount: 3,
		SourcesCount:  1,
		Position:      1,
	}})
	if err != nil {
		t.Fatalf("CreateSummary() error = %v", err)
	}
	topics, err := summaryRepo.ListTopics(ctx, summary.ID)
	if err != nil {
		t.Fatalf("ListTopics() error = %v", err)
	}
	if len(topics) != 1 || topics[0].Title != "Topic" {
		t.Fatalf("topics = %+v", topics)
	}

	articleRepo := NewArticleRepository(db)
	slug := "digest-" + time.Now().Format("20060102150405.000000000")
	article, err := articleRepo.Create(ctx, domain.Article{
		UserID:          user.ID,
		Title:           "Digest Article",
		Slug:            slug,
		Type:            domain.ArticleTypeAnalysis,
		Status:          domain.ArticleStatusDraft,
		ContentMarkdown: "# Digest Article",
	}, []domain.ArticleSource{{
		TelegramChatID: chat.TelegramChatID,
		MessageID:      900,
		SourceTitle:    chat.Title,
		SourceURL:      "https://t.me/golang_digest/900",
		PublishedAt:    now,
	}})
	if err != nil {
		t.Fatalf("article Create() error = %v", err)
	}
	foundArticle, err := articleRepo.FindBySlug(ctx, user.ID, article.Slug)
	if err != nil {
		t.Fatalf("FindBySlug() error = %v", err)
	}
	if foundArticle == nil || foundArticle.ID != article.ID {
		t.Fatalf("article = %+v, want id %d", foundArticle, article.ID)
	}

	exportRepo := NewObsidianExportRepository(db)
	export, err := exportRepo.Create(ctx, domain.ObsidianExport{
		ArticleID:    &article.ID,
		SummaryID:    &summary.ID,
		FileName:     "digest.md",
		VaultPath:    "Articles/Golang/digest.md",
		ExportMethod: "telegram_document",
		ContentHash:  "hash-" + slug,
	})
	if err != nil {
		t.Fatalf("export Create() error = %v", err)
	}
	foundExport, err := exportRepo.FindByContentHash(ctx, export.ContentHash)
	if err != nil {
		t.Fatalf("FindByContentHash() error = %v", err)
	}
	if foundExport == nil || foundExport.ID != export.ID {
		t.Fatalf("export = %+v, want id %d", foundExport, export.ID)
	}
}

func openIntegrationDB(t *testing.T) (context.Context, *sql.DB) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	db, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := RunMigrations(ctx, db, migrationsDir(t), MigrationUp); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}
	return ctx, db
}

func migrationsDir(t *testing.T) string {
	t.Helper()
	candidates := []string{"migrations", "../../../migrations"}
	for _, candidate := range candidates {
		matches, err := filepath.Glob(filepath.Join(candidate, "*.sql"))
		if err == nil && len(matches) > 0 {
			return candidate
		}
	}
	t.Fatal("migrations directory not found")
	return ""
}
