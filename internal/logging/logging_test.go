package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseLevel(t *testing.T) {
	if parseLevel("debug").Level() != slog.LevelDebug {
		t.Fatal("debug level was not parsed")
	}
	if parseLevel("error").Level() != slog.LevelError {
		t.Fatal("error level was not parsed")
	}
}

func TestRotatingWriterWritesHourlyFileAndCurrentLink(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 13, 18, 20, 0, 0, time.UTC)
	writer, err := newRotatingWriter(rotatingConfig{
		service:          "api",
		dir:              dir,
		retention:        24 * time.Hour,
		rotationInterval: time.Hour,
		now:              func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("newRotatingWriter() error = %v", err)
	}
	defer func() { _ = writer.Close() }()

	if _, err := writer.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	logPath := filepath.Join(dir, "api-2026-07-13-18.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", logPath, err)
	}
	if string(content) != "hello\n" {
		t.Fatalf("content = %q", content)
	}

	current := filepath.Join(dir, "api-current.log")
	target, err := os.Readlink(current)
	if err != nil {
		t.Fatalf("Readlink(%q) error = %v", current, err)
	}
	if target != "api-2026-07-13-18.log" {
		t.Fatalf("current target = %q", target)
	}
}

func TestRotatingWriterCleanupRemovesExpiredLogs(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "api-2026-07-12-18.log")
	newPath := filepath.Join(dir, "api-2026-07-13-18.log")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old log: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o644); err != nil {
		t.Fatalf("write new log: %v", err)
	}
	oldTime := time.Date(2026, 7, 12, 17, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 7, 13, 18, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes old log: %v", err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatalf("chtimes new log: %v", err)
	}

	writer := &rotatingWriter{cfg: rotatingConfig{
		service:   "api",
		dir:       dir,
		retention: 24 * time.Hour,
		now:       func() time.Time { return time.Date(2026, 7, 13, 18, 30, 0, 0, time.UTC) },
	}}
	writer.cleanup()

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old log still exists or unexpected error: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new log was removed: %v", err)
	}
}

func TestNewJSONLoggerIncludesService(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, handlerOptions(parseLevel("info")))
	logger := slog.New(handler).With("service", "api")
	logger.Info("started")

	output := buf.String()
	if !strings.Contains(output, `"time":"`) || !strings.Contains(output, `"service":"api"`) || !strings.Contains(output, `"msg":"started"`) {
		t.Fatalf("output = %s", output)
	}
}
