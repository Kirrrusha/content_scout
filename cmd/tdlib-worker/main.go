package main

import (
	"os"

	"github.com/kirilllebedenko/content_scout/internal/config"
	"github.com/kirilllebedenko/content_scout/internal/logging"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

func main() {
	logger := logging.Bootstrap("tdlib-worker")
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}
	logRuntime, err := logging.New(logging.Config{
		Service:          "tdlib-worker",
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
	logger.Info("tdlib worker is ready", "adapter_mode", tdlib.AdapterMode())
}
