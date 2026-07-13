package pipeline

import (
	"context"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/summary/deduplicator"
	"github.com/kirilllebedenko/content_scout/internal/summary/filter"
)

type Result struct {
	Messages []filter.Message
	Clusters []deduplicator.Cluster
	Stats    Stats
}

type Stats struct {
	InputMessages    int
	KeptMessages     int
	NoiseRemoved     int
	DuplicateRemoved int
	UniqueClusters   int
	Filter           filter.Stats
	Deduplication    deduplicator.Stats
}

type Pipeline struct {
	filter       *filter.Filter
	deduplicator *deduplicator.Deduplicator
}

func New() *Pipeline {
	return &Pipeline{
		filter:       filter.New(),
		deduplicator: deduplicator.New(),
	}
}

func (p *Pipeline) Process(ctx context.Context, messages []domain.CollectedMessage, rules filter.Rules) (*Result, error) {
	filtered, filterStats, err := p.filter.Filter(ctx, messages, rules)
	if err != nil {
		return nil, err
	}
	clusters, dedupStats, err := p.deduplicator.GroupDuplicates(ctx, filtered)
	if err != nil {
		return nil, err
	}
	return &Result{
		Messages: filtered,
		Clusters: clusters,
		Stats: Stats{
			InputMessages:    len(messages),
			KeptMessages:     len(filtered),
			NoiseRemoved:     len(messages) - len(filtered),
			DuplicateRemoved: dedupStats.DuplicateCount,
			UniqueClusters:   dedupStats.ClusterCount,
			Filter:           filterStats,
			Deduplication:    dedupStats,
		},
	}, nil
}
