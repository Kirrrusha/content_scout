//go:build unix

package logging

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"time"
)

type StderrPrefixer struct {
	once     sync.Once
	original *os.File
	reader   *os.File
	done     chan struct{}
	err      error
}

func StartStderrTimestampPrefixer(now func() time.Time) (*StderrPrefixer, error) {
	if now == nil {
		now = time.Now
	}

	originalFD, err := syscall.Dup(int(os.Stderr.Fd()))
	if err != nil {
		return nil, fmt.Errorf("duplicate stderr: %w", err)
	}
	original := os.NewFile(uintptr(originalFD), "original-stderr")

	reader, writer, err := os.Pipe()
	if err != nil {
		_ = original.Close()
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}
	if err := syscall.Dup2(int(writer.Fd()), int(os.Stderr.Fd())); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		_ = original.Close()
		return nil, fmt.Errorf("redirect stderr: %w", err)
	}
	_ = writer.Close()

	prefixer := &StderrPrefixer{
		original: original,
		reader:   reader,
		done:     make(chan struct{}),
	}
	go prefixer.copy(now)
	return prefixer, nil
}

func (p *StderrPrefixer) Close() error {
	if p == nil {
		return nil
	}
	p.once.Do(func() {
		if err := syscall.Dup2(int(p.original.Fd()), int(os.Stderr.Fd())); err != nil {
			p.err = fmt.Errorf("restore stderr: %w", err)
		}
		<-p.done
		if err := p.reader.Close(); p.err == nil && err != nil {
			p.err = err
		}
		if err := p.original.Close(); p.err == nil && err != nil {
			p.err = err
		}
	})
	return p.err
}

func (p *StderrPrefixer) copy(now func() time.Time) {
	defer close(p.done)

	reader := bufio.NewReader(p.reader)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			_, _ = fmt.Fprintf(p.original, "%s %s", now().Format(time.RFC3339Nano), line)
		}
		if err != nil {
			if err != io.EOF {
				_, _ = fmt.Fprintf(p.original, "%s stderr prefixer stopped: %v\n", now().Format(time.RFC3339Nano), err)
			}
			return
		}
	}
}
