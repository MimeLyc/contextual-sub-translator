package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryStore struct {
	jobs map[string]*TranslationJob
}

func newMemoryStore() *memoryStore {
	return &memoryStore{jobs: make(map[string]*TranslationJob)}
}

func (m *memoryStore) LoadJobs(_ context.Context) ([]*TranslationJob, error) {
	ret := make([]*TranslationJob, 0, len(m.jobs))
	for _, j := range m.jobs {
		ret = append(ret, cloneJob(j))
	}
	return ret, nil
}

func (m *memoryStore) UpsertJob(_ context.Context, job *TranslationJob) error {
	m.jobs[job.ID] = cloneJob(job)
	return nil
}

func (m *memoryStore) DeleteJob(_ context.Context, jobID string) error {
	delete(m.jobs, jobID)
	return nil
}

func (m *memoryStore) DeleteJobData(_ context.Context, _ string) error {
	return nil
}

func TestQueue_RecoversPendingAndRunningJobsFromStore(t *testing.T) {
	store := newMemoryStore()
	now := time.Now()
	store.jobs["job-1"] = &TranslationJob{
		ID:        "job-1",
		Source:    "cron",
		DedupeKey: "m1|s1|zh",
		Status:    StatusPending,
		Payload: JobPayload{
			MediaFile: "/media/ep1.mkv",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	store.jobs["job-2"] = &TranslationJob{
		ID:        "job-2",
		Source:    "cron",
		DedupeKey: "m2|s2|zh",
		Status:    StatusRunning,
		Payload: JobPayload{
			MediaFile: "/media/ep2.mkv",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	q := NewQueue(1, store)

	jobs := q.List()
	require.Len(t, jobs, 2)
	byID := map[string]*TranslationJob{}
	for _, j := range jobs {
		byID[j.ID] = j
	}
	require.Contains(t, byID, "job-2")
	assert.Equal(t, StatusPending, byID["job-2"].Status)

	q.Start(func(_ context.Context, _ *TranslationJob) error { return nil })
	defer q.Stop()

	require.Eventually(t, func() bool {
		got, ok := q.Get("job-1")
		return ok && got.Status == StatusSuccess
	}, time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		got, ok := q.Get("job-2")
		return ok && got.Status == StatusSuccess
	}, time.Second, 10*time.Millisecond)
}
