package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv           string
	HTTPAddr         string
	DatabaseURL      string
	ServiceToken     string
	LogFormat        string
	LogLevel         string
	LogDir           string
	LogRetention     time.Duration
	LogRotation      time.Duration
	TelegramBotToken string
	TelegramOwnerID  int64
	TelegramAPIID    int
	TelegramAPIHash  string
	TDLibDatabaseDir string
	LLMProvider      string
	LLMBaseURL       string
	LLMAPIKey        string
	LLMModel         string
	EncryptionKey    string
	ExportDir        string
	ObsidianRESTURL  string
	ObsidianAPIKey   string
	ObsidianInsecure bool
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:           getEnv("APP_ENV", "development"),
		HTTPAddr:         getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:      getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/telegram_summary?sslmode=disable"),
		ServiceToken:     os.Getenv("SERVICE_TOKEN"),
		LogFormat:        getEnv("LOG_FORMAT", "json"),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		LogDir:           getEnv("LOG_DIR", "./data/logs"),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramAPIHash:  os.Getenv("TELEGRAM_API_HASH"),
		TDLibDatabaseDir: getEnv("TDLIB_DATABASE_DIR", "./data/tdlib"),
		LLMProvider:      getEnv("LLM_PROVIDER", "openai"),
		LLMBaseURL:       os.Getenv("LLM_BASE_URL"),
		LLMAPIKey:        os.Getenv("LLM_API_KEY"),
		LLMModel:         os.Getenv("LLM_MODEL"),
		EncryptionKey:    os.Getenv("ENCRYPTION_KEY"),
		ExportDir:        getEnv("EXPORT_DIR", "./data/exports"),
		ObsidianRESTURL:  os.Getenv("OBSIDIAN_REST_URL"),
		ObsidianAPIKey:   os.Getenv("OBSIDIAN_API_KEY"),
	}

	var err error
	cfg.TelegramOwnerID, err = parseInt64Env("TELEGRAM_OWNER_ID", 0)
	if err != nil {
		return Config{}, err
	}
	cfg.ObsidianInsecure, err = parseBoolEnv("OBSIDIAN_INSECURE_SKIP_VERIFY", false)
	if err != nil {
		return Config{}, err
	}
	cfg.TelegramAPIID, err = parseIntEnv("TELEGRAM_API_ID", 0)
	if err != nil {
		return Config{}, err
	}
	cfg.LogRetention, err = parseDurationEnv("LOG_RETENTION", 24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	cfg.LogRotation, err = parseDurationEnv("LOG_ROTATION_INTERVAL", time.Hour)
	if err != nil {
		return Config{}, err
	}

	if cfg.HTTPAddr == "" {
		return Config{}, errors.New("HTTP_ADDR must not be empty")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL must not be empty")
	}
	return cfg, nil
}

func parseDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func parseBoolEnv(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func parseIntEnv(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func parseInt64Env(key string, fallback int64) (int64, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}
