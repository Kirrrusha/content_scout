package deduplicator

import (
	"context"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/summary/filter"
)

func TestGroupDuplicatesByURLAndSimilarity(t *testing.T) {
	messages, _, err := filter.New().Filter(context.Background(), []domain.CollectedMessage{
		{MessageID: 1, Text: "Go release improves compiler performance https://example.com/go", Date: time.Now()},
		{MessageID: 2, Text: "Another repost https://example.com/go", Date: time.Now().Add(time.Second)},
		{MessageID: 3, Text: "Go release improves compiler performance significantly", Date: time.Now().Add(2 * time.Second)},
		{MessageID: 4, Text: "Rust release changes borrow checker diagnostics", Date: time.Now().Add(3 * time.Second)},
	}, filter.Rules{MinTextLength: 5})
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}

	clusters, stats, err := New().GroupDuplicates(context.Background(), messages)
	if err != nil {
		t.Fatalf("GroupDuplicates() error = %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("clusters len = %d, want 2: %+v", len(clusters), clusters)
	}
	if stats.DuplicateCount != 2 || stats.URLCount != 1 || stats.SimilarityCount != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}
