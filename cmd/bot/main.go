package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/config"
	"github.com/kirilllebedenko/content_scout/internal/sourcegroups"
	"github.com/kirilllebedenko/content_scout/internal/storage/postgres"
	"github.com/kirilllebedenko/content_scout/internal/summary"
	"github.com/kirilllebedenko/content_scout/internal/summary/llm"
	tgbot "github.com/kirilllebedenko/content_scout/internal/telegram/bot"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}
	if cfg.TelegramBotToken == "" {
		logger.Warn("telegram bot token is not configured; bot shell is idle")
		return
	}
	if cfg.TelegramOwnerID == 0 {
		logger.Warn("telegram owner id is not configured; bot shell is idle")
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect database failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	factory := tdlib.UnavailableClientFactory{}
	userRepo := postgres.NewUserRepository(db)
	sessionRepo := postgres.NewTelegramSessionRepository(db)
	authService := tdlib.NewAuthService(tdlib.AuthServiceConfig{
		OwnerTelegramID: cfg.TelegramOwnerID,
		TelegramAPIID:   cfg.TelegramAPIID,
		TelegramAPIHash: cfg.TelegramAPIHash,
		StorageBaseDir:  cfg.TDLibDatabaseDir,
	}, userRepo, sessionRepo, factory)
	syncService := tdlib.NewSyncService(
		cfg.TelegramOwnerID,
		userRepo,
		sessionRepo,
		postgres.NewTelegramFolderRepository(db),
		postgres.NewTelegramChatRepository(db),
		factory,
	)
	groupService := sourcegroups.NewService(
		cfg.TelegramOwnerID,
		userRepo,
		postgres.NewSourceGroupRepository(db),
		postgres.NewTelegramChatRepository(db),
	)
	collectionService := collection.NewService(
		cfg.TelegramOwnerID,
		userRepo,
		sessionRepo,
		postgres.NewSourceGroupRepository(db),
		postgres.NewTelegramChatRepository(db),
		postgres.NewReadPositionRepository(db),
		postgres.NewMessageCollectionRepository(db),
		factory,
	)
	summaryService := summary.NewService(
		cfg.TelegramOwnerID,
		userRepo,
		postgres.NewMessageCollectionRepository(db),
		postgres.NewSummaryRepository(db),
		postgres.NewTelegramChatRepository(db),
		llm.NewOpenAICompatible(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, &http.Client{Timeout: 60 * time.Second}),
	)

	service, err := tgbot.NewServiceWithServices(cfg.TelegramBotToken, cfg.TelegramOwnerID, authService, syncService, groupService, collectionService, summaryService, logger)
	if err != nil {
		logger.Error("create bot service failed", "error", err)
		os.Exit(1)
	}

	if err := tgbot.RunWithShutdown(ctx, service); err != nil {
		logger.Error("bot service failed", "error", err)
		os.Exit(1)
	}
}
