package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/sourcegroups"
	"github.com/kirilllebedenko/content_scout/internal/summary"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

type Server struct {
	httpServer *http.Server
	db         *sql.DB
	logger     *slog.Logger
	auth       AuthController
	sync       SyncController
	groups     GroupController
	collector  CollectionController
	summary    SummaryController
}

type AuthController interface {
	Start(ctx context.Context, telegramUserID int64) (*tdlib.AuthStatus, error)
	SubmitPhoneNumber(ctx context.Context, telegramUserID int64, phone string) (*tdlib.AuthStatus, error)
	SubmitCode(ctx context.Context, telegramUserID int64, code string) (*tdlib.AuthStatus, error)
	SubmitPassword(ctx context.Context, telegramUserID int64, password string) (*tdlib.AuthStatus, error)
	Status(ctx context.Context, telegramUserID int64) (*tdlib.AuthStatus, error)
	DeleteSession(ctx context.Context, telegramUserID int64) error
}

type SyncController interface {
	Sync(ctx context.Context, telegramUserID int64) (*tdlib.SyncResult, error)
	ListFolders(ctx context.Context, telegramUserID int64) ([]domain.TelegramFolder, error)
	ListChats(ctx context.Context, telegramUserID int64) ([]domain.TelegramChat, error)
}

type GroupController interface {
	Create(ctx context.Context, telegramUserID int64, name, description string) (*domain.SourceGroup, error)
	Update(ctx context.Context, telegramUserID, groupID int64, name, description string) (*domain.SourceGroup, error)
	Delete(ctx context.Context, telegramUserID, groupID int64) error
	List(ctx context.Context, telegramUserID int64) ([]domain.SourceGroup, error)
	AddChat(ctx context.Context, telegramUserID, groupID, chatID int64, priority int, enabled bool) error
	RemoveChat(ctx context.Context, telegramUserID, groupID, chatID int64) error
	ListChats(ctx context.Context, telegramUserID, groupID int64) (*sourcegroups.GroupWithChats, error)
}

type CollectionController interface {
	CollectGroup(ctx context.Context, req collection.Request) (*collection.Result, error)
}

type SummaryController interface {
	GenerateFromCollection(ctx context.Context, req summary.GenerateRequest) (*summary.GenerateResult, error)
}

func New(addr string, db *sql.DB, logger *slog.Logger) *Server {
	return NewWithAuth(addr, db, logger, nil)
}

func NewWithAuth(addr string, db *sql.DB, logger *slog.Logger, auth AuthController) *Server {
	return NewWithControllers(addr, db, logger, auth, nil)
}

func NewWithControllers(addr string, db *sql.DB, logger *slog.Logger, auth AuthController, sync SyncController) *Server {
	return NewWithAllControllers(addr, db, logger, auth, sync, nil)
}

func NewWithAllControllers(addr string, db *sql.DB, logger *slog.Logger, auth AuthController, sync SyncController, groups GroupController) *Server {
	return NewWithRuntime(addr, db, logger, auth, sync, groups, nil)
}

func NewWithRuntime(addr string, db *sql.DB, logger *slog.Logger, auth AuthController, sync SyncController, groups GroupController, collector CollectionController) *Server {
	return NewWithServices(addr, db, logger, auth, sync, groups, collector, nil)
}

func NewWithServices(addr string, db *sql.DB, logger *slog.Logger, auth AuthController, sync SyncController, groups GroupController, collector CollectionController, summaryService SummaryController) *Server {
	server := &Server{
		db:        db,
		logger:    logger,
		auth:      auth,
		sync:      sync,
		groups:    groups,
		collector: collector,
		summary:   summaryService,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.health)
	mux.HandleFunc("GET /ready", server.ready)
	mux.HandleFunc("GET /telegram/auth/status", server.authStatus)
	mux.HandleFunc("POST /telegram/auth/start", server.authStart)
	mux.HandleFunc("POST /telegram/auth/phone", server.authPhone)
	mux.HandleFunc("POST /telegram/auth/code", server.authCode)
	mux.HandleFunc("POST /telegram/auth/password", server.authPassword)
	mux.HandleFunc("DELETE /telegram/session", server.authDeleteSession)
	mux.HandleFunc("POST /telegram/sync", server.telegramSync)
	mux.HandleFunc("GET /telegram/folders", server.telegramFolders)
	mux.HandleFunc("GET /telegram/chats", server.telegramChats)
	mux.HandleFunc("GET /groups", server.groupsList)
	mux.HandleFunc("POST /groups", server.groupsCreate)
	mux.HandleFunc("PATCH /groups/{id}", server.groupsUpdate)
	mux.HandleFunc("DELETE /groups/{id}", server.groupsDelete)
	mux.HandleFunc("GET /groups/{id}/chats", server.groupChatsList)
	mux.HandleFunc("POST /groups/{id}/chats", server.groupChatsAdd)
	mux.HandleFunc("DELETE /groups/{id}/chats/{chatId}", server.groupChatsRemove)
	mux.HandleFunc("POST /collections/group/{id}", server.collectionGroupCreate)
	mux.HandleFunc("POST /summaries/from-collection/{id}", server.summaryFromCollection)

	server.httpServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server
}

func (s *Server) Run() error {
	s.logger.Info("api server starting", "addr", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown api server: %w", err)
	}
	return nil
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.db.PingContext(ctx); err != nil {
		s.logger.Warn("readiness check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
