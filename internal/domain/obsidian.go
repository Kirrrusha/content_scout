package domain

import "time"

type ObsidianExport struct {
	ID           int64
	ArticleID    *int64
	SummaryID    *int64
	FileName     string
	VaultPath    string
	ExportMethod string
	ContentHash  string
	ExportedAt   time.Time
}
