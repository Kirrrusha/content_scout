package tdlib

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

func mapAuthorizationState(rawType string) AuthorizationState {
	switch rawType {
	case "authorizationStateWaitPhoneNumber":
		return AuthorizationStateWaitPhoneNumber
	case "authorizationStateWaitCode":
		return AuthorizationStateWaitCode
	case "authorizationStateWaitPassword":
		return AuthorizationStateWaitPassword
	case "authorizationStateReady":
		return AuthorizationStateReady
	case "authorizationStateClosed", "authorizationStateClosing", "authorizationStateLoggingOut":
		return AuthorizationStateClosed
	default:
		return AuthorizationStateUnknown
	}
}

func mapChat(raw map[string]any, archived bool) domain.TelegramChat {
	chatType := mapChatType(nestedMap(raw, "type"))
	lastMessageID := int64(0)
	if lastMessage, ok := raw["last_message"].(map[string]any); ok {
		lastMessageID = int64Field(lastMessage, "id")
	}
	return domain.TelegramChat{
		TelegramChatID: int64Field(raw, "id"),
		Title:          stringField(raw, "title"),
		Type:           chatType,
		IsArchived:     archived,
		IsMuted:        intField(nestedMap(raw, "notification_settings"), "mute_for") > 0,
		UnreadCount:    intField(raw, "unread_count"),
		LastMessageID:  lastMessageID,
		UpdatedAt:      time.Now().UTC(),
	}
}

func mapChatFolders(raw map[string]any) []domain.TelegramFolder {
	rawFolders, _ := raw["chat_folders"].([]any)
	folders := make([]domain.TelegramFolder, 0, len(rawFolders))
	for _, rawFolder := range rawFolders {
		folder, ok := rawFolder.(map[string]any)
		if !ok {
			continue
		}
		folders = append(folders, domain.TelegramFolder{
			TelegramID: int32(int64Field(folder, "id")),
			Name:       chatFolderName(nestedMap(folder, "name")),
		})
	}
	return folders
}

func chatFolderName(raw map[string]any) string {
	if text := formattedText(nestedMap(raw, "text")); text != "" {
		return text
	}
	return stringField(raw, "title")
}

func mapChatType(raw map[string]any) domain.ChatType {
	switch nestedTypeValue(raw) {
	case "chatTypePrivate":
		return domain.ChatTypePrivate
	case "chatTypeBasicGroup":
		return domain.ChatTypeGroup
	case "chatTypeSupergroup":
		if boolField(raw, "is_channel") {
			return domain.ChatTypeChannel
		}
		return domain.ChatTypeSupergroup
	default:
		return domain.ChatTypeUnknown
	}
}

func mapMessage(raw map[string]any) domain.TelegramMessage {
	content := nestedMap(raw, "content")
	text, caption, url, mediaType := messageContent(content)
	editDate := unixTimePtr(int64Field(raw, "edit_date"))
	replyToID := replyMessageID(raw)
	return domain.TelegramMessage{
		ChatID:     int64Field(raw, "chat_id"),
		MessageID:  int64Field(raw, "id"),
		Date:       unixTime(int64Field(raw, "date")),
		EditDate:   editDate,
		SenderID:   senderID(nestedMap(raw, "sender_id")),
		SenderName: senderName(nestedMap(raw, "sender_id")),
		Text:       text,
		Caption:    caption,
		URL:        url,
		ReplyToID:  replyToID,
		Forwarded:  raw["forward_info"] != nil,
		HasMedia:   mediaType != "",
		MediaType:  mediaType,
	}
}

func messageContent(content map[string]any) (text string, caption string, url string, mediaType string) {
	switch nestedTypeValue(content) {
	case "messageText":
		text = formattedText(nestedMap(content, "text"))
	case "messagePhoto":
		caption = formattedText(nestedMap(content, "caption"))
		mediaType = "photo"
	case "messageVideo":
		caption = formattedText(nestedMap(content, "caption"))
		mediaType = "video"
	case "messageDocument":
		caption = formattedText(nestedMap(content, "caption"))
		mediaType = "document"
	case "messageAnimation":
		caption = formattedText(nestedMap(content, "caption"))
		mediaType = "animation"
	case "messageAudio":
		caption = formattedText(nestedMap(content, "caption"))
		mediaType = "audio"
	case "messageVoiceNote":
		caption = formattedText(nestedMap(content, "caption"))
		mediaType = "voice"
	}
	if webPage, ok := nestedMap(content, "web_page")["url"].(string); ok {
		url = webPage
	}
	return text, caption, url, mediaType
}

func formattedText(raw map[string]any) string {
	return stringField(raw, "text")
}

func senderID(raw map[string]any) int64 {
	if id := int64Field(raw, "user_id"); id != 0 {
		return id
	}
	return int64Field(raw, "chat_id")
}

func senderName(raw map[string]any) string {
	if userID := int64Field(raw, "user_id"); userID != 0 {
		return fmt.Sprintf("user:%d", userID)
	}
	if chatID := int64Field(raw, "chat_id"); chatID != 0 {
		return fmt.Sprintf("chat:%d", chatID)
	}
	return ""
}

func replyMessageID(raw map[string]any) *int64 {
	replyTo := nestedMap(raw, "reply_to")
	id := int64Field(replyTo, "message_id")
	if id == 0 {
		return nil
	}
	return &id
}

func unixTime(timestamp int64) time.Time {
	if timestamp <= 0 {
		return time.Time{}
	}
	return time.Unix(timestamp, 0).UTC()
}

func unixTimePtr(timestamp int64) *time.Time {
	if timestamp <= 0 {
		return nil
	}
	value := unixTime(timestamp)
	return &value
}

// Used from client.go, which is behind the tdlib build tag the linter runs without.
//
//nolint:unused
func nestedType(raw map[string]any, key string) string {
	return nestedTypeValue(nestedMap(raw, key))
}

func nestedTypeValue(raw map[string]any) string {
	return stringField(raw, "@type")
}

func nestedMap(raw map[string]any, key string) map[string]any {
	value, ok := raw[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return value
}

func stringField(raw map[string]any, key string) string {
	value, _ := raw[key].(string)
	return value
}

func intField(raw map[string]any, key string) int {
	return int(int64Field(raw, key))
}

func int64Field(raw map[string]any, key string) int64 {
	switch value := raw[key].(type) {
	case float64:
		return int64(value)
	case int:
		return int64(value)
	case int64:
		return value
	case json.Number:
		parsed, err := value.Int64()
		if err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}

func boolField(raw map[string]any, key string) bool {
	value, _ := raw[key].(bool)
	return value
}
