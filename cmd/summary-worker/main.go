package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/config"
	"github.com/kirilllebedenko/content_scout/internal/jobs"
	"github.com/kirilllebedenko/content_scout/internal/logging"
	"github.com/kirilllebedenko/content_scout/internal/obsidian"
	"github.com/kirilllebedenko/content_scout/internal/scheduler"
	"github.com/kirilllebedenko/content_scout/internal/storage/postgres"
	"github.com/kirilllebedenko/content_scout/internal/summary"
	"github.com/kirilllebedenko/content_scout/internal/summary/llm"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

func main() {
	logger := logging.Bootstrap("summary-worker")

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}
	logRuntime, err := logging.New(logging.Config{
		Service:          "summary-worker",
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
	defer logRuntime.Close()
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

	factory := tdlib.NewClientFactory(tdlib.ClientConfig{
		APIID:   cfg.TelegramAPIID,
		APIHash: cfg.TelegramAPIHash,
	})
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := tdlib.CloseClientFactory(shutdownCtx, factory); err != nil {
			logger.Error("tdlib shutdown failed", "error", err)
		}
	}()
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
		postgres.NewReadPositionRepository(db),
		llm.NewOpenAICompatible(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, &http.Client{Timeout: cfg.LLMTimeout}),
	)
	summarizer.SetTelegramReadMarker(tdlib.NewReadService(cfg.TelegramOwnerID, userRepo, sessionRepo, factory))
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
	jobRepo := postgres.NewJobRepository(db)
	workerID := cfg.WorkerID
	if workerID == "" {
		hostname, _ := os.Hostname()
		workerID = "summary-worker"
		if hostname != "" {
			workerID += "-" + hostname
		}
	}
	jobWorker := jobs.NewWorker(jobRepo, jobs.MultiHandler{
		jobs.ScheduledPipelineHandler{Scheduler: service},
		jobs.SummaryGenerationHandler{Summarizer: summarizer},
	}, logger, workerID)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	logger.Info("summary scheduler worker started", "worker_id", workerID)
	for {
		count, err := service.EnqueueDue(ctx, jobRepo)
		if err != nil {
			logger.Warn("enqueue due schedules failed", "error", err)
		} else if count > 0 {
			logger.Info("due schedules enqueued", "count", count)
		}
		if cfg.SummaryRetention > 0 {
			cutoff := time.Now().Add(-cfg.SummaryRetention)
			deleted, err := summaryRepo.DeleteSummariesOlderThan(ctx, cutoff)
			if err != nil {
				logger.Warn("delete old summaries failed", "error", err)
			} else if deleted > 0 {
				logger.Info("old summaries deleted", "count", deleted, "cutoff", cutoff)
			}
		}
		for {
			claimed, err := jobWorker.RunOnce(ctx)
			if err != nil {
				logger.Warn("run queued job failed", "error", err)
				break
			}
			if !claimed {
				break
			}
		}
		select {
		case <-ctx.Done():
			logger.Info("summary scheduler worker stopped")
			return
		case <-ticker.C:
		}
	}
}
