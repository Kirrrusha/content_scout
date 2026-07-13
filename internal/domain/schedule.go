package domain

import "time"

type SummarySchedule struct {
	ID               int64
	UserID           int64
	GroupID          int64
	Cron             string
	Timezone         string
	Enabled          bool
	SummaryType      string
	QuietHoursStart  string
	QuietHoursEnd    string
	ExportToObsidian bool
	LastRunAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ScheduleRun struct {
	ID              int64
	ScheduleID      int64
	CollectionJobID *int64
	SummaryID       *int64
	ExportID        *int64
	Status          JobStatus
	Error           *string
	StartedAt       time.Time
	CompletedAt     *time.Time
}
