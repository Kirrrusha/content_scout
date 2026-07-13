package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type Summarizer interface {
	Summarize(ctx context.Context, input SummaryInput) (*SummaryResult, error)
	ConvertToArticle(ctx context.Context, input ArticleInput) (*ArticleResult, error)
}

type SummaryInput struct {
	Language string                `json:"language"`
	Format   string                `json:"format"`
	Messages []SummaryMessageInput `json:"messages"`
}

type SummaryMessageInput struct {
	Index       int      `json:"index"`
	ChatTitle   string   `json:"chat_title"`
	PublishedAt string   `json:"published_at"`
	Text        string   `json:"text"`
	URLs        []string `json:"urls"`
}

type SummaryResult struct {
	Title          string               `json:"title"`
	Overview       string               `json:"overview"`
	Topics         []SummaryTopicResult `json:"topics"`
	FilteredCount  int                  `json:"filtered_count,omitempty"`
	DuplicateCount int                  `json:"duplicate_count,omitempty"`
}

type SummaryTopicResult struct {
	Title         string `json:"title"`
	Category      string `json:"category"`
	ShortSummary  string `json:"short_summary"`
	FullSummary   string `json:"full_summary"`
	WhyImportant  string `json:"why_important"`
	Confidence    string `json:"confidence"`
	Importance    int    `json:"importance"`
	SourceIndexes []int  `json:"source_indexes"`
}

type ArticleInput struct {
	Language   string               `json:"language"`
	Type       string               `json:"type"`
	Title      string               `json:"title,omitempty"`
	Tags       []string             `json:"tags,omitempty"`
	SourceKind string               `json:"source_kind"`
	Summary    string               `json:"summary,omitempty"`
	Topic      *ArticleTopicInput   `json:"topic,omitempty"`
	Sources    []ArticleSourceInput `json:"sources,omitempty"`
}

type ArticleTopicInput struct {
	Title        string `json:"title"`
	Category     string `json:"category"`
	ShortSummary string `json:"short_summary"`
	FullSummary  string `json:"full_summary"`
}

type ArticleSourceInput struct {
	Title       string `json:"title"`
	URL         string `json:"url,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
	Text        string `json:"text,omitempty"`
}

type ArticleResult struct {
	Title           string   `json:"title"`
	Type            string   `json:"type"`
	Tags            []string `json:"tags"`
	ContentMarkdown string   `json:"content_markdown"`
}

func ParseSummaryResult(raw []byte) (*SummaryResult, error) {
	var result SummaryResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse summary json: %w", err)
	}
	if err := ValidateSummaryResult(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func ParseArticleResult(raw []byte) (*ArticleResult, error) {
	var result ArticleResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse article json: %w", err)
	}
	if err := ValidateArticleResult(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func ValidateSummaryResult(result *SummaryResult) error {
	if result == nil {
		return errors.New("summary result is nil")
	}
	if result.Title == "" {
		return errors.New("summary title is required")
	}
	if result.Overview == "" {
		return errors.New("summary overview is required")
	}
	if len(result.Topics) == 0 {
		return errors.New("summary topics are required")
	}
	for i, topic := range result.Topics {
		if topic.Title == "" || topic.ShortSummary == "" || topic.FullSummary == "" {
			return fmt.Errorf("topic %d is incomplete", i)
		}
		switch topic.Confidence {
		case "high", "medium", "low":
		default:
			return fmt.Errorf("topic %d has invalid confidence %q", i, topic.Confidence)
		}
		if topic.Importance < 1 || topic.Importance > 10 {
			return fmt.Errorf("topic %d has invalid importance %d", i, topic.Importance)
		}
	}
	return nil
}

func ValidateArticleResult(result *ArticleResult) error {
	if result == nil {
		return errors.New("article result is nil")
	}
	if result.Title == "" {
		return errors.New("article title is required")
	}
	if result.ContentMarkdown == "" {
		return errors.New("article content_markdown is required")
	}
	switch result.Type {
	case "educational", "guide", "analysis", "outline", "telegram_post":
	default:
		return fmt.Errorf("article has invalid type %q", result.Type)
	}
	return nil
}
