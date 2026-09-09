package link

import (
	"fmt"
	"strconv"
	"strings"
)

const tdlibServerMessageIDShift = 20

// PublicMessageID converts TDLib's internal server message identifier to the
// message number used in t.me links. Small identifiers are kept as-is for
// compatibility with already-normalized records and test fixtures.
func PublicMessageID(messageID int64) (int64, bool) {
	if messageID <= 0 {
		return 0, false
	}
	if messageID < 1<<tdlibServerMessageIDShift {
		return messageID, true
	}
	if messageID&((1<<tdlibServerMessageIDShift)-1) != 0 {
		return 0, false
	}
	return messageID >> tdlibServerMessageIDShift, true
}

// MessageURL builds a public or private-channel Telegram message URL.
func MessageURL(telegramChatID, messageID int64, username, fallback string) string {
	publicMessageID, ok := PublicMessageID(messageID)
	if !ok {
		return strings.TrimSpace(fallback)
	}
	if username = strings.TrimPrefix(strings.TrimSpace(username), "@"); username != "" {
		return fmt.Sprintf("https://t.me/%s/%d", username, publicMessageID)
	}
	chatID := strconv.FormatInt(telegramChatID, 10)
	if strings.HasPrefix(chatID, "-100") {
		return fmt.Sprintf("https://t.me/c/%s/%d", strings.TrimPrefix(chatID, "-100"), publicMessageID)
	}
	return strings.TrimSpace(fallback)
}
