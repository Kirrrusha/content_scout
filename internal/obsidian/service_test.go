package obsidian

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

func TestRenderArticleIncludesFrontmatterAndSources(t *testing.T) {
	created := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	content := RenderArticle(domain.Article{
		Title:           "Go Guide",
		Status:          domain.ArticleStatusDraft,
		Tags:            []string{"go", "telegram"},
		ContentMarkdown: "# Go Guide\n\nBody",
		CreatedAt:       created,
		UpdatedAt:       created,
	}, []domain.ArticleSource{{
		SourceTitle: "Golang Digest",
		SourceURL:   "https://t.me/golang/1",
		PublishedAt: created,
	}})

	text := string(content)
	for _, want := range []string{`type: "article"`, `status: "draft"`, "sources_count: 1", "- go", "# Go Guide", "[Golang Digest — исходный пост](https://t.me/golang/1)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered article missing %q:\n%s", want, text)
		}
	}
}

func TestRenderArticleDoesNotAppendSourcesWhenContentHasSourceSection(t *testing.T) {
	created := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	content := RenderArticle(domain.Article{
		Title:           "Go Guide",
		Status:          domain.ArticleStatusDraft,
		ContentMarkdown: "# Go Guide\n\nBody\n\n## Источники\n- [Primary](https://t.me/golang/1)",
		CreatedAt:       created,
		UpdatedAt:       created,
	}, []domain.ArticleSource{{
		SourceTitle: "Noise",
		SourceURL:   "https://t.me/noise/2",
		PublishedAt: created,
	}})

	text := string(content)
	if strings.Contains(text, "Noise") || strings.Contains(text, "https://t.me/noise/2") {
		t.Fatalf("rendered article appended duplicate sources:\n%s", text)
	}
}

func TestExportArticleWritesFileAndReusesHash(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	created := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	article := &domain.Article{
		ID:              7,
		UserID:          1,
		Title:           `Go:/Guide?`,
		Slug:            "go-guide",
		Status:          domain.ArticleStatusDraft,
		Tags:            []string{"go"},
		ContentMarkdown: "# Go Guide",
		CreatedAt:       created,
		UpdatedAt:       created,
	}
	exports := &fakeExports{}
	service := NewService(42, dir, &fakeUsers{user: &domain.User{ID: 1, TelegramUserID: 42}}, &fakeArticles{article: article}, nil, exports)

	first, err := service.ExportArticle(ctx, 42, 7)
	if err != nil {
		t.Fatalf("ExportArticle() error = %v", err)
	}
	if _, err := os.Stat(first.Path); err != nil {
		t.Fatalf("export file stat: %v", err)
	}
	if strings.Contains(first.Export.FileName, ":") || strings.Contains(first.Export.FileName, "?") {
		t.Fatalf("unsafe file name = %q", first.Export.FileName)
	}

	second, err := service.ExportArticle(ctx, 42, 7)
	if err != nil {
		t.Fatalf("second ExportArticle() error = %v", err)
	}
	if !second.Reused || second.Export.ID != first.Export.ID {
		t.Fatalf("second result = %+v, want reused first export", second)
	}
}

func TestExportArticleFallsBackWhenExportDirIsReadOnly(t *testing.T) {
	ctx := context.Background()
	cwd := t.TempDir()
	previousCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previousCWD)
	})
	blockedDir := filepath.Join(cwd, "blocked")
	if err := os.Mkdir(blockedDir, 0o500); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(blockedDir, 0o700)
	})
	created := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	article := &domain.Article{
		ID:              7,
		UserID:          1,
		Title:           "Go Guide",
		Slug:            "go-guide",
		Status:          domain.ArticleStatusDraft,
		ContentMarkdown: "# Go Guide",
		CreatedAt:       created,
		UpdatedAt:       created,
	}
	service := NewService(42, blockedDir, &fakeUsers{user: &domain.User{ID: 1, TelegramUserID: 42}}, &fakeArticles{article: article}, nil, &fakeExports{})

	result, err := service.ExportArticle(ctx, 42, 7)
	if err != nil {
		t.Fatalf("ExportArticle() error = %v", err)
	}
	if !strings.HasPrefix(result.Path, filepath.Clean("./data/exports")+string(os.PathSeparator)) {
		t.Fatalf("export path = %q, want local fallback under ./data/exports", result.Path)
	}
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatalf("fallback export stat: %v", err)
	}
}

func TestExportArticleWritesRESTBackupBeforeUpdate(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	created := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	article := &domain.Article{
		ID:              7,
		UserID:          1,
		Title:           "Go Guide",
		Slug:            "go-guide",
		Status:          domain.ArticleStatusDraft,
		ContentMarkdown: "# Go Guide",
		CreatedAt:       created,
		UpdatedAt:       created,
	}
	exports := &fakeExports{}
	mock := &fakeREST{existing: []byte("# Old")}
	service := NewServiceWithREST(42, dir, &fakeUsers{user: &domain.User{ID: 1, TelegramUserID: 42}}, &fakeArticles{article: article}, nil, exports, mock)

	result, err := service.ExportArticle(ctx, 42, 7)
	if err != nil {
		t.Fatalf("ExportArticle() error = %v", err)
	}
	if result.Export.ExportMethod != "obsidian_rest" {
		t.Fatalf("export method = %q", result.Export.ExportMethod)
	}
	if len(mock.writes) != 2 || !strings.Contains(mock.writes[0].path, ".backup-") || mock.writes[1].path != result.Export.VaultPath {
		t.Fatalf("writes = %+v", mock.writes)
	}
}

type fakeUsers struct{ user *domain.User }

func (f *fakeUsers) UpsertByTelegramID(context.Context, int64) (*domain.User, error) {
	return f.user, nil
}
func (f *fakeUsers) FindByTelegramID(_ context.Context, telegramUserID int64) (*domain.User, error) {
	if f.user != nil && f.user.TelegramUserID == telegramUserID {
		return f.user, nil
	}
	return nil, nil
}

type fakeArticles struct {
	article *domain.Article
	sources []domain.ArticleSource
}

func (f *fakeArticles) Create(context.Context, domain.Article, []domain.ArticleSource) (*domain.Article, error) {
	return nil, nil
}
func (f *fakeArticles) Find(context.Context, int64) (*domain.Article, error) { return f.article, nil }
func (f *fakeArticles) FindByUser(context.Context, int64, int64) (*domain.Article, error) {
	return f.article, nil
}
func (f *fakeArticles) FindBySlug(context.Context, int64, string) (*domain.Article, error) {
	return nil, nil
}
func (f *fakeArticles) ListByUser(context.Context, int64, int) ([]domain.Article, error) {
	return nil, nil
}
func (f *fakeArticles) ListSources(context.Context, int64) ([]domain.ArticleSource, error) {
	return f.sources, nil
}
func (f *fakeArticles) Update(context.Context, domain.Article) (*domain.Article, error) {
	return nil, nil
}

type fakeExports struct {
	byHash map[string]*domain.ObsidianExport
	nextID int64
}

type fakeREST struct {
	existing []byte
	writes   []restWrite
}

func (f *fakeREST) Enabled() bool { return true }

type restWrite struct {
	path    string
	content string
}

func (f *fakeREST) ReadNote(context.Context, string) ([]byte, error) {
	if f.existing == nil {
		return nil, ErrNoteNotFound
	}
	return f.existing, nil
}

func (f *fakeREST) WriteNote(_ context.Context, path string, content []byte) error {
	f.writes = append(f.writes, restWrite{path: path, content: string(content)})
	return nil
}

func (f *fakeExports) Create(_ context.Context, export domain.ObsidianExport) (*domain.ObsidianExport, error) {
	if f.byHash == nil {
		f.byHash = make(map[string]*domain.ObsidianExport)
	}
	f.nextID++
	export.ID = f.nextID
	export.ExportedAt = time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	f.byHash[export.ContentHash] = &export
	return &export, nil
}

func (f *fakeExports) FindByContentHash(_ context.Context, hash string) (*domain.ObsidianExport, error) {
	if f.byHash == nil {
		return nil, nil
	}
	return f.byHash[hash], nil
}
