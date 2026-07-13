package domain

import "time"

type SummarySchedule struct {
	ID               int64      `json:"id"`
	UserID           int64      `json:"user_id"`
	GroupID          int64      `json:"group_id"`
	Cron             string     `json:"cron"`
	Timezone         string     `json:"timezone"`
	Enabled          bool       `json:"enabled"`
	SummaryType      string     `json:"summary_type"`
	QuietHoursStart  string     `json:"quiet_hours_start"`
	QuietHoursEnd    string     `json:"quiet_hours_end"`
	ExportToObsidian bool       `json:"export_to_obsidian"`
	LastRunAt        *time.Time `json:"last_run_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at,omitempty"`
}

type ScheduleRun struct {
	ID              int64      `json:"id"`
	ScheduleID      int64      `json:"schedule_id"`
	CollectionJobID *int64     `json:"collection_job_id,omitempty"`
	SummaryID       *int64     `json:"summary_id,omitempty"`
	ExportID        *int64     `json:"export_id,omitempty"`
	Status          JobStatus  `json:"status"`
	Error           *string    `json:"error,omitempty"`
	StartedAt       time.Time  `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}
