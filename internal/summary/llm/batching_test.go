package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func messagesOfSize(count, size int) []SummaryMessageInput {
	messages := make([]SummaryMessageInput, 0, count)
	for i := range count {
		messages = append(messages, SummaryMessageInput{Index: i, Text: strings.Repeat("a", size)})
	}
	return messages
}

func TestSplitSummaryMessagesRenumbersAndKeepsGlobalIndexes(t *testing.T) {
	batches := splitSummaryMessages(messagesOfSize(5, 100), 250)
	if len(batches) != 3 {
		t.Fatalf("got %d batches, want 3", len(batches))
	}
	var seen []int
	for _, batch := range batches {
		for i, message := range batch.messages {
			if message.Index != i {
				t.Fatalf("message index = %d, want %d after renumbering", message.Index, i)
			}
		}
		seen = append(seen, batch.globalIndexes...)
	}
	for i, global := range seen {
		if global != i {
			t.Fatalf("global index %d = %d, want %d", i, global, i)
		}
	}
}

func TestSplitSummaryMessagesKeepsOversizedMessage(t *testing.T) {
	batches := splitSummaryMessages(messagesOfSize(2, 5000), 100)
	if len(batches) != 2 {
		t.Fatalf("got %d batches, want 2", len(batches))
	}
	for _, batch := range batches {
		if len(batch.messages) != 1 {
			t.Fatalf("batch holds %d messages, want 1", len(batch.messages))
		}
	}
}

func TestRemapSourceIndexesTranslatesAndDropsOutOfRange(t *testing.T) {
	result := &SummaryResult{Topics: []SummaryTopicResult{{SourceIndexes: []int{0, 2, 9, -1}}}}
	remapSourceIndexes(result, []int{7, 8, 9})
	got := result.Topics[0].SourceIndexes
	if len(got) != 2 || got[0] != 7 || got[1] != 9 {
		t.Fatalf("source indexes = %v, want [7 9]", got)
	}
}

// batchTestServer answers every summarize request with one topic naming the
// batch's own message text, and answers the headline request separately.
func batchTestServer(t *testing.T, calls *int32, mu *sync.Mutex) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		*calls++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		system := request.Messages[0].Content
		if strings.Contains(system, "общий заголовок") {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"title\":\"Общий заголовок\",\"overview\":\"Общий обзор\"}"}}]}`))
			return
		}
		var input SummaryInput
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &input); err != nil {
			t.Errorf("decode summary input: %v", err)
		}
		indexes := make([]int, 0, len(input.Messages))
		for _, message := range input.Messages {
			indexes = append(indexes, message.Index)
		}
		payload, err := json.Marshal(SummaryResult{
			Title:    "Batch",
			Overview: "Batch overview",
			Topics: []SummaryTopicResult{{
				Title:         "Topic",
				Category:      "News",
				ShortSummary:  "Short",
				FullSummary:   "Full",
				Confidence:    "high",
				Importance:    5,
				SourceIndexes: indexes,
			}},
		})
		if err != nil {
			t.Errorf("marshal batch result: %v", err)
		}
		response, err := json.Marshal(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": string(payload)}}},
		})
		if err != nil {
			t.Errorf("marshal response: %v", err)
		}
		_, _ = w.Write(response)
	}))
}

func TestSummarizeBatchesLargeInputAndMergesTopics(t *testing.T) {
	var calls int32
	var mu sync.Mutex
	server := batchTestServer(t, &calls, &mu)
	defer server.Close()

	client := NewOpenAICompatible(server.URL, "key", "model", server.Client())
	// 6 messages of 10000 bytes against a 20000 byte budget: 3 batches.
	result, err := client.Summarize(context.Background(), SummaryInput{
		Language: "ru",
		Format:   "standard",
		Messages: messagesOfSize(6, 10000),
	})
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	if len(result.Topics) != 3 {
		t.Fatalf("got %d topics, want one per batch (3)", len(result.Topics))
	}
	if result.Title != "Общий заголовок" || result.Overview != "Общий обзор" {
		t.Fatalf("headline = (%q, %q), want the reduced headline", result.Title, result.Overview)
	}
	var covered []int
	for _, topic := range result.Topics {
		covered = append(covered, topic.SourceIndexes...)
	}
	if len(covered) != 6 {
		t.Fatalf("source indexes cover %d messages, want 6", len(covered))
	}
	seen := make(map[int]bool, len(covered))
	for _, index := range covered {
		if index < 0 || index > 5 {
			t.Fatalf("source index %d is outside the original input", index)
		}
		if seen[index] {
			t.Fatalf("source index %d appears twice", index)
		}
		seen[index] = true
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 4 {
		t.Fatalf("made %d requests, want 3 batches plus 1 headline", calls)
	}
}

func TestSummarizeSendsOneRequestForSmallInput(t *testing.T) {
	var calls int32
	var mu sync.Mutex
	server := batchTestServer(t, &calls, &mu)
	defer server.Close()

	client := NewOpenAICompatible(server.URL, "key", "model", server.Client())
	if _, err := client.Summarize(context.Background(), SummaryInput{
		Language: "ru",
		Messages: messagesOfSize(2, 100),
	}); err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("made %d requests, want 1 for an input that fits in a batch", calls)
	}
}
