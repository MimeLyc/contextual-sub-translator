package jobs

import "time"

type Status string

const (
	StatusPending Status = "pending"
	StatusRunning Status = "running"
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
)

type EnqueueRequest struct {
	Source    string
	DedupeKey string
	Payload   JobPayload
}

type JobPayload struct {
	MediaFile    string `json:"media_file"`
	SubtitleFile string `json:"subtitle_file"`
	NFOFile      string `json:"nfo_file"`
}

type TranslationJob struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	DedupeKey string    `json:"dedupe_key"`
	Payload   JobPayload `json:"payload"`
	Status    Status    `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
