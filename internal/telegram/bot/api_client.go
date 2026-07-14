package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/summary"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

type APIClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewAPIClient(baseURL, token string, httpClient *http.Client) *APIClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &APIClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: httpClient,
	}
}

func (c *APIClient) Start(ctx context.Context, telegramUserID int64) (*tdlib.AuthStatus, error) {
	var response authStatusAPIResponse
	if err := c.doJSON(ctx, http.MethodPost, "/telegram/auth/start", authOwnerAPIRequest{TelegramUserID: telegramUserID}, &response); err != nil {
		return nil, err
	}
	return response.authStatus(), nil
}

func (c *APIClient) SubmitPhoneNumber(ctx context.Context, telegramUserID int64, phone string) (*tdlib.AuthStatus, error) {
	var response authStatusAPIResponse
	if err := c.doJSON(ctx, http.MethodPost, "/telegram/auth/phone", authPhoneAPIRequest{TelegramUserID: telegramUserID, Phone: phone}, &response); err != nil {
		return nil, err
	}
	return response.authStatus(), nil
}

func (c *APIClient) SubmitCode(ctx context.Context, telegramUserID int64, code string) (*tdlib.AuthStatus, error) {
	var response authStatusAPIResponse
	if err := c.doJSON(ctx, http.MethodPost, "/telegram/auth/code", authCodeAPIRequest{TelegramUserID: telegramUserID, Code: code}, &response); err != nil {
		return nil, err
	}
	return response.authStatus(), nil
}

func (c *APIClient) SubmitPassword(ctx context.Context, telegramUserID int64, password string) (*tdlib.AuthStatus, error) {
	var response authStatusAPIResponse
	if err := c.doJSON(ctx, http.MethodPost, "/telegram/auth/password", authPasswordAPIRequest{TelegramUserID: telegramUserID, Password: password}, &response); err != nil {
		return nil, err
	}
	return response.authStatus(), nil
}

func (c *APIClient) Status(ctx context.Context, telegramUserID int64) (*tdlib.AuthStatus, error) {
	var response authStatusAPIResponse
	path := fmt.Sprintf("/telegram/auth/status?telegram_user_id=%d", telegramUserID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	return response.authStatus(), nil
}

func (c *APIClient) DeleteSession(ctx context.Context, telegramUserID int64) error {
	return c.doJSON(ctx, http.MethodDelete, "/telegram/session", authOwnerAPIRequest{TelegramUserID: telegramUserID}, nil)
}

func (c *APIClient) Sync(ctx context.Context, telegramUserID int64) (*tdlib.SyncResult, error) {
	var response syncAPIResponse
	if err := c.doJSON(ctx, http.MethodPost, "/telegram/sync", authOwnerAPIRequest{TelegramUserID: telegramUserID}, &response); err != nil {
		return nil, err
	}
	return &tdlib.SyncResult{
		UserID:       response.UserID,
		FoldersCount: response.FoldersCount,
		ChatsCount:   response.ChatsCount,
		SyncedAt:     response.SyncedAt,
	}, nil
}

func (c *APIClient) ListFolders(ctx context.Context, telegramUserID int64) ([]domain.TelegramFolder, error) {
	var response []folderAPIResponse
	path := fmt.Sprintf("/telegram/folders?telegram_user_id=%d", telegramUserID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	folders := make([]domain.TelegramFolder, 0, len(response))
	for _, folder := range response {
		folders = append(folders, domain.TelegramFolder{
			ID:         folder.ID,
			TelegramID: folder.TelegramID,
			Name:       folder.Name,
			SyncedAt:   folder.SyncedAt,
		})
	}
	return folders, nil
}

func (c *APIClient) ListChats(ctx context.Context, telegramUserID int64) ([]domain.TelegramChat, error) {
	var response []chatAPIResponse
	path := fmt.Sprintf("/telegram/chats?telegram_user_id=%d", telegramUserID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	chats := make([]domain.TelegramChat, 0, len(response))
	for _, chat := range response {
		chats = append(chats, domain.TelegramChat{
			ID:             chat.ID,
			TelegramChatID: chat.TelegramChatID,
			Title:          chat.Title,
			Username:       chat.Username,
			Type:           domain.ChatType(chat.Type),
			IsArchived:     chat.IsArchived,
			IsMuted:        chat.IsMuted,
			UnreadCount:    chat.UnreadCount,
			LastMessageID:  chat.LastMessageID,
		})
	}
	return chats, nil
}

func (c *APIClient) CollectGroup(ctx context.Context, req collection.Request) (*collection.Result, error) {
	var response collectionAPIResponse
	path := fmt.Sprintf("/collections/group/%d", req.GroupID)
	body := collectionAPIRequest{
		TelegramUserID: req.TelegramUserID,
		Mode:           string(req.Mode),
		Limit:          req.Limit,
	}
	if err := c.doJSON(ctx, http.MethodPost, path, body, &response); err != nil {
		return nil, err
	}
	return &collection.Result{
		JobID:         response.JobID,
		UserID:        response.UserID,
		GroupID:       response.GroupID,
		ChatsCount:    response.ChatsCount,
		MessagesCount: response.MessagesCount,
	}, nil
}

func (c *APIClient) GenerateFromCollection(ctx context.Context, req summary.GenerateRequest) (*summary.GenerateResult, error) {
	var response summaryAPIResponse
	path := fmt.Sprintf("/summaries/from-collection/%d", req.CollectionJobID)
	body := summaryAPIRequest{
		TelegramUserID: req.TelegramUserID,
		Format:         req.Format,
	}
	if err := c.doJSON(ctx, http.MethodPost, path, body, &response); err != nil {
		return nil, err
	}
	return &summary.GenerateResult{
		SummaryID:      response.SummaryID,
		SummaryJobID:   response.SummaryJobID,
		TopicsCount:    response.TopicsCount,
		MessagesCount:  response.MessagesCount,
		DuplicateCount: response.DuplicateCount,
	}, nil
}

func (c *APIClient) doJSON(ctx context.Context, method, path string, body any, target any) error {
	if c.baseURL == "" {
		return errors.New("internal API URL is not configured")
	}
	endpoint, err := url.JoinPath(c.baseURL, strings.TrimPrefix(path, "/"))
	if err != nil {
		return err
	}
	if strings.Contains(path, "?") {
		endpoint = c.baseURL + path
	}

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var errorResponse apiErrorResponse
		if err := json.NewDecoder(response.Body).Decode(&errorResponse); err == nil && errorResponse.Error != "" {
			return errors.New(errorResponse.Error)
		}
		return fmt.Errorf("internal API request failed: %s", response.Status)
	}
	if target == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(target)
}

type authOwnerAPIRequest struct {
	TelegramUserID int64 `json:"telegram_user_id"`
}

type authPhoneAPIRequest struct {
	TelegramUserID int64  `json:"telegram_user_id"`
	Phone          string `json:"phone"`
}

type authCodeAPIRequest struct {
	TelegramUserID int64  `json:"telegram_user_id"`
	Code           string `json:"code"`
}

type authPasswordAPIRequest struct {
	TelegramUserID int64  `json:"telegram_user_id"`
	Password       string `json:"password"`
}

type authStatusAPIResponse struct {
	UserID       int64  `json:"user_id"`
	SessionID    int64  `json:"session_id"`
	SessionState string `json:"session_state"`
	AuthState    string `json:"auth_state"`
}

func (r authStatusAPIResponse) authStatus() *tdlib.AuthStatus {
	return &tdlib.AuthStatus{
		UserID:       r.UserID,
		SessionID:    r.SessionID,
		SessionState: domain.SessionStatus(r.SessionState),
		AuthState:    tdlib.AuthorizationState(r.AuthState),
	}
}

type syncAPIResponse struct {
	UserID       int64     `json:"user_id"`
	FoldersCount int       `json:"folders_count"`
	ChatsCount   int       `json:"chats_count"`
	SyncedAt     time.Time `json:"synced_at"`
}

type folderAPIResponse struct {
	ID         int64     `json:"id"`
	TelegramID int32     `json:"telegram_id"`
	Name       string    `json:"name"`
	SyncedAt   time.Time `json:"synced_at"`
}

type chatAPIResponse struct {
	ID             int64   `json:"id"`
	TelegramChatID int64   `json:"telegram_chat_id"`
	Title          string  `json:"title"`
	Username       *string `json:"username,omitempty"`
	Type           string  `json:"type"`
	IsArchived     bool    `json:"is_archived"`
	IsMuted        bool    `json:"is_muted"`
	UnreadCount    int     `json:"unread_count"`
	LastMessageID  int64   `json:"last_message_id"`
}

type collectionAPIRequest struct {
	TelegramUserID int64  `json:"telegram_user_id"`
	Mode           string `json:"mode"`
	Limit          int    `json:"limit"`
}

type collectionAPIResponse struct {
	JobID         int64 `json:"job_id"`
	UserID        int64 `json:"user_id"`
	GroupID       int64 `json:"group_id"`
	ChatsCount    int   `json:"chats_count"`
	MessagesCount int   `json:"messages_count"`
}

type summaryAPIRequest struct {
	TelegramUserID int64  `json:"telegram_user_id"`
	Format         string `json:"format"`
}

type summaryAPIResponse struct {
	SummaryID      int64 `json:"summary_id"`
	SummaryJobID   int64 `json:"summary_job_id"`
	TopicsCount    int   `json:"topics_count"`
	MessagesCount  int   `json:"messages_count"`
	DuplicateCount int   `json:"duplicate_count"`
}

type apiErrorResponse struct {
	Error string `json:"error"`
}
