package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/kirilllebedenko/content_scout/internal/config"
	"github.com/kirilllebedenko/content_scout/internal/storage/postgres"
)

func main() {
	var direction string
	var dir string
	flag.StringVar(&direction, "direction", "up", "migration direction: up or down")
	flag.StringVar(&dir, "dir", "migrations", "migrations directory")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	db, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect database failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

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
