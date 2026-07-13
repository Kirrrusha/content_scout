package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	unsetEnv(t, "APP_ENV")
	unsetEnv(t, "HTTP_ADDR")
	unsetEnv(t, "DATABASE_URL")
	unsetEnv(t, "TELEGRAM_OWNER_ID")
	unsetEnv(t, "TELEGRAM_API_ID")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AppEnv != "development" {
		t.Fatalf("AppEnv = %q, want development", cfg.AppEnv)
	}
	if cfg.TelegramOwnerID != 0 {
		t.Fatalf("TelegramOwnerID = %d, want 0", cfg.TelegramOwnerID)
	}
}

func TestLoadParsesNumbers(t *testing.T) {
	t.Setenv("TELEGRAM_OWNER_ID", "12345")
	t.Setenv("TELEGRAM_API_ID", "42")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.TelegramOwnerID != 12345 {
		t.Fatalf("TelegramOwnerID = %d, want 12345", cfg.TelegramOwnerID)
	}
	if cfg.TelegramAPIID != 42 {
		t.Fatalf("TelegramAPIID = %d, want 42", cfg.TelegramAPIID)
	}
}

func TestLoadReadsServiceToken(t *testing.T) {
	t.Setenv("SERVICE_TOKEN", "secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ServiceToken != "secret" {
		t.Fatalf("ServiceToken = %q, want secret", cfg.ServiceToken)
	}
}

func TestLoadParsesLoggingConfig(t *testing.T) {
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_DIR", "/tmp/content-scout-logs")
	t.Setenv("LOG_RETENTION", "12h")
	t.Setenv("LOG_ROTATION_INTERVAL", "30m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LogFormat != "text" || cfg.LogLevel != "debug" || cfg.LogDir != "/tmp/content-scout-logs" {
		t.Fatalf("logging config = %+v", cfg)
	}
	if cfg.LogRetention != 12*time.Hour || cfg.LogRotation != 30*time.Minute {
		t.Fatalf("durations retention=%s rotation=%s", cfg.LogRetention, cfg.LogRotation)
	}
}

func TestLoadRejectsInvalidLoggingDuration(t *testing.T) {
	t.Setenv("LOG_RETENTION", "not-a-duration")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoadParsesObsidianRESTConfig(t *testing.T) {
	t.Setenv("OBSIDIAN_REST_URL", "https://127.0.0.1:27124")
	t.Setenv("OBSIDIAN_API_KEY", "secret")
	t.Setenv("OBSIDIAN_INSECURE_SKIP_VERIFY", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ObsidianRESTURL != "https://127.0.0.1:27124" || cfg.ObsidianAPIKey != "secret" || !cfg.ObsidianInsecure {
		t.Fatalf("obsidian config = %+v", cfg)
	}
}

func TestLoadRejectsInvalidNumbers(t *testing.T) {
	t.Setenv("TELEGRAM_OWNER_ID", "not-a-number")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	previous, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("Unsetenv(%q) error = %v", key, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, previous)
			return
		}
		_ = os.Unsetenv(key)
	})
}
