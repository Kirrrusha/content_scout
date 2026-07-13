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
	"github.com/kirilllebedenko/content_scout/internal/obsidian"
	"github.com/kirilllebedenko/content_scout/internal/scheduler"
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
	if cfg.TelegramOwnerID == 0 {
		logger.Warn("telegram owner id is not configured; scheduler worker is idle")
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
	summaryRepo := postgres.NewSummaryRepository(db)
	collectionRepo := postgres.NewMessageCollectionRepository(db)

	collector := collection.NewService(
		cfg.TelegramOwnerID,
		userRepo,
		sessionRepo,
		postgres.NewSourceGroupRepository(db),
		postgres.NewTelegramChatRepository(db),
		postgres.NewReadPositionRepository(db),
		collectionRepo,
		factory,
	)
	summarizer := summary.NewService(
		cfg.TelegramOwnerID,
		userRepo,
		collectionRepo,
		summaryRepo,
		postgres.NewTelegramChatRepository(db),
		llm.NewOpenAICompatible(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, &http.Client{Timeout: 60 * time.Second}),
	)
	exporter := obsidian.NewService(
		cfg.TelegramOwnerID,
		cfg.ExportDir,
		userRepo,
		postgres.NewArticleRepository(db),
		summaryRepo,
		postgres.NewObsidianExportRepository(db),
	)
	if cfg.ObsidianAPIKey != "" {
		exporter = obsidian.NewServiceWithREST(
			cfg.TelegramOwnerID,
			cfg.ExportDir,
			userRepo,
			postgres.NewArticleRepository(db),
			summaryRepo,
			postgres.NewObsidianExportRepository(db),
			obsidian.NewRESTClient(cfg.ObsidianRESTURL, cfg.ObsidianAPIKey, cfg.ObsidianInsecure),
		)
	}
	service := scheduler.NewService(
		cfg.TelegramOwnerID,
		postgres.NewSummaryScheduleRepository(db),
		collector,
		summarizer,
		exporter,
		logger,
	)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	logger.Info("summary scheduler worker started")
	for {
		count, err := service.RunDue(ctx)
		if err != nil {
			logger.Warn("run due schedules failed", "error", err)
		} else if count > 0 {
			logger.Info("due schedules processed", "count", count)
		}
		select {
		case <-ctx.Done():
			logger.Info("summary scheduler worker stopped")
			return
		case <-ticker.C:
		}
	}
}
