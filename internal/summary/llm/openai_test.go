package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompatibleSummarize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		var request chatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(request.Messages) != 2 {
			t.Fatalf("messages len = %d", len(request.Messages))
		}
		if !strings.Contains(request.Messages[0].Content, "разбей весь набор сообщений на самостоятельные темы") {
			t.Fatalf("system prompt does not require topic clustering: %q", request.Messages[0].Content)
		}
		if !strings.Contains(request.Messages[0].Content, "не ограничивайся заранее заданной темой") {
			t.Fatalf("system prompt still looks single-topic oriented: %q", request.Messages[0].Content)
		}
		_ = json.NewEncoder(w).Encode(chatResponse{Choices: []struct {
			Message chatMessage `json:"message"`
		}{{Message: chatMessage{Role: "assistant", Content: `{"title":"Digest","overview":"Overview","topics":[{"title":"Topic","category":"Go","short_summary":"Short","full_summary":"Full","why_important":"Important","confidence":"medium","importance":7,"source_indexes":[0]}]}`}}}})
	}))
	defer server.Close()

	result, err := NewOpenAICompatible(server.URL, "key", "model", server.Client()).Summarize(context.Background(), SummaryInput{
		Language: "ru",
		Format:   "standard",
		Messages: []SummaryMessageInput{{Index: 0, Text: "Message"}},
	})
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	if result.Title != "Digest" || len(result.Topics) != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestOpenAICompatibleConvertToArticle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(chatResponse{Choices: []struct {
			Message chatMessage `json:"message"`
		}{{Message: chatMessage{Role: "assistant", Content: `{"title":"Go Guide","type":"guide","tags":["go"],"content_markdown":"# Go Guide\n\n## Источники"}`}}}})
	}))
	defer server.Close()

	result, err := NewOpenAICompatible(server.URL, "key", "model", server.Client()).ConvertToArticle(context.Background(), ArticleInput{
		Language:   "ru",
		Type:       "guide",
		SourceKind: "summary_topic",
		Summary:    "# Digest",
	})
	if err != nil {
		t.Fatalf("ConvertToArticle() error = %v", err)
	}
	if result.Title != "Go Guide" || result.Type != "guide" || len(result.Tags) != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestOpenAICompatibleIncludesErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	}))
	defer server.Close()

	_, err := NewOpenAICompatible(server.URL, "key", "model", server.Client()).Summarize(context.Background(), SummaryInput{
		Language: "ru",
		Format:   "standard",
		Messages: []SummaryMessageInput{{Index: 0, Text: "Message"}},
	})
	if err == nil {
		t.Fatal("Summarize() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "path=/chat/completions") || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q", err)
	}
}
