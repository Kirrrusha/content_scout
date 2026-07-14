//go:build !unix

package logging

import "time"

type StderrPrefixer struct{}

func StartStderrTimestampPrefixer(func() time.Time) (*StderrPrefixer, error) {
	return &StderrPrefixer{}, nil
}

func (*StderrPrefixer) Close() error {
	return nil
}
