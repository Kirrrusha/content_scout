package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/telegram/tdlib"
)

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
	if len(chats) == 0 {
		return "Чаты пока не синхронизированы."
	}
	var builder strings.Builder
	builder.WriteString("Каналы и группы:\n\n")
	for i, chat := range chats {
		if i >= 30 {
			builder.WriteString("\nСписок обрезан до 30 чатов.")
			break
		}
		muted := ""
		if chat.IsMuted {
			muted = ", muted"
		}
		archived := ""
		if chat.IsArchived {
			archived = ", archive"
		}
		fmt.Fprintf(&builder, "%d. %s [%s, unread: %d%s%s]\n", i+1, chat.Title, chat.Type, chat.UnreadCount, muted, archived)
	}
	return builder.String()
}
