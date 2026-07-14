package httpserver

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type jobResponse struct {
	ID               int64            `json:"id"`
	Type             string           `json:"type"`
	Status           string           `json:"status"`
	Payload          json.RawMessage  `json:"payload"`
	Result           json.RawMessage  `json:"result"`
	Attempt          int              `json:"attempt"`
	MaxAttempts      int              `json:"max_attempts"`
	AvailableAt      time.Time        `json:"available_at"`
	LockedBy         *string          `json:"locked_by,omitempty"`
	LastError        *string          `json:"last_error,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	StartedAt        *time.Time       `json:"started_at,omitempty"`
	FinishedAt       *time.Time       `json:"finished_at,omitempty"`
	DeduplicationKey *string          `json:"deduplication_key,omitempty"`
	Artifacts        jobArtifactsView `json:"artifacts"`
}

type jobArtifactsView struct {
	CollectionJobID int64 `json:"collection_job_id,omitempty"`
	SummaryID      int64 `json:"summary_id,omitempty"`
	SummaryJobID   int64 `json:"summary_job_id,omitempty"`
	TopicsCount    int   `json:"topics_count,omitempty"`
	MessagesCount  int   `json:"messages_count,omitempty"`
	DuplicateCount int   `json:"duplicate_count,omitempty"`
}

func (s *Server) jobGet(w http.ResponseWriter, r *http.Request) {
	if s.jobs == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "jobs are not configured"})
		return
	}
	jobID, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	telegramUserID, ok := queryInt64(w, r, "telegram_user_id")
	if !ok {
		return
	}
	job, err := s.jobs.Find(r.Context(), jobID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if job == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	if !jobBelongsToTelegramUser(*job, telegramUserID) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, jobResponseFromDomain(*job))
}

func jobResponseFromDomain(job domain.Job) jobResponse {
	payload := job.Payload
	if len(payload) == 0 {
		payload = []byte(`{}`)
	}
	result := job.Result
	if len(result) == 0 {
		result = []byte(`{}`)
	}
	return jobResponse{
		ID:               job.ID,
		Type:             string(job.Type),
		Status:           string(job.Status),
		Payload:          json.RawMessage(payload),
		Result:           json.RawMessage(result),
		Attempt:          job.Attempt,
		MaxAttempts:      job.MaxAttempts,
		AvailableAt:      job.AvailableAt,
		LockedBy:         job.LockedBy,
		LastError:        job.LastError,
		CreatedAt:        job.CreatedAt,
		StartedAt:        job.StartedAt,
		FinishedAt:       job.FinishedAt,
		DeduplicationKey: job.DeduplicationKey,
		Artifacts:        jobArtifacts(job),
	}
}

func jobBelongsToTelegramUser(job domain.Job, telegramUserID int64) bool {
	switch job.Type {
	case domain.JobTypeSummaryGeneration:
		var payload domain.JobPayloadSummaryGeneration
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return false
		}
		return payload.TelegramUserID == telegramUserID
	default:
		return true
	}
}

func jobArtifacts(job domain.Job) jobArtifactsView {
	var artifacts jobArtifactsView
	if job.Type == domain.JobTypeSummaryGeneration {
		var payload domain.JobPayloadSummaryGeneration
		_ = json.Unmarshal(job.Payload, &payload)
		artifacts.CollectionJobID = payload.CollectionJobID
		var result domain.JobResultSummaryGeneration
		if err := json.Unmarshal(job.Result, &result); err == nil {
			artifacts.SummaryID = result.SummaryID
			artifacts.SummaryJobID = result.SummaryJobID
			artifacts.TopicsCount = result.TopicsCount
			artifacts.MessagesCount = result.MessagesCount
			artifacts.DuplicateCount = result.DuplicateCount
		}
	}
	return artifacts
}
