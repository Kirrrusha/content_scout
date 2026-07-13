package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kirilllebedenko/content_scout/internal/config"
	tgbot "github.com/kirilllebedenko/content_scout/internal/telegram/bot"
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

	service, err := tgbot.NewService(cfg.TelegramBotToken, cfg.TelegramOwnerID, logger)
	if err != nil {
		logger.Error("create bot service failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := tgbot.RunWithShutdown(ctx, service); err != nil {
		logger.Error("bot service failed", "error", err)
		os.Exit(1)
	}
}
