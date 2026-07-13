//go:build tdlib && cgo

package tdlib

/*
#cgo LDFLAGS: -ltdjson
#include <stdlib.h>

extern void *td_json_client_create();
extern void td_json_client_send(void *client, const char *request);
extern const char *td_json_client_receive(void *client, double timeout);
extern const char *td_json_client_execute(void *client, const char *request);
extern void td_json_client_destroy(void *client);
*/
import "C"

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type NativeClient struct {
	cfg         ClientConfig
	sessionPath string
	handle      unsafe.Pointer
	mu          sync.Mutex
	extraSeq    atomic.Uint64
	authState   AuthorizationState
}

func NewNativeClient(cfg ClientConfig, sessionPath string) *NativeClient {
	return &NativeClient{
		cfg:         cfg,
		sessionPath: sessionPath,
		authState:   AuthorizationStateUnknown,
	}
}

func (c *NativeClient) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureHandle(); err != nil {
		return err
	}
	return c.advanceAuthorization(ctx)
}

func (c *NativeClient) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.handle == nil {
		return nil
	}
	_, err := c.sendAndWait(ctx, map[string]any{"@type": "close"})
	c.authState = AuthorizationStateClosed
	C.td_json_client_destroy(c.handle)
	c.handle = nil
	return err
}

func (c *NativeClient) AuthorizationState(ctx context.Context) (AuthorizationState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureHandle(); err != nil {
		return AuthorizationStateError, err
	}
	if c.authState == AuthorizationStateUnknown {
		if err := c.advanceAuthorization(ctx); err != nil {
			return AuthorizationStateError, err
		}
	}
	return c.authState, nil
}

func (c *NativeClient) SubmitPhoneNumber(ctx context.Context, phone string) error {
	return c.submitAuth(ctx, map[string]any{
		"@type":        "setAuthenticationPhoneNumber",
		"phone_number": phone,
		"settings": map[string]any{
			"@type": "phoneNumberAuthenticationSettings",
		},
	})
}

func (c *NativeClient) SubmitCode(ctx context.Context, code string) error {
	return c.submitAuth(ctx, map[string]any{
		"@type": "checkAuthenticationCode",
		"code":  code,
	})
}

func (c *NativeClient) SubmitPassword(ctx context.Context, password string) error {
	return c.submitAuth(ctx, map[string]any{
		"@type":    "checkAuthenticationPassword",
		"password": password,
	})
}

func (c *NativeClient) ListFolders(ctx context.Context) ([]domain.TelegramFolder, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	response, err := c.sendAndWait(ctx, map[string]any{"@type": "getChatFolders"})
	if err != nil {
		return nil, err
	}
	rawFolders, _ := response["chat_folders"].([]any)
	folders := make([]domain.TelegramFolder, 0, len(rawFolders))
	for _, rawFolder := range rawFolders {
		folder, ok := rawFolder.(map[string]any)
		if !ok {
			continue
		}
		folders = append(folders, domain.TelegramFolder{
			TelegramID: int32(int64Field(folder, "id")),
			Name:       stringField(folder, "title"),
		})
	}
	return folders, nil
}

func (c *NativeClient) ListChats(ctx context.Context, list ChatList) ([]domain.TelegramChat, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	response, err := c.sendAndWait(ctx, map[string]any{
		"@type":     "getChats",
		"chat_list": tdlibChatList(list),
		"limit":     1000,
	})
	if err != nil {
		return nil, err
	}
	rawIDs, _ := response["chat_ids"].([]any)
	chats := make([]domain.TelegramChat, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		chatID := int64FromAny(rawID)
		if chatID == 0 {
			continue
		}
		chat, err := c.getChat(ctx, chatID, list == ChatListArchive)
		if err != nil {
			return nil, err
		}
		chats = append(chats, chat)
	}
	return chats, nil
}

func (c *NativeClient) GetChatHistory(ctx context.Context, chatID int64, _ int64, limit int) ([]domain.TelegramMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if limit <= 0 {
		limit = 100
	}
	response, err := c.sendAndWait(ctx, map[string]any{
		"@type":           "getChatHistory",
		"chat_id":         chatID,
		"from_message_id": 0,
		"offset":          0,
		"limit":           limit,
		"only_local":      false,
	})
	if err != nil {
		return nil, err
	}
	rawMessages, _ := response["messages"].([]any)
	messages := make([]domain.TelegramMessage, 0, len(rawMessages))
	for _, rawMessage := range rawMessages {
		message, ok := rawMessage.(map[string]any)
		if !ok {
			continue
		}
		messages = append(messages, mapMessage(message))
	}
	return messages, nil
}

func (c *NativeClient) submitAuth(ctx context.Context, request map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureHandle(); err != nil {
		return err
	}
	if _, err := c.sendAndWait(ctx, request); err != nil {
		return err
	}
	return c.advanceAuthorization(ctx)
}

func (c *NativeClient) ensureHandle() error {
	if c.handle != nil {
		return nil
	}
	if c.cfg.APIID == 0 {
		return errors.New("TELEGRAM_API_ID is not configured")
	}
	if c.cfg.APIHash == "" {
		return errors.New("TELEGRAM_API_HASH is not configured")
	}
	if c.sessionPath == "" {
		return errors.New("TDLib session path is not configured")
	}
	if err := os.MkdirAll(c.sessionPath, 0o700); err != nil {
		return fmt.Errorf("create tdlib session directory: %w", err)
	}
	c.handle = C.td_json_client_create()
	if c.handle == nil {
		return errors.New("create tdlib json client")
	}
	c.execute(map[string]any{
		"@type":               "setLogVerbosityLevel",
		"new_verbosity_level": 2,
	})
	return nil
}

func (c *NativeClient) advanceAuthorization(ctx context.Context) error {
	for {
		response, err := c.sendAndWait(ctx, map[string]any{"@type": "getAuthorizationState"})
		if err != nil {
			return err
		}
		rawState := rawAuthorizationState(response)
		c.updateAuthorizationFrom(response)
		switch rawState {
		case "authorizationStateWaitTdlibParameters":
			if _, err := c.sendAndWait(ctx, c.tdlibParameters()); err != nil {
				return err
			}
		case "authorizationStateWaitEncryptionKey":
			if _, err := c.sendAndWait(ctx, map[string]any{
				"@type":          "checkDatabaseEncryptionKey",
				"encryption_key": "",
			}); err != nil {
				return err
			}
		case "authorizationStateReady", "authorizationStateWaitPhoneNumber", "authorizationStateWaitCode", "authorizationStateWaitPassword", "authorizationStateClosed", "authorizationStateClosing", "authorizationStateLoggingOut":
			return nil
		default:
			update, err := c.receive(ctx)
			if err != nil {
				return err
			}
			c.updateAuthorizationFrom(update)
		}
	}
}

func (c *NativeClient) getAuthorizationState(ctx context.Context) (AuthorizationState, error) {
	response, err := c.sendAndWait(ctx, map[string]any{"@type": "getAuthorizationState"})
	if err != nil {
		return AuthorizationStateError, err
	}
	c.updateAuthorizationFrom(response)
	return c.authState, nil
}

func (c *NativeClient) tdlibParameters() map[string]any {
	return map[string]any{
		"@type":                    "setTdlibParameters",
		"use_test_dc":              false,
		"database_directory":       c.sessionPath,
		"files_directory":          filepath.Join(c.sessionPath, "files"),
		"use_file_database":        true,
		"use_chat_info_database":   true,
		"use_message_database":     true,
		"use_secret_chats":         false,
		"api_id":                   c.cfg.APIID,
		"api_hash":                 c.cfg.APIHash,
		"system_language_code":     "en",
		"device_model":             "content-scout",
		"system_version":           "linux",
		"application_version":      "0.1.0",
		"enable_storage_optimizer": true,
		"ignore_file_names":        false,
	}
}

func (c *NativeClient) getChat(ctx context.Context, chatID int64, archived bool) (domain.TelegramChat, error) {
	response, err := c.sendAndWait(ctx, map[string]any{
		"@type":   "getChat",
		"chat_id": chatID,
	})
	if err != nil {
		return domain.TelegramChat{}, err
	}
	return mapChat(response, archived), nil
}

func (c *NativeClient) sendAndWait(ctx context.Context, request map[string]any) (map[string]any, error) {
	if err := c.ensureHandle(); err != nil {
		return nil, err
	}
	extra := fmt.Sprintf("content-scout:%d", c.extraSeq.Add(1))
	request["@extra"] = extra
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	cString := C.CString(string(payload))
	defer C.free(unsafe.Pointer(cString))
	C.td_json_client_send(c.handle, cString)

	for {
		response, err := c.receive(ctx)
		if err != nil {
			return nil, err
		}
		c.updateAuthorizationFrom(response)
		if stringField(response, "@extra") != extra {
			continue
		}
		if stringField(response, "@type") == "error" {
			return nil, fmt.Errorf("tdlib error %d: %s", intField(response, "code"), stringField(response, "message"))
		}
		return response, nil
	}
}

func (c *NativeClient) receive(ctx context.Context) (map[string]any, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		raw := C.td_json_client_receive(c.handle, 0.5)
		if raw == nil {
			continue
		}
		decoder := json.NewDecoder(bytes.NewBufferString(C.GoString(raw)))
		decoder.UseNumber()
		var response map[string]any
		if err := decoder.Decode(&response); err != nil {
			return nil, fmt.Errorf("decode tdlib response: %w", err)
		}
		return response, nil
	}
}

func (c *NativeClient) execute(request map[string]any) {
	payload, err := json.Marshal(request)
	if err != nil {
		return
	}
	cString := C.CString(string(payload))
	defer C.free(unsafe.Pointer(cString))
	C.td_json_client_execute(c.handle, cString)
}

func (c *NativeClient) updateAuthorizationFrom(response map[string]any) {
	rawType := rawAuthorizationState(response)
	state := mapAuthorizationState(rawType)
	if state != AuthorizationStateUnknown {
		c.authState = state
	}
}

func rawAuthorizationState(response map[string]any) string {
	rawType := nestedTypeValue(response)
	if rawType == "updateAuthorizationState" {
		return nestedType(response, "authorization_state")
	}
	return rawType
}

func tdlibChatList(list ChatList) map[string]any {
	if list == ChatListArchive {
		return map[string]any{"@type": "chatListArchive"}
	}
	return map[string]any{"@type": "chatListMain"}
}

func int64FromAny(value any) int64 {
	return int64Field(map[string]any{"value": value}, "value")
}

var _ TelegramClient = (*NativeClient)(nil)
