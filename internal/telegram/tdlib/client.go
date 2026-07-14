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
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type NativeClient struct {
	cfg          ClientConfig
	sessionPath  string
	handle       unsafe.Pointer
	mu           sync.Mutex
	extraSeq     atomic.Uint64
	authState    AuthorizationState
	rawAuthState string
	folders      []domain.TelegramFolder
	haveFolders  bool
	proxySent    bool
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
	c.rawAuthState = "authorizationStateClosed"
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

	if c.haveFolders {
		return cloneFolders(c.folders), nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := c.waitChatFolders(waitCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	return cloneFolders(c.folders), nil
}

func (c *NativeClient) ListChats(ctx context.Context, list ChatList) ([]domain.TelegramChat, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.listChatsLocked(ctx, tdlibChatList(list), list == ChatListArchive)
}

func (c *NativeClient) ListFolderChats(ctx context.Context, folderID int32) ([]domain.TelegramChat, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.listChatsLocked(ctx, tdlibFolderChatList(folderID), false)
}

func (c *NativeClient) listChatsLocked(ctx context.Context, chatList map[string]any, archived bool) ([]domain.TelegramChat, error) {
	if err := c.loadChats(ctx, chatList); err != nil {
		return nil, err
	}
	response, err := c.sendAndWait(ctx, map[string]any{
		"@type":     "getChats",
		"chat_list": chatList,
		"limit":     10000,
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
		chat, err := c.getChat(ctx, chatID, archived)
		if err != nil {
			return nil, err
		}
		chats = append(chats, chat)
	}
	return chats, nil
}

func (c *NativeClient) loadChats(ctx context.Context, chatList map[string]any) error {
	for {
		_, err := c.sendAndWait(ctx, map[string]any{
			"@type":     "loadChats",
			"chat_list": chatList,
			"limit":     100,
		})
		if err == nil {
			continue
		}
		if isChatsFullyLoadedError(err) {
			return nil
		}
		return fmt.Errorf("load telegram chats: %w", err)
	}
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

func (c *NativeClient) MarkMessagesRead(ctx context.Context, chatID int64, messageIDs []int64) error {
	if len(messageIDs) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	rawIDs := make([]any, 0, len(messageIDs))
	for _, messageID := range messageIDs {
		if messageID > 0 {
			rawIDs = append(rawIDs, messageID)
		}
	}
	if len(rawIDs) == 0 {
		return nil
	}
	_, err := c.sendAndWait(ctx, map[string]any{
		"@type":       "viewMessages",
		"chat_id":     chatID,
		"message_ids": rawIDs,
		"force_read":  true,
	})
	return err
}

func (c *NativeClient) submitAuth(ctx context.Context, request map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureHandle(); err != nil {
		return err
	}
	if c.authState == AuthorizationStateUnknown {
		if err := c.advanceAuthorization(ctx); err != nil {
			return err
		}
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
	c.authState = AuthorizationStateUnknown
	c.rawAuthState = ""
	c.proxySent = false
	c.execute(map[string]any{
		"@type":               "setLogVerbosityLevel",
		"new_verbosity_level": 2,
	})
	return nil
}

func (c *NativeClient) advanceAuthorization(ctx context.Context) error {
	if !c.proxySent {
		if err := c.applyProxy(ctx); err != nil {
			return err
		}
		c.proxySent = true
	}
	for {
		rawState := c.rawAuthState
		if rawState == "" {
			var err error
			rawState, err = c.waitAuthorizationState(ctx)
			if err != nil {
				return err
			}
		}
		switch rawState {
		case "authorizationStateWaitTdlibParameters":
			if err := c.sendAuthTransition(ctx, c.tdlibParameters()); err != nil {
				return err
			}
		case "authorizationStateWaitEncryptionKey":
			if err := c.sendAuthTransition(ctx, map[string]any{
				"@type":          "checkDatabaseEncryptionKey",
				"encryption_key": "",
			}); err != nil {
				return err
			}
		case "authorizationStateReady", "authorizationStateWaitPhoneNumber", "authorizationStateWaitCode", "authorizationStateWaitPassword", "authorizationStateClosed", "authorizationStateClosing", "authorizationStateLoggingOut":
			return nil
		default:
			c.rawAuthState = ""
		}
	}
}

func (c *NativeClient) sendAuthTransition(ctx context.Context, request map[string]any) error {
	if err := c.send(ctx, request); err != nil {
		return err
	}
	c.authState = AuthorizationStateUnknown
	c.rawAuthState = ""
	return nil
}

func (c *NativeClient) waitAuthorizationState(ctx context.Context) (string, error) {
	for {
		response, err := c.receive(ctx)
		if err != nil {
			return "", err
		}
		if stringField(response, "@type") == "error" {
			return "", &TDLibError{Code: intField(response, "code"), Message: stringField(response, "message")}
		}
		rawState := rawAuthorizationState(response)
		c.updateAuthorizationFrom(response)
		if isRawAuthorizationState(rawState) {
			return rawState, nil
		}
	}
}

func (c *NativeClient) waitChatFolders(ctx context.Context) error {
	for !c.haveFolders {
		response, err := c.receive(ctx)
		if err != nil {
			return err
		}
		if stringField(response, "@type") == "error" {
			tdErr := &TDLibError{Code: intField(response, "code"), Message: stringField(response, "message")}
			if isUnexpectedSetTDLibParametersError(tdErr) {
				continue
			}
			return tdErr
		}
	}
	return nil
}

func isParametersNotSpecifiedError(err error) bool {
	var tdErr *TDLibError
	if !errors.As(err, &tdErr) {
		return false
	}
	return strings.Contains(tdErr.Message, "Parameters aren't specified")
}

func (c *NativeClient) send(ctx context.Context, request map[string]any) error {
	if err := c.ensureHandle(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	cString := C.CString(string(payload))
	defer C.free(unsafe.Pointer(cString))
	C.td_json_client_send(c.handle, cString)
	return nil
}

func (c *NativeClient) getAuthorizationState(ctx context.Context) (AuthorizationState, error) {
	response, err := c.sendAndWait(ctx, map[string]any{"@type": "getAuthorizationState"})
	if err != nil {
		return AuthorizationStateError, err
	}
	c.updateAuthorizationFrom(response)
	return c.authState, nil
}

// applyProxy configures TDLib to route its MTProto traffic through a SOCKS5
// proxy, needed when Telegram's servers are unreachable directly from the
// deployment region. A no-op if ProxyURL is not configured.
func (c *NativeClient) applyProxy(ctx context.Context) error {
	if c.cfg.ProxyURL == "" {
		return nil
	}
	parsed, err := url.Parse(c.cfg.ProxyURL)
	if err != nil {
		return fmt.Errorf("parse tdlib proxy url: %w", err)
	}
	if parsed.Scheme != "socks5" {
		return fmt.Errorf("tdlib proxy url must use socks5 scheme, got %q", parsed.Scheme)
	}
	host := parsed.Hostname()
	if host == "" {
		return errors.New("tdlib proxy url is missing a host")
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		return fmt.Errorf("parse tdlib proxy port: %w", err)
	}
	proxyType := map[string]any{"@type": "proxyTypeSocks5"}
	if username := parsed.User.Username(); username != "" {
		proxyType["username"] = username
		if password, ok := parsed.User.Password(); ok {
			proxyType["password"] = password
		}
	}
	_, err = c.sendAndWait(ctx, map[string]any{
		"@type":  "addProxy",
		"server": host,
		"port":   port,
		"enable": true,
		"type":   proxyType,
	})
	return err
}

func (c *NativeClient) tdlibParameters() map[string]any {
	return map[string]any{
		"@type":                   "setTdlibParameters",
		"use_test_dc":             false,
		"database_directory":      c.sessionPath,
		"files_directory":         filepath.Join(c.sessionPath, "files"),
		"database_encryption_key": "",
		"use_file_database":       true,
		"use_chat_info_database":  true,
		"use_message_database":    true,
		"use_secret_chats":        false,
		"api_id":                  c.cfg.APIID,
		"api_hash":                c.cfg.APIHash,
		"system_language_code":    "en",
		"device_model":            "content-scout",
		"system_version":          runtime.GOOS,
		"application_version":     "0.1.0",
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
			return nil, &TDLibError{Code: intField(response, "code"), Message: stringField(response, "message")}
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
		c.updateChatFoldersFrom(response)
		return response, nil
	}
}

func (c *NativeClient) execute(request map[string]any) {
	_, _ = c.executeAndDecode(request)
}

func (c *NativeClient) executeAndDecode(request map[string]any) (map[string]any, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	cString := C.CString(string(payload))
	defer C.free(unsafe.Pointer(cString))
	raw := C.td_json_client_execute(c.handle, cString)
	if raw == nil {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewBufferString(C.GoString(raw)))
	decoder.UseNumber()
	var response map[string]any
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("decode tdlib execute response: %w", err)
	}
	return response, nil
}

func (c *NativeClient) updateAuthorizationFrom(response map[string]any) {
	rawType := rawAuthorizationState(response)
	if !isRawAuthorizationState(rawType) {
		return
	}
	c.rawAuthState = rawType
	state := mapAuthorizationState(rawType)
	if state != AuthorizationStateUnknown {
		c.authState = state
	}
}

func (c *NativeClient) updateChatFoldersFrom(response map[string]any) {
	if nestedTypeValue(response) != "updateChatFolders" {
		return
	}
	c.folders = mapChatFolders(response)
	c.haveFolders = true
}

func rawAuthorizationState(response map[string]any) string {
	rawType := nestedTypeValue(response)
	if rawType == "updateAuthorizationState" {
		return nestedType(response, "authorization_state")
	}
	return rawType
}

func isRawAuthorizationState(rawType string) bool {
	return strings.HasPrefix(rawType, "authorizationState")
}

func tdlibChatList(list ChatList) map[string]any {
	if list == ChatListArchive {
		return map[string]any{"@type": "chatListArchive"}
	}
	return map[string]any{"@type": "chatListMain"}
}

func tdlibFolderChatList(folderID int32) map[string]any {
	return map[string]any{"@type": "chatListFolder", "chat_folder_id": folderID}
}

func int64FromAny(value any) int64 {
	return int64Field(map[string]any{"value": value}, "value")
}

func cloneFolders(folders []domain.TelegramFolder) []domain.TelegramFolder {
	return append([]domain.TelegramFolder(nil), folders...)
}

var _ TelegramClient = (*NativeClient)(nil)
