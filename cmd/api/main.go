package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/app/httpserver"
	"github.com/kirilllebedenko/content_scout/internal/article"
	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/config"
	"github.com/kirilllebedenko/content_scout/internal/obsidian"
	"github.com/kirilllebedenko/content_scout/internal/sourcegroups"
	"github.com/kirilllebedenko/content_scout/internal/storage/postgres"
	"github.com/kirilllebedenko/content_scout/internal/summary"
	"github.com/kirilllebedenko/content_scout/internal/summary/llm"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
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
	summaryRepo := postgres.NewSummaryRepository(db)
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
		summaryRepo,
		postgres.NewTelegramChatRepository(db),
		llm.NewOpenAICompatible(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, &http.Client{Timeout: 60 * time.Second}),
	)
	summaryBrowser := summary.NewBrowser(cfg.TelegramOwnerID, userRepo, summaryRepo)
	articleService := article.NewService(
		cfg.TelegramOwnerID,
		userRepo,
		summaryRepo,
		postgres.NewMessageCollectionRepository(db),
		postgres.NewTelegramChatRepository(db),
		postgres.NewArticleRepository(db),
		llm.NewOpenAICompatible(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, &http.Client{Timeout: 60 * time.Second}),
	)
	exportService := obsidian.NewService(
		cfg.TelegramOwnerID,
		cfg.ExportDir,
		userRepo,
		postgres.NewArticleRepository(db),
		summaryRepo,
		postgres.NewObsidianExportRepository(db),
	)
	if cfg.ObsidianAPIKey != "" {
		exportService = obsidian.NewServiceWithREST(
			cfg.TelegramOwnerID,
			cfg.ExportDir,
			userRepo,
			postgres.NewArticleRepository(db),
			summaryRepo,
			postgres.NewObsidianExportRepository(db),
			obsidian.NewRESTClient(cfg.ObsidianRESTURL, cfg.ObsidianAPIKey, cfg.ObsidianInsecure),
		)
	}

	server := httpserver.NewWithExports(cfg.HTTPAddr, db, logger, authService, syncService, groupService, collectionService, summaryService, summaryBrowser, articleService, exportService)
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("api shutdown failed", "error", err)
			os.Exit(1)
		}
	case err := <-errCh:
		if err != nil {
			logger.Error("api server failed", "error", err)
			os.Exit(1)
		}
	}
}
