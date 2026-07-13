package deduplicator

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/kirilllebedenko/content_scout/internal/summary/filter"
)

type Cluster struct {
	ID        int
	Canonical filter.Message
	Messages  []filter.Message
	Reasons   []string
}

type Stats struct {
	InputCount      int
	ClusterCount    int
	DuplicateCount  int
	ExactCount      int
	URLCount        int
	SimilarityCount int
}

type Deduplicator struct {
	SimilarityThreshold float64
}

func New() *Deduplicator {
	return &Deduplicator{SimilarityThreshold: 0.80}
}

func (d *Deduplicator) GroupDuplicates(_ context.Context, messages []filter.Message) ([]Cluster, Stats, error) {
	threshold := d.SimilarityThreshold
	if threshold <= 0 {
		threshold = 0.80
	}
	stats := Stats{InputCount: len(messages)}
	clusters := make([]Cluster, 0, len(messages))

	for _, message := range messages {
		index, reason := d.findCluster(clusters, message, threshold)
		if index == -1 {
			clusters = append(clusters, Cluster{
				ID:        len(clusters) + 1,
				Canonical: message,
				Messages:  []filter.Message{message},
			})
			continue
		}
		clusters[index].Messages = append(clusters[index].Messages, message)
		clusters[index].Reasons = appendUnique(clusters[index].Reasons, reason)
		switch reason {
		case "exact":
			stats.ExactCount++
		case "url":
			stats.URLCount++
		case "similarity":
			stats.SimilarityCount++
		}
	}

	for i := range clusters {
		sort.SliceStable(clusters[i].Messages, func(left, right int) bool {
			return clusters[i].Messages[left].Source.Date.Before(clusters[i].Messages[right].Source.Date)
		})
		clusters[i].Canonical = clusters[i].Messages[0]
		if len(clusters[i].Messages) > 1 {
			stats.DuplicateCount += len(clusters[i].Messages) - 1
		}
	}
	stats.ClusterCount = len(clusters)
	return clusters, stats, nil
}

func (d *Deduplicator) findCluster(clusters []Cluster, message filter.Message, threshold float64) (int, string) {
	messageTokens := tokenSet(message.Content)
	for i, cluster := range clusters {
		if cluster.Canonical.ContentHash == message.ContentHash {
			return i, "exact"
		}
		if shareURL(cluster.Canonical.URLs, message.URLs) {
			return i, "url"
		}
		if jaccard(messageTokens, tokenSet(cluster.Canonical.Content)) >= threshold {
			return i, "similarity"
		}
	}
	return -1, ""
}

func shareURL(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; ok {
			return true
		}
	}
	return false
}

var tokenPattern = regexp.MustCompile(`[\p{L}\p{N}]+`)

func tokenSet(content string) map[string]struct{} {
	content = regexp.MustCompile(`https?://[^\s)>\]]+|t\.me/[^\s)>\]]+`).ReplaceAllString(content, " ")
	raw := tokenPattern.FindAllString(strings.ToLower(content), -1)
	tokens := make(map[string]struct{}, len(raw))
	for _, token := range raw {
		if len([]rune(token)) < 3 {
			continue
		}
		tokens[token] = struct{}{}
	}
	return tokens
}

func jaccard(left, right map[string]struct{}) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	intersection := 0
	for token := range left {
		if _, ok := right[token]; ok {
			intersection++
		}
	}
	union := len(left) + len(right) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
