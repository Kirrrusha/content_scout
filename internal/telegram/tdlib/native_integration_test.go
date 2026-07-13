//go:build tdlib && cgo && integration

package tdlib

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestNativeClientAuthorizationStateIntegration(t *testing.T) {
	apiID, err := strconv.Atoi(os.Getenv("TELEGRAM_API_ID"))
	if err != nil || apiID == 0 {
		t.Skip("TELEGRAM_API_ID is not configured")
	}
	apiHash := os.Getenv("TELEGRAM_API_HASH")
	if apiHash == "" {
		t.Skip("TELEGRAM_API_HASH is not configured")
	}
	sessionDir := os.Getenv("TDLIB_INTEGRATION_SESSION_DIR")
	if sessionDir == "" {
		sessionDir = filepath.Join(t.TempDir(), "tdlib")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := NewNativeClient(ClientConfig{APIID: apiID, APIHash: apiHash}, sessionDir)
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		_ = client.Stop(stopCtx)
	}()

	state, err := client.AuthorizationState(ctx)
	if err != nil {
		t.Fatalf("AuthorizationState() error = %v", err)
	}
	switch state {
	case AuthorizationStateWaitPhoneNumber, AuthorizationStateWaitCode, AuthorizationStateWaitPassword, AuthorizationStateReady, AuthorizationStateClosed:
	default:
		t.Fatalf("AuthorizationState() = %s, want known TDLib state", state)
	}
}
