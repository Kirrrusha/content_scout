package filter

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

func TestFilterNormalizesAndRemovesNoise(t *testing.T) {
	messages := []domain.CollectedMessage{
		{MessageID: 1, Text: "  Go 1.25 вышел   сегодня\n\nПодписаться: @channel  ", Date: time.Now()},
		{MessageID: 2, Text: "🔥🔥🔥", Date: time.Now()},
		{MessageID: 3, Text: "Реклама: скидка на курс Go", Date: time.Now()},
		{MessageID: 4, Text: "Вакансия: ищем разработчика Go, зарплата высокая", Date: time.Now()},
		{MessageID: 5, Text: "ok", Date: time.Now()},
	}

	filtered, stats, err := New().Filter(context.Background(), messages, Rules{
		MinTextLength: 10,
		DropAds:       true,
		DropJobs:      true,
	})
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1", len(filtered))
	}
	if strings.Contains(filtered[0].Content, "Подписаться") {
		t.Fatalf("footer was not removed: %q", filtered[0].Content)
	}
	if stats.FooterRemovedCount != 1 || stats.EmojiOnlyCount != 1 || stats.AdvertisementCount != 1 || stats.JobPostCount != 1 || stats.TooShortCount != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestFilterKeepsShortMessageWithURL(t *testing.T) {
	messages := []domain.CollectedMessage{{MessageID: 1, Text: "Новость https://example.com/x", Date: time.Now()}}

	filtered, stats, err := New().Filter(context.Background(), messages, Rules{MinTextLength: 100})
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}
	if len(filtered) != 1 || len(filtered[0].URLs) != 1 {
		t.Fatalf("filtered = %+v stats = %+v", filtered, stats)
	}
}
