package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Service          string
	Format           string
	Level            string
	Dir              string
	Retention        time.Duration
	RotationInterval time.Duration
	Now              func() time.Time
}

type Runtime struct {
	Logger *slog.Logger
	close  func() error
}

func New(cfg Config) (*Runtime, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Retention <= 0 {
		cfg.Retention = 24 * time.Hour
	}
	if cfg.RotationInterval <= 0 {
		cfg.RotationInterval = time.Hour
	}
	level := parseLevel(cfg.Level)
	var writer io.Writer = os.Stdout
	var closer func() error
	if strings.TrimSpace(cfg.Dir) != "" {
		fileWriter, err := newRotatingWriter(rotatingConfig{
			service:          cfg.Service,
			dir:              cfg.Dir,
			retention:        cfg.Retention,
			rotationInterval: cfg.RotationInterval,
			now:              cfg.Now,
		})
		if err != nil {
			return nil, err
		}
		writer = io.MultiWriter(os.Stdout, fileWriter)
		closer = fileWriter.Close
	} else {
		closer = func() error { return nil }
	}

	options := handlerOptions(level)
	var handler slog.Handler
	if strings.EqualFold(cfg.Format, "text") {
		handler = slog.NewTextHandler(writer, options)
	} else {
		handler = slog.NewJSONHandler(writer, options)
	}
	logger := slog.New(handler)
	if cfg.Service != "" {
		logger = logger.With("service", cfg.Service)
	}
	return &Runtime{Logger: logger, close: closer}, nil
}

func (r *Runtime) Close() error {
	if r == nil || r.close == nil {
		return nil
	}
	return r.close()
}

func Bootstrap(service string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, handlerOptions(slog.LevelInfo))).With("service", service)
}

func handlerOptions(level slog.Leveler) *slog.HandlerOptions {
	return &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key != slog.TimeKey {
				return attr
			}
			return slog.String(slog.TimeKey, attr.Value.Time().Format(time.RFC3339Nano))
		},
	}
}

func parseLevel(value string) slog.Leveler {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type rotatingConfig struct {
	service          string
	dir              string
	retention        time.Duration
	rotationInterval time.Duration
	now              func() time.Time
}

type rotatingWriter struct {
	mu            sync.Mutex
	cfg           rotatingConfig
	file          *os.File
	currentBucket time.Time
	cleanupCancel context.CancelFunc
	cleanupDone   chan struct{}
}

func newRotatingWriter(cfg rotatingConfig) (*rotatingWriter, error) {
	if cfg.service == "" {
		cfg.service = "app"
	}
	if err := os.MkdirAll(cfg.dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	writer := &rotatingWriter{cfg: cfg, cleanupDone: make(chan struct{})}
	if err := writer.rotateLocked(cfg.now()); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	writer.cleanupCancel = cancel
	go writer.cleanupLoop(ctx)
	return writer, nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := w.cfg.now()
	bucket := w.bucket(now)
	if w.file == nil || !bucket.Equal(w.currentBucket) {
		if err := w.rotateLocked(now); err != nil {
			return 0, err
		}
	}
	return w.file.Write(p)
}

func (w *rotatingWriter) Close() error {
	if w.cleanupCancel != nil {
		w.cleanupCancel()
		<-w.cleanupDone
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *rotatingWriter) rotateLocked(now time.Time) error {
	bucket := w.bucket(now)
	path := filepath.Join(w.cfg.dir, fmt.Sprintf("%s-%s.log", w.cfg.service, bucket.Format("2006-01-02-15")))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	var closeErr error
	if w.file != nil {
		closeErr = w.file.Close()
	}
	w.file = file
	w.currentBucket = bucket
	w.updateCurrentLink(path)
	return closeErr
}

func (w *rotatingWriter) updateCurrentLink(path string) {
	current := filepath.Join(w.cfg.dir, fmt.Sprintf("%s-current.log", w.cfg.service))
	_ = os.Remove(current)
	if err := os.Symlink(filepath.Base(path), current); err == nil {
		return
	}
	_ = os.WriteFile(current, nil, 0o644)
}

func (w *rotatingWriter) bucket(now time.Time) time.Time {
	return now.Truncate(w.cfg.rotationInterval)
}

func (w *rotatingWriter) cleanupLoop(ctx context.Context) {
	defer close(w.cleanupDone)
	w.cleanup()
	ticker := time.NewTicker(w.cfg.rotationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.cleanup()
		}
	}
}

func (w *rotatingWriter) cleanup() {
	cutoff := w.cfg.now().Add(-w.cfg.retention)
	entries, err := os.ReadDir(w.cfg.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, w.cfg.service+"-") || !strings.HasSuffix(name, ".log") || strings.HasSuffix(name, "-current.log") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(w.cfg.dir, name))
	}
}
