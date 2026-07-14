package obsidian

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/article"
	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/storage"
	"github.com/kirilllebedenko/content_scout/internal/summary"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

type Service struct {
	ownerTelegramID int64
	exportDir       string
	users           storage.UserRepository
	articles        storage.ArticleRepository
	summaries       storage.SummaryRepository
	exports         storage.ObsidianExportRepository
	rest            RESTNoteClient
	now             func() time.Time
}

type RESTNoteClient interface {
	Enabled() bool
	ReadNote(ctx context.Context, vaultPath string) ([]byte, error)
	WriteNote(ctx context.Context, vaultPath string, content []byte) error
}

type Result struct {
	Export  domain.ObsidianExport
	Path    string
	Content []byte
	Reused  bool
}

func NewService(ownerTelegramID int64, exportDir string, users storage.UserRepository, articles storage.ArticleRepository, summaries storage.SummaryRepository, exports storage.ObsidianExportRepository) *Service {
	return NewServiceWithREST(ownerTelegramID, exportDir, users, articles, summaries, exports, nil)
}

func NewServiceWithREST(ownerTelegramID int64, exportDir string, users storage.UserRepository, articles storage.ArticleRepository, summaries storage.SummaryRepository, exports storage.ObsidianExportRepository, rest RESTNoteClient) *Service {
	if strings.TrimSpace(exportDir) == "" {
		exportDir = "./data/exports"
	}
	return &Service{
		ownerTelegramID: ownerTelegramID,
		exportDir:       exportDir,
		users:           users,
		articles:        articles,
		summaries:       summaries,
		exports:         exports,
		rest:            rest,
		now:             time.Now,
	}
}

func (s *Service) ExportArticle(ctx context.Context, telegramUserID, articleID int64) (*Result, error) {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	item, err := s.articles.FindByUser(ctx, user.ID, articleID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, article.ErrArticleNotFound
	}
	sources, err := s.articles.ListSources(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	content := RenderArticle(*item, sources)
	fileName := safeFileName(item.Title, ".md")
	vaultPath := filepath.ToSlash(filepath.Join("Articles", categoryDir(item.Tags), fileName))
	return s.persist(ctx, content, fileName, vaultPath, &item.ID, nil)
}

func (s *Service) ExportSummary(ctx context.Context, telegramUserID, summaryID int64) (*Result, error) {
	user, err := s.ownerUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	item, err := s.summaries.FindSummaryByUser(ctx, user.ID, summaryID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, summary.ErrSummaryNotFound
	}
	content := RenderSummary(*item)
	fileName := safeFileName(item.Title, ".md")
	vaultPath := filepath.ToSlash(filepath.Join("Summaries", "Telegram", fileName))
	return s.persist(ctx, content, fileName, vaultPath, nil, &item.ID)
}

func (s *Service) persist(ctx context.Context, content []byte, fileName, vaultPath string, articleID, summaryID *int64) (*Result, error) {
	hash := contentHash(content)
	if existing, err := s.exports.FindByContentHash(ctx, hash); err != nil {
		return nil, err
	} else if existing != nil {
		path := filepath.Join(s.exportDir, existing.VaultPath)
		return &Result{Export: *existing, Path: path, Content: content, Reused: true}, nil
	}
	path, vaultPath, fileName, err := s.writeLocalExport(content, vaultPath)
	if err != nil {
		return nil, err
	}
	exportMethod := "telegram_document"
	if s.rest != nil && s.rest.Enabled() {
		if err := s.writeRESTNote(ctx, vaultPath, content); err != nil {
			return nil, err
		}
		exportMethod = "obsidian_rest"
	}
	created, err := s.exports.Create(ctx, domain.ObsidianExport{
		ArticleID:    articleID,
		SummaryID:    summaryID,
		FileName:     fileName,
		VaultPath:    vaultPath,
		ExportMethod: exportMethod,
		ContentHash:  hash,
	})
	if err != nil {
		return nil, err
	}
	return &Result{Export: *created, Path: path, Content: content}, nil
}

func (s *Service) writeLocalExport(content []byte, vaultPath string) (string, string, string, error) {
	var lastErr error
	for _, exportDir := range exportDirCandidates(s.exportDir) {
		path := filepath.Join(exportDir, vaultPath)
		path = uniquePath(path)
		currentVaultPath := filepath.ToSlash(strings.TrimPrefix(path, filepath.Clean(exportDir)+string(os.PathSeparator)))
		fileName := filepath.Base(path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			lastErr = fmt.Errorf("create export directory %q: %w", filepath.Dir(path), err)
			if recoverableExportDirError(err) {
				continue
			}
			return "", "", "", lastErr
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			lastErr = fmt.Errorf("write obsidian markdown %q: %w", path, err)
			if recoverableExportDirError(err) {
				continue
			}
			return "", "", "", lastErr
		}
		return path, currentVaultPath, fileName, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no export directories configured")
	}
	return "", "", "", lastErr
}

func exportDirCandidates(exportDir string) []string {
	seen := make(map[string]struct{}, 3)
	candidates := make([]string, 0, 3)
	for _, candidate := range []string{
		exportDir,
		"./data/exports",
		filepath.Join(os.TempDir(), "content_scout", "exports"),
	} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		clean := filepath.Clean(candidate)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		candidates = append(candidates, clean)
	}
	return candidates
}

func recoverableExportDirError(err error) bool {
	return errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EROFS) || errors.Is(err, syscall.EACCES)
}

func (s *Service) writeRESTNote(ctx context.Context, vaultPath string, content []byte) error {
	existing, err := s.rest.ReadNote(ctx, vaultPath)
	if err != nil && !errors.Is(err, ErrNoteNotFound) {
		return err
	}
	if err == nil && len(existing) > 0 {
		backupPath := backupVaultPath(vaultPath, s.now())
		if err := s.rest.WriteNote(ctx, backupPath, existing); err != nil {
			return fmt.Errorf("create obsidian backup: %w", err)
		}
	}
	if err := s.rest.WriteNote(ctx, vaultPath, content); err != nil {
		return fmt.Errorf("write obsidian note: %w", err)
	}
	return nil
}

func (s *Service) ownerUser(ctx context.Context, telegramUserID int64) (*domain.User, error) {
	if s.ownerTelegramID == 0 || telegramUserID != s.ownerTelegramID {
		return nil, tdlib.ErrUnauthorizedOwner
	}
	user, err := s.users.FindByTelegramID(ctx, telegramUserID)
	if err != nil {
		return nil, fmt.Errorf("find owner user: %w", err)
	}
	if user == nil {
		return nil, errors.New("owner user is not initialized")
	}
	return user, nil
}

func RenderArticle(item domain.Article, sources []domain.ArticleSource) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	writeYAMLString(&b, "type", "article")
	writeYAMLString(&b, "status", string(item.Status))
	writeYAMLString(&b, "created", item.CreatedAt.Format("2006-01-02"))
	writeYAMLString(&b, "updated", item.UpdatedAt.Format("2006-01-02"))
	writeYAMLString(&b, "source", "telegram")
	fmt.Fprintf(&b, "sources_count: %d\n", len(sources))
	writeYAMLList(&b, "tags", item.Tags)
	b.WriteString("---\n\n")
	content := strings.TrimSpace(item.ContentMarkdown)
	if content == "" || !strings.HasPrefix(content, "#") {
		fmt.Fprintf(&b, "# %s\n\n", item.Title)
	}
	b.WriteString(content)
	b.WriteString("\n")
	if len(sources) > 0 && strings.Contains(strings.ToLower(content), "## источники") {
		return []byte(b.String())
	}
	if len(sources) > 0 {
		b.WriteString("\n## Источники\n")
	}
	for i, source := range sources {
		if source.SourceURL == "" {
			fmt.Fprintf(&b, "\n%d. %s", i+1, source.SourceTitle)
			continue
		}
		fmt.Fprintf(&b, "\n%d. [%s — исходный пост](%s)", i+1, source.SourceTitle, source.SourceURL)
	}
	b.WriteString("\n")
	return []byte(b.String())
}

func RenderSummary(item domain.Summary) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	writeYAMLString(&b, "type", "summary")
	writeYAMLString(&b, "status", "ready")
	writeYAMLString(&b, "created", item.CreatedAt.Format("2006-01-02"))
	writeYAMLString(&b, "source", "telegram")
	fmt.Fprintf(&b, "sources_count: %d\n", item.SourcesCount)
	fmt.Fprintf(&b, "messages_count: %d\n", item.MessagesCount)
	fmt.Fprintf(&b, "topics_count: %d\n", item.TopicsCount)
	writeYAMLList(&b, "tags", []string{"telegram", "summary"})
	b.WriteString("---\n\n")
	content := strings.TrimSpace(item.Markdown)
	if content == "" {
		fmt.Fprintf(&b, "# %s\n\n%s\n", item.Title, item.Overview)
	} else {
		b.WriteString(content)
		b.WriteString("\n")
	}
	return []byte(b.String())
}

func contentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func safeFileName(title, ext string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "telegram-export"
	}
	title = regexp.MustCompile(`[\/\\:\*\?"<>\|]+`).ReplaceAllString(title, "-")
	title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")
	title = strings.Trim(title, " .-_")
	if title == "" {
		title = "telegram-export"
	}
	runes := []rune(title)
	if len(runes) > 80 {
		title = string(runes[:80])
	}
	return title + ext
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d%s", base, time.Now().Unix(), ext)
}

func categoryDir(tags []string) string {
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			return safePathSegment(tag)
		}
	}
	return "Telegram"
}

func safePathSegment(value string) string {
	value = strings.TrimSuffix(safeFileName(value, ""), ".md")
	if value == "" {
		return "Telegram"
	}
	return value
}

func backupVaultPath(vaultPath string, now time.Time) string {
	ext := filepath.Ext(vaultPath)
	base := strings.TrimSuffix(vaultPath, ext)
	if ext == "" {
		ext = ".md"
	}
	return fmt.Sprintf("%s.backup-%s%s", base, now.Format("20060102-150405"), ext)
}

func writeYAMLString(b *strings.Builder, key, value string) {
	value = strings.ReplaceAll(value, `"`, `\"`)
	fmt.Fprintf(b, "%s: \"%s\"\n", key, value)
}

func writeYAMLList(b *strings.Builder, key string, values []string) {
	fmt.Fprintf(b, "%s:\n", key)
	if len(values) == 0 {
		b.WriteString("  - telegram\n")
		return
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		fmt.Fprintf(b, "  - %s\n", value)
	}
}
