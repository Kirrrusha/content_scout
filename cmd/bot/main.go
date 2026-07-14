package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/article"
	"github.com/kirilllebedenko/content_scout/internal/config"
	"github.com/kirilllebedenko/content_scout/internal/logging"
	"github.com/kirilllebedenko/content_scout/internal/obsidian"
	"github.com/kirilllebedenko/content_scout/internal/schedules"
	"github.com/kirilllebedenko/content_scout/internal/sourcegroups"
	"github.com/kirilllebedenko/content_scout/internal/storage/postgres"
	"github.com/kirilllebedenko/content_scout/internal/summary"
	"github.com/kirilllebedenko/content_scout/internal/summary/llm"
	tgbot "github.com/kirilllebedenko/content_scout/internal/telegram/bot"
)

func main() {
	logger := logging.Bootstrap("bot")

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}
	logRuntime, err := logging.New(logging.Config{
		Service:          "bot",
		Format:           cfg.LogFormat,
		Level:            cfg.LogLevel,
		Dir:              cfg.LogDir,
		Retention:        cfg.LogRetention,
		RotationInterval: cfg.LogRotation,
	})
	if err != nil {
		logger.Error("configure logging failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = logRuntime.Close() }()
	logger = logRuntime.Logger
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
	defer func() { _ = db.Close() }()

	userRepo := postgres.NewUserRepository(db)
	internalAPITimeout := cfg.LLMTimeout + time.Minute
	if internalAPITimeout < 5*time.Minute {
		internalAPITimeout = 5 * time.Minute
	}
	apiClient := tgbot.NewAPIClient(cfg.InternalAPIURL, cfg.ServiceToken, &http.Client{Timeout: internalAPITimeout})
	groupService := sourcegroups.NewService(
		cfg.TelegramOwnerID,
		userRepo,
		postgres.NewSourceGroupRepository(db),
		postgres.NewTelegramChatRepository(db),
	)
	summaryRepo := postgres.NewSummaryRepository(db)
	summaryBrowser := summary.NewBrowser(cfg.TelegramOwnerID, userRepo, summaryRepo)
	articleService := article.NewService(
		cfg.TelegramOwnerID,
		userRepo,
		summaryRepo,
		postgres.NewMessageCollectionRepository(db),
		postgres.NewTelegramChatRepository(db),
		postgres.NewArticleRepository(db),
		llm.NewOpenAICompatible(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, &http.Client{Timeout: cfg.LLMTimeout}),
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

	scheduleService := schedules.NewService(cfg.TelegramOwnerID, userRepo, postgres.NewSummaryScheduleRepository(db), postgres.NewJobRepository(db))
	service, err := tgbot.NewServiceWithExports(cfg.TelegramBotToken, cfg.TelegramOwnerID, apiClient, apiClient, groupService, apiClient, apiClient, summaryBrowser, articleService, exportService, logger)
	if err != nil {
		logger.Error("create bot service failed", "error", err)
		os.Exit(1)
	}
	service.SetSchedules(scheduleService)

	if err := tgbot.RunWithShutdown(ctx, service); err != nil {
		logger.Error("bot service failed", "error", err)
		os.Exit(1)
	}
}
