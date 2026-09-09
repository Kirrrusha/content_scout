package llm

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
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

func TestSplitSummaryMessagesTruncatesOversizedMessage(t *testing.T) {
	messages := []SummaryMessageInput{
		{Index: 0, Text: strings.Repeat("я", 100)},
		{Index: 1, Text: strings.Repeat("a", 5000)},
	}
	batches := splitSummaryMessages(messages, 101)
	if len(batches) != 2 {
		t.Fatalf("got %d batches, want 2", len(batches))
	}
	for _, batch := range batches {
		if len(batch.messages) != 1 {
			t.Fatalf("batch holds %d messages, want 1", len(batch.messages))
		}
		if got := len(batch.messages[0].Text) + len(batch.messages[0].ChatTitle); got > 101 {
			t.Fatalf("batch message is %d bytes, want at most 101", got)
		}
		if !utf8.ValidString(batch.messages[0].Text) {
			t.Fatal("truncated message is not valid UTF-8")
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
		if strings.Contains(system, "семантически одинаковые темы") {
			if strings.Contains(request.Messages[1].Content, strings.Repeat("a", 1000)) {
				t.Error("topic reduce request repeats original message text")
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"merges\":[]}"}}]}`))
			return
		}
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
	client.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	// Two messages fill one batch exactly, so six messages make three batches
	// whatever the configured budget is.
	result, err := client.Summarize(context.Background(), SummaryInput{
		Language: "ru",
		Format:   "standard",
		Messages: messagesOfSize(6, maxBatchContentBytes/2),
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
	if calls != 5 {
		t.Fatalf("made %d requests, want 3 batches plus topic reduce and headline", calls)
	}
}

func TestApplyTopicMergesCombinesDuplicatesAndSourceIndexes(t *testing.T) {
	topics := []SummaryTopicResult{
		{Title: "Kimi вышла", Category: "AI", ShortSummary: "Новая версия", FullSummary: "Первая деталь", WhyImportant: "Влияние", Confidence: "high", Importance: 6, SourceIndexes: []int{4, 1, 4}},
		{Title: "Релиз Kimi", Category: "AI", ShortSummary: "Обновление модели", FullSummary: "Вторая деталь", WhyImportant: "Влияние", Confidence: "medium", Importance: 8, SourceIndexes: []int{2, 1}},
		{Title: "Погода", Category: "Природа", ShortSummary: "Шторм", FullSummary: "Независимый сюжет", Confidence: "high", Importance: 5, SourceIndexes: []int{9}},
	}

	got := applyTopicMerges(topics, []topicMerge{{
		TopicIndexes: []int{1, 0, 1},
		SharedAnchor: "Kimi",
		Title:        "Релиз новой версии Kimi",
		ShortSummary: "Модель получила обновление.",
	}})
	if len(got) != 2 {
		t.Fatalf("topics = %d, want merged duplicate plus independent topic", len(got))
	}
	if got[0].Title != "Релиз новой версии Kimi" || got[0].ShortSummary != "Модель получила обновление." {
		t.Fatalf("merged topic = %+v", got[0])
	}
	if got[0].Confidence != "medium" || got[0].Importance != 8 {
		t.Fatalf("merged confidence/importance = %s/%d, want medium/8", got[0].Confidence, got[0].Importance)
	}
	if indexes := got[0].SourceIndexes; len(indexes) != 3 || indexes[0] != 1 || indexes[1] != 2 || indexes[2] != 4 {
		t.Fatalf("merged source indexes = %v, want [1 2 4]", indexes)
	}
	if got[1].Title != "Погода" || len(got[1].SourceIndexes) != 1 || got[1].SourceIndexes[0] != 9 {
		t.Fatalf("independent topic changed: %+v", got[1])
	}
}

func TestApplyTopicMergesIgnoresInvalidAndOverlappingGroups(t *testing.T) {
	topics := []SummaryTopicResult{
		{Title: "A", SourceIndexes: []int{0}},
		{Title: "A duplicate", SourceIndexes: []int{1}},
		{Title: "Independent", SourceIndexes: []int{2}},
	}
	got := applyTopicMerges(topics, []topicMerge{
		{TopicIndexes: []int{0, 1}, Title: "A merged"},
		{TopicIndexes: []int{1, 2}, Title: "overlap"},
		{TopicIndexes: []int{2, 99}, Title: "out of range"},
	})
	if len(got) != 2 || got[0].Title != "A merged" || got[1].Title != "Independent" {
		t.Fatalf("partially invalid merges produced %+v", got)
	}
}

func TestSplitReduceTopicsBoundsDescriptorPayload(t *testing.T) {
	topics := make([]SummaryTopicResult, 10)
	for i := range topics {
		topics[i] = SummaryTopicResult{Title: strings.Repeat("т", 500), ShortSummary: strings.Repeat("я", 1000)}
	}
	order := make([]int, len(topics))
	for i := range order {
		order[i] = i
	}
	chunks := splitReduceTopics(topics, order, 2500)
	if len(chunks) < 2 {
		t.Fatalf("chunks = %d, want bounded split", len(chunks))
	}
	for _, chunk := range chunks {
		size := 0
		for _, index := range chunk {
			size += len(mustJSON(compactReduceTopic(index, topics[index]))) + 1
		}
		if size > 2500 {
			t.Fatalf("reduce chunk size = %d, want <= 2500", size)
		}
	}
}

func TestMergeSummariesUsesSemanticReduceWithoutOriginalMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request chatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		var content string
		if strings.Contains(request.Messages[0].Content, "семантически одинаковые темы") {
			if strings.Contains(request.Messages[1].Content, "SECRET ORIGINAL MESSAGE") {
				t.Fatal("reduce request contains original message text")
			}
			if thinking, ok := request.ChatTemplateKwargs["thinking"].(bool); !ok || thinking || request.Temperature != nil {
				t.Fatalf("Kimi reduce is not in instant mode: kwargs=%#v temperature=%v", request.ChatTemplateKwargs, request.Temperature)
			}
			if request.MaxCompletionTokens != maxReduceCompletionTokens {
				t.Fatalf("reduce completion tokens = %d, want %d", request.MaxCompletionTokens, maxReduceCompletionTokens)
			}
			content = `{"merges":[{"topic_indexes":[0,1],"shared_anchor":"Kimi","title":"Один релиз","short_summary":"Два батча описывают один релиз."}]}`
		} else {
			content = `{"title":"Общий заголовок","overview":"Общий обзор"}`
		}
		_ = json.NewEncoder(w).Encode(chatResponse{Choices: []struct {
			Message chatMessage `json:"message"`
		}{{Message: chatMessage{Content: content}}}})
	}))
	defer server.Close()

	client := NewOpenAICompatible(server.URL, "key", "moonshotai/Kimi-K2.6", server.Client())
	partials := []*SummaryResult{
		{Title: "B1", Overview: "O1", Topics: []SummaryTopicResult{{Title: "Kimi: версия вышла", Category: "AI", ShortSummary: "Компания выпустила модель Kimi", FullSummary: "Деталь 1", Confidence: "high", Importance: 6, SourceIndexes: []int{0, 1}}}},
		{Title: "B2", Overview: "O2", Topics: []SummaryTopicResult{{Title: "Новая версия Kimi", Category: "AI", ShortSummary: "Состоялся релиз Kimi", FullSummary: "Деталь 2", Confidence: "medium", Importance: 7, SourceIndexes: []int{1, 4}}}},
	}
	result, err := client.mergeSummaries(context.Background(), SummaryInput{Messages: []SummaryMessageInput{{Text: "SECRET ORIGINAL MESSAGE"}}}, partials)
	if err != nil {
		t.Fatalf("mergeSummaries() error = %v", err)
	}
	if len(result.Topics) != 1 || result.Topics[0].Title != "Один релиз" {
		t.Fatalf("topics = %+v, want one semantic merge", result.Topics)
	}
	if got := result.Topics[0].SourceIndexes; len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 4 {
		t.Fatalf("source indexes = %v, want [0 1 4]", got)
	}
}

func TestHasConcreteSharedAnchorRejectsBroadOrMissingConnection(t *testing.T) {
	topics := []SummaryTopicResult{
		{Title: "Фанфики и мёртвый интернет", ShortSummary: "Сатирический пост о фанфиках"},
		{Title: "Индийский Человек-паук", ShortSummary: "Сатирический пост о кино"},
		{Title: "ChatGPT заблокирован в России", ShortSummary: "Доступ к ChatGPT ограничен"},
		{Title: "ChatGPT снова доступен", ShortSummary: "Блокировка ChatGPT снята"},
	}
	if hasConcreteSharedAnchor(topics, topicMerge{TopicIndexes: []int{0, 1}, SharedAnchor: "сатирический пост"}) {
		t.Fatal("broad stylistic anchor must not merge unrelated posts")
	}
	if hasConcreteSharedAnchor(topics, topicMerge{TopicIndexes: []int{0, 1}, SharedAnchor: "фанфики"}) {
		t.Fatal("anchor missing from one topic must be rejected")
	}
	if !hasConcreteSharedAnchor(topics, topicMerge{TopicIndexes: []int{2, 3}, SharedAnchor: "ChatGPT"}) {
		t.Fatal("shared concrete product anchor should be accepted")
	}
}

func TestMergeSummariesFallsBackWhenReduceFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary failure", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client := NewOpenAICompatible(server.URL, "key", "model", server.Client())
	client.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	partials := []*SummaryResult{{
		Title: "Fallback title", Overview: "Fallback overview",
		Topics: []SummaryTopicResult{
			{Title: "A", ShortSummary: "A short", FullSummary: "A full", Confidence: "high", Importance: 5, SourceIndexes: []int{0}},
			{Title: "B", ShortSummary: "B short", FullSummary: "B full", Confidence: "medium", Importance: 4, SourceIndexes: []int{1}},
		},
	}}
	result, err := client.mergeSummaries(context.Background(), SummaryInput{}, partials)
	if err != nil {
		t.Fatalf("mergeSummaries() error = %v, want usable fallback", err)
	}
	if result.Title != "Fallback title" || result.Overview != "Fallback overview" || len(result.Topics) != 2 {
		t.Fatalf("fallback result = %+v", result)
	}
}

func TestSummarizeBatchesKeepsSuccessfulBatchAfterPartialFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request chatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		system := request.Messages[0].Content
		if strings.Contains(system, "семантически одинаковые темы") {
			writeChatContent(t, w, `{"merges":[]}`)
			return
		}
		if strings.Contains(system, "общий заголовок") {
			writeChatContent(t, w, `{"title":"Reduced","overview":"Reduced overview"}`)
			return
		}
		var input SummaryInput
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &input); err != nil {
			t.Fatalf("decode input: %v", err)
		}
		if strings.HasPrefix(input.Messages[0].Text, "f") {
			http.Error(w, "bad batch", http.StatusBadRequest)
			return
		}
		writeChatContent(t, w, `{"title":"Batch","overview":"Batch overview","topics":[{"title":"Survivor","short_summary":"Short","full_summary":"Full","confidence":"high","importance":5,"source_indexes":[0,1]}]}`)
	}))
	defer server.Close()
	client := NewOpenAICompatible(server.URL, "key", "model", server.Client())
	client.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	result, err := client.Summarize(context.Background(), SummaryInput{Messages: []SummaryMessageInput{
		{Text: strings.Repeat("f", maxBatchContentBytes/2)},
		{Text: strings.Repeat("x", maxBatchContentBytes/2)},
		{Text: strings.Repeat("y", maxBatchContentBytes/2)},
		{Text: strings.Repeat("y", maxBatchContentBytes/2)},
	}})
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	if len(result.Topics) != 1 {
		t.Fatalf("topics = %d, want successful batch fallback", len(result.Topics))
	}
	if got := result.Topics[0].SourceIndexes; len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("source indexes = %v, want surviving global indexes [2 3]", got)
	}
}

func writeChatContent(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(chatResponse{Choices: []struct {
		Message chatMessage `json:"message"`
	}{{Message: chatMessage{Content: content}}}}); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func TestSummarizeSendsOneRequestForSmallInput(t *testing.T) {
	var calls int32
	var mu sync.Mutex
	server := batchTestServer(t, &calls, &mu)
	defer server.Close()

	client := NewOpenAICompatible(server.URL, "key", "model", server.Client())
	client.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
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
