package postgres

import (
	"database/sql"
	"testing"

	"github.com/kirilllebedenko/content_scout/internal/storage"
)

func TestRepositoriesImplementInterfaces(t *testing.T) {
	var db *sql.DB

	var _ storage.UserRepository = NewUserRepository(db)
	var _ storage.TelegramSessionRepository = NewTelegramSessionRepository(db)
	var _ storage.TelegramFolderRepository = NewTelegramFolderRepository(db)
	var _ storage.TelegramChatRepository = NewTelegramChatRepository(db)
	var _ storage.SourceGroupRepository = NewSourceGroupRepository(db)
	var _ storage.ReadPositionRepository = NewReadPositionRepository(db)
	var _ storage.MessageCollectionRepository = NewMessageCollectionRepository(db)
	var _ storage.SummaryRepository = NewSummaryRepository(db)
	var _ storage.ArticleRepository = NewArticleRepository(db)
	var _ storage.ObsidianExportRepository = NewObsidianExportRepository(db)
	var _ storage.SummaryScheduleRepository = NewSummaryScheduleRepository(db)
}
