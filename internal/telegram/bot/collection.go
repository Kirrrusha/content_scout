package bot

import (
	"context"
	"fmt"

	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type CollectionController interface {
	CollectGroup(ctx context.Context, req collection.Request) (*collection.Result, error)
}

func collectionResultText(result *collection.Result) string {
	if result == nil {
		return "Сбор сообщений не выполнен."
	}
	return fmt.Sprintf("Сбор сообщений завершен.\n\nJob: %d\nИсточников: %d\nСообщений: %d", result.JobID, result.ChatsCount, result.MessagesCount)
}

func parseCollectionMode(value string) domain.CollectionMode {
	if value == "" {
		return domain.CollectionModeNewOnly
	}
	return domain.CollectionMode(value)
}
