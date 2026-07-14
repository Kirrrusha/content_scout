//go:build unix

package logging

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStderrTimestampPrefixerAddsTime(t *testing.T) {
	realStderr, err := syscall.Dup(int(os.Stderr.Fd()))
	if err != nil {
		t.Fatalf("dup stderr: %v", err)
	}
	defer syscall.Close(realStderr)
	defer func() {
		if err := syscall.Dup2(realStderr, int(os.Stderr.Fd())); err != nil {
			t.Fatalf("restore real stderr: %v", err)
		}
	}()

	tmp, err := os.CreateTemp(t.TempDir(), "stderr-*.log")
	if err != nil {
		t.Fatalf("create temp stderr: %v", err)
	}
	defer tmp.Close()
	if err := syscall.Dup2(int(tmp.Fd()), int(os.Stderr.Fd())); err != nil {
		t.Fatalf("redirect stderr to temp file: %v", err)
	}

	now := func() time.Time {
		return time.Date(2026, 7, 14, 12, 30, 45, 123456789, time.UTC)
	}
	prefixer, err := StartStderrTimestampPrefixer(now)
	if err != nil {
		t.Fatalf("StartStderrTimestampPrefixer() error = %v", err)
	}
	if _, err := syscall.Write(int(os.Stderr.Fd()), []byte("[ 3][t 0][1784014772.757076025] Create client 1\n")); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := prefixer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := tmp.Seek(0, 0); err != nil {
		t.Fatalf("seek temp stderr: %v", err)
	}
	content, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatalf("read temp stderr: %v", err)
	}
	output := string(content)
	if !strings.Contains(output, "2026-07-14T12:30:45.123456789Z [ 3][t 0][1784014772.757076025] Create client 1\n") {
		t.Fatalf("output = %q", output)
	}
}
