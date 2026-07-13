package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/summary/filter"
)

func TestPipelineFiltersAndDeduplicates(t *testing.T) {
	result, err := New().Process(context.Background(), []domain.CollectedMessage{
		{MessageID: 1, Text: "Go team published a detailed compiler performance update https://example.com/go", Date: time.Now()},
		{MessageID: 2, Text: "Repost https://example.com/go", Date: time.Now()},
		{MessageID: 3, Text: "🔥🔥🔥", Date: time.Now()},
	}, filter.Rules{MinTextLength: 10})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if result.Stats.InputMessages != 3 || result.Stats.KeptMessages != 2 || result.Stats.NoiseRemoved != 1 {
		t.Fatalf("stats = %+v", result.Stats)
	}
	if result.Stats.UniqueClusters != 1 || result.Stats.DuplicateRemoved != 1 {
		t.Fatalf("dedup stats = %+v", result.Stats)
	}
}
