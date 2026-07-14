package main

import (
	"context"
	"flag"
	"os"

	"github.com/kirilllebedenko/content_scout/internal/config"
	"github.com/kirilllebedenko/content_scout/internal/logging"
	"github.com/kirilllebedenko/content_scout/internal/storage/postgres"
)

func main() {
	var direction string
	var dir string
	flag.StringVar(&direction, "direction", "up", "migration direction: up or down")
	flag.StringVar(&dir, "dir", "migrations", "migrations directory")
	flag.Parse()

	logger := logging.Bootstrap("migrate")
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}
	logRuntime, err := logging.New(logging.Config{
		Service:          "migrate",
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

	ctx := context.Background()
	db, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect database failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	if direction != string(postgres.MigrationUp) && direction != string(postgres.MigrationDown) {
		logger.Error("invalid migration direction", "direction", direction)
		os.Exit(1)
	}
	if err := postgres.RunMigrations(ctx, db, dir, postgres.MigrationDirection(direction)); err != nil {
		logger.Error("run migrations failed", "error", err)
		os.Exit(1)
	}
	logger.Info("migrations complete", "direction", direction)
}
