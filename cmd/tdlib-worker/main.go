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
	defer logRuntime.Close()
	logger = logRuntime.Logger
	logger.Info("tdlib worker is ready", "adapter_mode", tdlib.AdapterMode())
}
