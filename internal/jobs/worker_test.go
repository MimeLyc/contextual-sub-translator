package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQueue_Worker_TransitionsStatus(t *testing.T) {
	q := NewQueue(1, nil)
	q.Start(func(_ context.Context, _ *TranslationJob) error { return nil })
	defer q.Stop()

	job, _ := q.Enqueue(EnqueueRequest{
		Source:    "manual",
		DedupeKey: "k1",
	})

	require.Eventually(t, func() bool {
		got, ok := q.Get(job.ID)
		if !ok || got == nil {
			return false
		}
		return got.Status == StatusSuccess
	}, time.Second, 10*time.Millisecond)
}
