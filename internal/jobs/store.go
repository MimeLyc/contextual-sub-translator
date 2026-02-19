package jobs

import "context"

// Store persists job states for queue restart recovery.
type Store interface {
	LoadJobs(ctx context.Context) ([]*TranslationJob, error)
	UpsertJob(ctx context.Context, job *TranslationJob) error
	DeleteJob(ctx context.Context, jobID string) error
	// DeleteJobData removes all auxiliary data (checkpoints, temp caches) for a job.
	DeleteJobData(ctx context.Context, jobID string) error
}
