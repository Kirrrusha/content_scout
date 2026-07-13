package main

import (
	"log/slog"
	"os"

	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("tdlib worker is ready", "adapter_mode", tdlib.AdapterMode())
}
