package bot

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

const chatsPageSize = 30

type SyncController interface {
	Sync(ctx context.Context, telegramUserID int64) (*tdlib.SyncResult, error)
	ListFolders(ctx context.Context, telegramUserID int64) ([]domain.TelegramFolder, error)
	ListChats(ctx context.Context, telegramUserID int64) ([]domain.TelegramChat, error)
}

func syncResultText(result *tdlib.SyncResult) string {
	if result == nil {
		return "Синхронизация не выполнена."
	}
	return fmt.Sprintf("Синхронизация завершена.\n\nПапок: %d\nЧатов: %d", result.FoldersCount, result.ChatsCount)
}

func foldersText(folders []domain.TelegramFolder) string {
	if len(folders) == 0 {
		return "Папки Telegram пока не синхронизированы."
	}
	var builder strings.Builder
	builder.WriteString("Папки Telegram:\n\n")
	for i, folder := range folders {
		if i >= 30 {
			builder.WriteString("\nСписок обрезан до 30 папок.")
			break
		}
		fmt.Fprintf(&builder, "%d. %s\n", i+1, folder.Name)
	}
	return builder.String()
}

func chatsText(chats []domain.TelegramChat) string {
	return chatsPageText(chats, 0)
}

func chatsPageText(chats []domain.TelegramChat, page int) string {
	if len(chats) == 0 {
		return "Чаты пока не синхронизированы."
	}
	page = clampChatsPage(page, len(chats))
	start := page * chatsPageSize
	end := min(start+chatsPageSize, len(chats))
	var builder strings.Builder
	fmt.Fprintf(&builder, "Каналы и группы: %d-%d из %d\n\n", start+1, end, len(chats))
	for i, chat := range chats[start:end] {
		muted := ""
		if chat.IsMuted {
			muted = ", muted"
		}
		archived := ""
		if chat.IsArchived {
			archived = ", archive"
		}
		fmt.Fprintf(&builder, "%d. %s [ID: %d, %s, unread: %d%s%s]\n", start+i+1, chat.Title, chat.ID, chat.Type, chat.UnreadCount, muted, archived)
	}
	builder.WriteString("\nДля выбора источника используйте ID из строки, например: /group_add_chat <group_id> <chat_id>.")
	if len(chats) > chatsPageSize {
		fmt.Fprintf(&builder, "\nСтраница %d/%d.", page+1, chatsPages(len(chats)))
	}
	return builder.String()
}

func chatsPages(total int) int {
	if total <= 0 {
		return 1
	}
	return int(math.Ceil(float64(total) / float64(chatsPageSize)))
}

func clampChatsPage(page, total int) int {
	pages := chatsPages(total)
	if page < 0 {
		return 0
	}
	if page >= pages {
		return pages - 1
	}
	return page
}
