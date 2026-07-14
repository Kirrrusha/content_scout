package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/app/httpserver"
	"github.com/kirilllebedenko/content_scout/internal/article"
	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/config"
	"github.com/kirilllebedenko/content_scout/internal/logging"
	"github.com/kirilllebedenko/content_scout/internal/obsidian"
	"github.com/kirilllebedenko/content_scout/internal/schedules"
	"github.com/kirilllebedenko/content_scout/internal/sourcegroups"
	"github.com/kirilllebedenko/content_scout/internal/storage/postgres"
	"github.com/kirilllebedenko/content_scout/internal/summary"
	"github.com/kirilllebedenko/content_scout/internal/summary/llm"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

func main() {
	logger := logging.Bootstrap("api")

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}
	logRuntime, err := logging.New(logging.Config{
		Service:          "api",
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
	stderrPrefixer, err := logging.StartStderrTimestampPrefixer(nil)
	if err != nil {
		logger.Error("configure stderr timestamp prefixer failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := stderrPrefixer.Close(); err != nil {
			logger.Error("close stderr timestamp prefixer failed", "error", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect database failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	factory := tdlib.NewClientFactory(tdlib.ClientConfig{
		APIID:   cfg.TelegramAPIID,
		APIHash: cfg.TelegramAPIHash,
	})
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
		postgres.NewSourceGroupRepository(db),
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
		postgres.NewReadPositionRepository(db),
		llm.NewOpenAICompatible(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, &http.Client{Timeout: cfg.LLMTimeout}),
	)
	summaryService.SetTelegramReadMarker(tdlib.NewReadService(cfg.TelegramOwnerID, userRepo, sessionRepo, factory))
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

	scheduleRepo := postgres.NewSummaryScheduleRepository(db)
	jobRepo := postgres.NewJobRepository(db)
	scheduleService := schedules.NewService(cfg.TelegramOwnerID, userRepo, scheduleRepo, jobRepo)
	serverOptions := httpserver.DefaultOptions()
	serverOptions.ServiceToken = cfg.ServiceToken
	serverOptions.RequireAuth = true
	serverOptions.WriteTimeout = 5 * time.Minute
	server := httpserver.NewWithOptions(
		cfg.HTTPAddr,
		db,
		logger,
		serverOptions,
		authService,
		syncService,
		groupService,
		collectionService,
		summaryService,
		summaryBrowser,
		articleService,
		exportService,
	)
	server.SetSchedules(scheduleService)
	server.SetJobs(jobRepo)
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
		if err := tdlib.CloseClientFactory(shutdownCtx, factory); err != nil {
			logger.Error("tdlib shutdown failed", "error", err)
			os.Exit(1)
		}
	case err := <-errCh:
		if err != nil {
			logger.Error("api server failed", "error", err)
			os.Exit(1)
		}
	}
}
