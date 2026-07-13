package tdlib

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/storage"
)

var ErrUnauthorizedOwner = errors.New("telegram user is not allowed to manage this bot")

type AuthStatus struct {
	UserID       int64
	SessionID    int64
	SessionPath  string
	SessionState domain.SessionStatus
	AuthState    AuthorizationState
}

type AuthService struct {
	ownerTelegramID int64
	apiID           int
	apiHash         string
	storageBaseDir  string
	users           storage.UserRepository
	sessions        storage.TelegramSessionRepository
	factory         ClientFactory
	removeAll       func(string) error
	now             func() time.Time
}

type AuthServiceConfig struct {
	OwnerTelegramID int64
	TelegramAPIID   int
	TelegramAPIHash string
	StorageBaseDir  string
}

func NewAuthService(cfg AuthServiceConfig, users storage.UserRepository, sessions storage.TelegramSessionRepository, factory ClientFactory) *AuthService {
	return &AuthService{
		ownerTelegramID: cfg.OwnerTelegramID,
		apiID:           cfg.TelegramAPIID,
		apiHash:         cfg.TelegramAPIHash,
		storageBaseDir:  cfg.StorageBaseDir,
		users:           users,
		sessions:        sessions,
		factory:         factory,
		removeAll:       os.RemoveAll,
		now:             time.Now,
	}
}

func (s *AuthService) Start(ctx context.Context, telegramUserID int64) (*AuthStatus, error) {
	if err := s.ensureOwner(telegramUserID); err != nil {
		return nil, err
	}
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}

	user, err := s.users.UpsertByTelegramID(ctx, telegramUserID)
	if err != nil {
		return nil, fmt.Errorf("upsert owner user: %w", err)
	}
	sessionPath := s.sessionPath(user.ID)
	if err := os.MkdirAll(sessionPath, 0o700); err != nil {
		return nil, fmt.Errorf("create tdlib session directory: %w", err)
	}

	client, err := s.factory.NewClient(sessionPath)
	if err != nil {
		return nil, fmt.Errorf("create tdlib client: %w", err)
	}
	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("start tdlib client: %w", err)
	}
	return s.persistClientState(ctx, user.ID, sessionPath, client)
}

func (s *AuthService) SubmitPhoneNumber(ctx context.Context, telegramUserID int64, phone string) (*AuthStatus, error) {
	return s.submit(ctx, telegramUserID, func(client TelegramClient) error {
		return client.SubmitPhoneNumber(ctx, phone)
	})
}

func (s *AuthService) SubmitCode(ctx context.Context, telegramUserID int64, code string) (*AuthStatus, error) {
	return s.submit(ctx, telegramUserID, func(client TelegramClient) error {
		return client.SubmitCode(ctx, code)
	})
}

func (s *AuthService) SubmitPassword(ctx context.Context, telegramUserID int64, password string) (*AuthStatus, error) {
	return s.submit(ctx, telegramUserID, func(client TelegramClient) error {
		return client.SubmitPassword(ctx, password)
	})
}

func (s *AuthService) Status(ctx context.Context, telegramUserID int64) (*AuthStatus, error) {
	if err := s.ensureOwner(telegramUserID); err != nil {
		return nil, err
	}
	user, err := s.users.FindByTelegramID(ctx, telegramUserID)
	if err != nil {
		return nil, fmt.Errorf("find owner user: %w", err)
	}
	if user == nil {
		return &AuthStatus{AuthState: AuthorizationStateClosed, SessionState: domain.SessionStatusDisconnected}, nil
	}
	session, err := s.sessions.FindByUserID(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("find telegram session: %w", err)
	}
	if session == nil {
		return &AuthStatus{UserID: user.ID, AuthState: AuthorizationStateClosed, SessionState: domain.SessionStatusDisconnected}, nil
	}
	client, err := s.factory.NewClient(session.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("create tdlib client: %w", err)
	}
	authState, err := client.AuthorizationState(ctx)
	if err != nil {
		return nil, fmt.Errorf("get tdlib authorization state: %w", err)
	}
	return s.upsertSession(ctx, user.ID, session.StoragePath, authState)
}

func (s *AuthService) DeleteSession(ctx context.Context, telegramUserID int64) error {
	if err := s.ensureOwner(telegramUserID); err != nil {
		return err
	}
	user, err := s.users.FindByTelegramID(ctx, telegramUserID)
	if err != nil {
		return fmt.Errorf("find owner user: %w", err)
	}
	if user == nil {
		return nil
	}
	session, err := s.sessions.FindByUserID(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("find telegram session: %w", err)
	}
	if session != nil {
		client, err := s.factory.NewClient(session.StoragePath)
		if err != nil {
			return fmt.Errorf("create tdlib client: %w", err)
		}
		if err := client.Stop(ctx); err != nil {
			return fmt.Errorf("stop tdlib client: %w", err)
		}
		if err := s.sessions.DeleteByUserID(ctx, user.ID); err != nil {
			return err
		}
		if err := s.removeAll(session.StoragePath); err != nil {
			return fmt.Errorf("remove tdlib session directory: %w", err)
		}
	}
	return nil
}

func (s *AuthService) submit(ctx context.Context, telegramUserID int64, submit func(TelegramClient) error) (*AuthStatus, error) {
	if err := s.ensureOwner(telegramUserID); err != nil {
		return nil, err
	}
	user, err := s.users.FindByTelegramID(ctx, telegramUserID)
	if err != nil {
		return nil, fmt.Errorf("find owner user: %w", err)
	}
	if user == nil {
		return nil, errors.New("telegram session is not started")
	}
	session, err := s.sessions.FindByUserID(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("find telegram session: %w", err)
	}
	if session == nil {
		return nil, errors.New("telegram session is not started")
	}
	client, err := s.factory.NewClient(session.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("create tdlib client: %w", err)
	}
	if err := submit(client); err != nil {
		return nil, fmt.Errorf("submit tdlib authorization input: %w", err)
	}
	return s.persistClientState(ctx, user.ID, session.StoragePath, client)
}

func (s *AuthService) persistClientState(ctx context.Context, userID int64, sessionPath string, client TelegramClient) (*AuthStatus, error) {
	authState, err := client.AuthorizationState(ctx)
	if err != nil {
		return nil, fmt.Errorf("get tdlib authorization state: %w", err)
	}
	return s.upsertSession(ctx, userID, sessionPath, authState)
}

func (s *AuthService) upsertSession(ctx context.Context, userID int64, sessionPath string, authState AuthorizationState) (*AuthStatus, error) {
	lastConnected := (*time.Time)(nil)
	if authState == AuthorizationStateReady {
		now := s.now()
		lastConnected = &now
	}
	session, err := s.sessions.Upsert(ctx, domain.TelegramSession{
		UserID:        userID,
		StoragePath:   sessionPath,
		Status:        sessionStatus(authState),
		LastConnected: lastConnected,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert telegram session: %w", err)
	}
	return &AuthStatus{
		UserID:       userID,
		SessionID:    session.ID,
		SessionPath:  session.StoragePath,
		SessionState: session.Status,
		AuthState:    authState,
	}, nil
}

func (s *AuthService) ensureOwner(telegramUserID int64) error {
	if s.ownerTelegramID == 0 || telegramUserID != s.ownerTelegramID {
		return ErrUnauthorizedOwner
	}
	return nil
}

func (s *AuthService) ensureConfigured() error {
	if s.apiID == 0 {
		return errors.New("TELEGRAM_API_ID is not configured")
	}
	if strings.TrimSpace(s.apiHash) == "" {
		return errors.New("TELEGRAM_API_HASH is not configured")
	}
	if strings.TrimSpace(s.storageBaseDir) == "" {
		return errors.New("TDLIB_DATABASE_DIR is not configured")
	}
	if s.factory == nil {
		return errors.New("tdlib client factory is not configured")
	}
	return nil
}

func (s *AuthService) sessionPath(userID int64) string {
	return filepath.Join(s.storageBaseDir, fmt.Sprintf("user_%d", userID))
}

func sessionStatus(authState AuthorizationState) domain.SessionStatus {
	switch authState {
	case AuthorizationStateReady:
		return domain.SessionStatusConnected
	case AuthorizationStateWaitPhoneNumber, AuthorizationStateWaitCode, AuthorizationStateWaitPassword, AuthorizationStateUnknown:
		return domain.SessionStatusAuthorizing
	case AuthorizationStateError:
		return domain.SessionStatusError
	default:
		return domain.SessionStatusDisconnected
	}
}
