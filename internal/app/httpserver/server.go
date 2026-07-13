package httpserver

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/article"
	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/obsidian"
	"github.com/kirilllebedenko/content_scout/internal/sourcegroups"
	"github.com/kirilllebedenko/content_scout/internal/summary"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

type Server struct {
	httpServer *http.Server
	db         *sql.DB
	logger     *slog.Logger
	options    Options
	auth       AuthController
	sync       SyncController
	groups     GroupController
	collector  CollectionController
	summary    SummaryController
	browser    SummaryBrowser
	articles   ArticleController
	exports    ExportController
}

type Options struct {
	ServiceToken      string
	RequireAuth       bool
	MaxRequestBytes   int64
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

const defaultMaxRequestBytes int64 = 1 << 20

func DefaultOptions() Options {
	return Options{
		MaxRequestBytes:   defaultMaxRequestBytes,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
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

type SummaryBrowser interface {
	ListSummaries(ctx context.Context, telegramUserID int64, limit int) ([]domain.Summary, error)
	GetSummary(ctx context.Context, telegramUserID, summaryID int64) (*domain.Summary, error)
	ListTopics(ctx context.Context, telegramUserID, summaryID int64) ([]domain.SummaryTopic, error)
}

type ArticleController interface {
	ConvertSummary(ctx context.Context, req article.ConvertRequest) (*article.Result, error)
	ConvertTopic(ctx context.Context, req article.ConvertRequest) (*article.Result, error)
	List(ctx context.Context, telegramUserID int64, limit int) ([]domain.Article, error)
	Get(ctx context.Context, telegramUserID, articleID int64) (*domain.Article, error)
	UpdateMetadata(ctx context.Context, telegramUserID, articleID int64, title string, tags []string) (*domain.Article, error)
}

type ExportController interface {
	ExportArticle(ctx context.Context, telegramUserID, articleID int64) (*obsidian.Result, error)
	ExportSummary(ctx context.Context, telegramUserID, summaryID int64) (*obsidian.Result, error)
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
	return NewWithBrowser(addr, db, logger, auth, sync, groups, collector, summaryService, nil)
}

func NewWithBrowser(addr string, db *sql.DB, logger *slog.Logger, auth AuthController, sync SyncController, groups GroupController, collector CollectionController, summaryService SummaryController, browser SummaryBrowser) *Server {
	return NewWithArticle(addr, db, logger, auth, sync, groups, collector, summaryService, browser, nil)
}

func NewWithArticle(addr string, db *sql.DB, logger *slog.Logger, auth AuthController, sync SyncController, groups GroupController, collector CollectionController, summaryService SummaryController, browser SummaryBrowser, articles ArticleController) *Server {
	return NewWithExports(addr, db, logger, auth, sync, groups, collector, summaryService, browser, articles, nil)
}

func NewWithExports(addr string, db *sql.DB, logger *slog.Logger, auth AuthController, sync SyncController, groups GroupController, collector CollectionController, summaryService SummaryController, browser SummaryBrowser, articles ArticleController, exports ExportController) *Server {
	return NewWithOptions(addr, db, logger, DefaultOptions(), auth, sync, groups, collector, summaryService, browser, articles, exports)
}

func NewWithOptions(addr string, db *sql.DB, logger *slog.Logger, options Options, auth AuthController, sync SyncController, groups GroupController, collector CollectionController, summaryService SummaryController, browser SummaryBrowser, articles ArticleController, exports ExportController) *Server {
	options = normalizeOptions(options)
	server := &Server{
		db:        db,
		logger:    logger,
		options:   options,
		auth:      auth,
		sync:      sync,
		groups:    groups,
		collector: collector,
		summary:   summaryService,
		browser:   browser,
		articles:  articles,
		exports:   exports,
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
	mux.HandleFunc("GET /summaries", server.summariesList)
	mux.HandleFunc("GET /summaries/{id}", server.summaryGet)
	mux.HandleFunc("GET /summaries/{id}/topics", server.summaryTopics)
	mux.HandleFunc("POST /articles/from-summary/{id}", server.articleFromSummary)
	mux.HandleFunc("POST /articles/from-summary/{id}/topics/{position}", server.articleFromTopic)
	mux.HandleFunc("GET /articles", server.articlesList)
	mux.HandleFunc("GET /articles/{id}", server.articleGet)
	mux.HandleFunc("PATCH /articles/{id}", server.articleUpdate)
	mux.HandleFunc("POST /exports/articles/{id}", server.exportArticle)
	mux.HandleFunc("POST /exports/summaries/{id}", server.exportSummary)

	handler := server.securityHeaders(server.authenticate(mux))
	server.httpServer = &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: options.ReadHeaderTimeout,
		ReadTimeout:       options.ReadTimeout,
		WriteTimeout:      options.WriteTimeout,
		IdleTimeout:       options.IdleTimeout,
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

func normalizeOptions(options Options) Options {
	defaults := DefaultOptions()
	if options.MaxRequestBytes <= 0 {
		options.MaxRequestBytes = defaults.MaxRequestBytes
	}
	if options.ReadHeaderTimeout <= 0 {
		options.ReadHeaderTimeout = defaults.ReadHeaderTimeout
	}
	if options.ReadTimeout <= 0 {
		options.ReadTimeout = defaults.ReadTimeout
	}
	if options.WriteTimeout <= 0 {
		options.WriteTimeout = defaults.WriteTimeout
	}
	if options.IdleTimeout <= 0 {
		options.IdleTimeout = defaults.IdleTimeout
	}
	return options
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, s.options.MaxRequestBytes)
		if isPublicEndpoint(r) {
			next.ServeHTTP(w, r)
			return
		}
		if !s.options.RequireAuth {
			next.ServeHTTP(w, r)
			return
		}
		if s.options.ServiceToken == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "service token is not configured"})
			return
		}
		if !validBearerToken(r.Header.Get("Authorization"), s.options.ServiceToken) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func isPublicEndpoint(r *http.Request) bool {
	return r.Method == http.MethodGet && (r.URL.Path == "/health" || r.URL.Path == "/ready")
}

func validBearerToken(header, expected string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
