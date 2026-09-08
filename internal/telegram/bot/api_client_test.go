package bot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/summary"
)

func TestGenerateFromCollectionQueuesTaskAndPollsUntilCompleted(t *testing.T) {
	var paths []string
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/summaries/from-collection/7/tasks":
			var body summaryAPIRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode task request: %v", err)
			}
			if body.TelegramUserID != 42 {
				t.Errorf("telegram_user_id = %d, want 42", body.TelegramUserID)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"job_id":11,"status":"pending"}`))
		case "/jobs/11":
			if got := r.URL.Query().Get("telegram_user_id"); got != "42" {
				t.Errorf("poll telegram_user_id = %q, want \"42\"", got)
			}
			polls++
			if polls < 2 {
				_, _ = w.Write([]byte(`{"id":11,"status":"running","artifacts":{}}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":11,"status":"completed","artifacts":{"summary_id":5,"summary_job_id":6,"topics_count":3,"messages_count":81,"duplicate_count":2}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, "token", server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.GenerateFromCollection(ctx, summary.GenerateRequest{
		TelegramUserID:  42,
		CollectionJobID: 7,
		Format:          "standard",
	})
	if err != nil {
		t.Fatalf("GenerateFromCollection returned error: %v", err)
	}
	if result.SummaryID != 5 || result.SummaryJobID != 6 {
		t.Fatalf("summary ids = (%d, %d), want (5, 6)", result.SummaryID, result.SummaryJobID)
	}
	if result.TopicsCount != 3 || result.MessagesCount != 81 || result.DuplicateCount != 2 {
		t.Fatalf("counts = (%d, %d, %d), want (3, 81, 2)", result.TopicsCount, result.MessagesCount, result.DuplicateCount)
	}
	if polls != 2 {
		t.Fatalf("polled %d times, want 2", polls)
	}
	if paths[0] != "POST /summaries/from-collection/7/tasks" {
		t.Fatalf("first call was %q, want the task endpoint", paths[0])
	}
}

func TestGenerateFromCollectionReportsJobFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/summaries/from-collection/7/tasks" {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"job_id":11,"status":"pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":11,"status":"failed","last_error":"summarize with llm: context deadline exceeded","artifacts":{}}`))
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, "token", server.Client())
	_, err := client.GenerateFromCollection(context.Background(), summary.GenerateRequest{
		TelegramUserID:  42,
		CollectionJobID: 7,
	})
	if err == nil {
		t.Fatal("expected an error for a failed job")
	}
	if err.Error() != "summarize with llm: context deadline exceeded" {
		t.Fatalf("error = %q, want the job's last_error", err.Error())
	}
}
