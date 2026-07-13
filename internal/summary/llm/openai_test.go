package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
