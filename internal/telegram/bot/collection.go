package bot

import (
	"context"
	"fmt"

	"github.com/kirilllebedenko/content_scout/internal/collection"
	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/summary"
)

type CollectionController interface {
	CollectGroup(ctx context.Context, req collection.Request) (*collection.Result, error)
}

type SummaryController interface {
	GenerateFromCollection(ctx context.Context, req summary.GenerateRequest) (*summary.GenerateResult, error)
}

func collectionResultText(result *collection.Result) string {
	if result == nil {
		return "Сбор сообщений не выполнен."
	}
	return fmt.Sprintf("Сбор сообщений завершен.\n\nJob: %d\nИсточников: %d\nСообщений: %d", result.JobID, result.ChatsCount, result.MessagesCount)
}

func summaryResultText(result *summary.GenerateResult) string {
	if result == nil {
		return "Summary не создано."
	}
	return fmt.Sprintf("Summary создано.\n\nSummary: %d\nJob: %d\nТем: %d\nСообщений после фильтрации: %d\nДубликатов удалено: %d", result.SummaryID, result.SummaryJobID, result.TopicsCount, result.MessagesCount, result.DuplicateCount)
}

func parseCollectionMode(value string) domain.CollectionMode {
	if value == "" {
		return domain.CollectionModeNewOnly
	}
	return domain.CollectionMode(value)
}
