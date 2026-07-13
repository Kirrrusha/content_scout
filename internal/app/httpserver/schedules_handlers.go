package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/kirilllebedenko/content_scout/internal/domain"
	"github.com/kirilllebedenko/content_scout/internal/schedules"
)

type scheduleRequest struct {
	TelegramUserID  int64  `json:"telegram_user_id"`
	SourceGroupID   int64  `json:"source_group_id"`
	Time            string `json:"time"`
	Timezone        string `json:"timezone"`
	QuietHoursStart string `json:"quiet_hours_start"`
	QuietHoursEnd   string `json:"quiet_hours_end"`
	SummaryType     string `json:"summary_type"`
	ExportEnabled   *bool  `json:"export_enabled"`
	Enabled         *bool  `json:"enabled"`
}

type scheduleRunRequest struct {
	TelegramUserID int64 `json:"telegram_user_id"`
}

type scheduleResponse struct {
	ID              int64      `json:"id"`
	UserID          int64      `json:"user_id"`
	SourceGroupID   int64      `json:"source_group_id"`
	Time            string     `json:"time"`
	Timezone        string     `json:"timezone"`
	QuietHoursStart string     `json:"quiet_hours_start"`
	QuietHoursEnd   string     `json:"quiet_hours_end"`
	SummaryType     string     `json:"summary_type"`
	ExportEnabled   bool       `json:"export_enabled"`
	Enabled         bool       `json:"enabled"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type scheduleJobResponse struct {
	JobID  int64  `json:"job_id"`
	Status string `json:"status"`
}

func (s *Server) schedulesList(w http.ResponseWriter, r *http.Request) {
	if !s.requireScheduleController(w) {
		return
	}
	telegramUserID, ok := telegramUserIDFromQuery(w, r)
	if !ok {
		return
	}
	items, err := s.schedules.List(r.Context(), telegramUserID)
	if err != nil {
		s.writeScheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, scheduleResponses(items))
}

func (s *Server) schedulesCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requireScheduleController(w) {
		return
	}
	var req scheduleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	item, err := s.schedules.Create(r.Context(), scheduleServiceRequest(req))
	if err != nil {
		s.writeScheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, scheduleResponseFromDomain(*item))
}

func (s *Server) schedulesGet(w http.ResponseWriter, r *http.Request) {
	if !s.requireScheduleController(w) {
		return
	}
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	telegramUserID, ok := telegramUserIDFromQuery(w, r)
	if !ok {
		return
	}
	item, err := s.schedules.Get(r.Context(), telegramUserID, id)
	if err != nil {
		s.writeScheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, scheduleResponseFromDomain(*item))
}

func (s *Server) schedulesUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requireScheduleController(w) {
		return
	}
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	var req scheduleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	item, err := s.schedules.Update(r.Context(), id, scheduleServiceRequest(req))
	if err != nil {
		s.writeScheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, scheduleResponseFromDomain(*item))
}

func (s *Server) schedulesDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireScheduleController(w) {
		return
	}
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	var req scheduleRunRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.schedules.Delete(r.Context(), req.TelegramUserID, id); err != nil {
		s.writeScheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) schedulesEnable(w http.ResponseWriter, r *http.Request) {
	s.setScheduleEnabled(w, r, true)
}

func (s *Server) schedulesDisable(w http.ResponseWriter, r *http.Request) {
	s.setScheduleEnabled(w, r, false)
}

func (s *Server) setScheduleEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	if !s.requireScheduleController(w) {
		return
	}
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	var req scheduleRunRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	item, err := s.schedules.SetEnabled(r.Context(), req.TelegramUserID, id, enabled)
	if err != nil {
		s.writeScheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, scheduleResponseFromDomain(*item))
}

func (s *Server) schedulesRun(w http.ResponseWriter, r *http.Request) {
	if !s.requireScheduleController(w) {
		return
	}
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	var req scheduleRunRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	job, err := s.schedules.Run(r.Context(), req.TelegramUserID, id)
	if err != nil {
		s.writeScheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, scheduleJobResponse{JobID: job.ID, Status: string(job.Status)})
}

func (s *Server) schedulesRuns(w http.ResponseWriter, r *http.Request) {
	if !s.requireScheduleController(w) {
		return
	}
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	telegramUserID, ok := telegramUserIDFromQuery(w, r)
	if !ok {
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a non-negative integer"})
			return
		}
		limit = parsed
	}
	runs, err := s.schedules.ListRuns(r.Context(), telegramUserID, id, limit)
	if err != nil {
		s.writeScheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) requireScheduleController(w http.ResponseWriter) bool {
	if s.schedules == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "schedules are not configured"})
		return false
	}
	return true
}

func (s *Server) writeScheduleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, schedules.ErrScheduleNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrBadRequest):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		s.writeAuthError(w, err)
	}
}

func scheduleServiceRequest(req scheduleRequest) schedules.Request {
	out := schedules.Request{
		TelegramUserID:  req.TelegramUserID,
		GroupID:         req.SourceGroupID,
		Time:            req.Time,
		Timezone:        req.Timezone,
		QuietHoursStart: req.QuietHoursStart,
		QuietHoursEnd:   req.QuietHoursEnd,
		SummaryType:     req.SummaryType,
	}
	if req.ExportEnabled != nil {
		out.ExportToObsidian = *req.ExportEnabled
		out.ExportProvided = true
	}
	if req.Enabled != nil {
		out.Enabled = *req.Enabled
		out.EnabledProvided = true
	}
	return out
}

func scheduleResponses(items []domain.SummarySchedule) []scheduleResponse {
	responses := make([]scheduleResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, scheduleResponseFromDomain(item))
	}
	return responses
}

func scheduleResponseFromDomain(item domain.SummarySchedule) scheduleResponse {
	return scheduleResponse{
		ID:              item.ID,
		UserID:          item.UserID,
		SourceGroupID:   item.GroupID,
		Time:            item.Cron,
		Timezone:        item.Timezone,
		QuietHoursStart: item.QuietHoursStart,
		QuietHoursEnd:   item.QuietHoursEnd,
		SummaryType:     item.SummaryType,
		ExportEnabled:   item.ExportToObsidian,
		Enabled:         item.Enabled,
		LastRunAt:       item.LastRunAt,
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
	}
}
