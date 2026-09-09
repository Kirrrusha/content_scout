package link

import "testing"

func TestMessageURLConvertsTDLibServerMessageID(t *testing.T) {
	const tdlibMessageID = int64(12345 << tdlibServerMessageIDShift)

	if got := MessageURL(-100987, tdlibMessageID, "@backend", ""); got != "https://t.me/backend/12345" {
		t.Fatalf("MessageURL() = %q", got)
	}
	if got := MessageURL(-100987, tdlibMessageID, "", ""); got != "https://t.me/c/987/12345" {
		t.Fatalf("private MessageURL() = %q", got)
	}
}

func TestMessageURLKeepsAlreadyNormalizedMessageID(t *testing.T) {
	if got := MessageURL(-100987, 12345, "backend", ""); got != "https://t.me/backend/12345" {
		t.Fatalf("MessageURL() = %q", got)
	}
}

func TestMessageURLUsesFallbackForNonServerTDLibMessageID(t *testing.T) {
	if got := MessageURL(-100987, (12345<<tdlibServerMessageIDShift)+1, "backend", " https://example.com/source "); got != "https://example.com/source" {
		t.Fatalf("MessageURL() = %q", got)
	}
}
