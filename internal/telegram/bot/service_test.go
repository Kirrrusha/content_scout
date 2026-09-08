package bot

import (
	"strings"
	"testing"
)

func TestRedactTelegramBotToken(t *testing.T) {
	token := "123456:secret-token"
	message := `Post "https://api.telegram.org/bot123456:secret-token/getMe": connection failed`

	got := redactTelegramBotToken(message, token)
	if strings.Contains(got, token) {
		t.Fatalf("redacted message still contains token: %q", got)
	}
	if !strings.Contains(got, "bot<redacted>/getMe") {
		t.Fatalf("redacted message = %q", got)
	}
}
