package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueue_Enqueue_DeduplicatesSameKey(t *testing.T) {
	q := NewQueue(2, nil)

	jobA, createdA := q.Enqueue(EnqueueRequest{
		Source:    "manual",
		DedupeKey: "ep1|sub1|zh",
	})
	jobB, createdB := q.Enqueue(EnqueueRequest{
		Source:    "cron",
		DedupeKey: "ep1|sub1|zh",
	})

	require.True(t, createdA)
	require.False(t, createdB)
	require.NotNil(t, jobA)
	require.NotNil(t, jobB)
	assert.Equal(t, jobA.ID, jobB.ID)
}

func TestQueue_Enqueue_AllowsRetryAfterFailure(t *testing.T) {
	q := NewQueue(1, nil)

	var attempts int
	q.Start(func(_ context.Context, _ *TranslationJob) error {
		attempts++
		if attempts == 1 {
			return assert.AnError
		}
		return nil
	})
	defer q.Stop()

	first, created := q.Enqueue(EnqueueRequest{
		Source:    "manual",
		DedupeKey: "retry-key",
	})
	require.True(t, created)
	require.NotNil(t, first)

	require.Eventually(t, func() bool {
		got, ok := q.Get(first.ID)
		return ok && got != nil && got.Status == StatusFailed
	}, time.Second, 10*time.Millisecond)

	second, created := q.Enqueue(EnqueueRequest{
		Source:    "manual",
		DedupeKey: "retry-key",
	})
	require.True(t, created)
	require.NotNil(t, second)
	assert.NotEqual(t, first.ID, second.ID)

	require.Eventually(t, func() bool {
		got, ok := q.Get(second.ID)
		return ok && got != nil && got.Status == StatusSuccess
	}, time.Second, 10*time.Millisecond)
}

func TestQueue_Enqueue_AllowsRetryAfterSuccess(t *testing.T) {
	q := NewQueue(1, nil)
	q.Start(func(_ context.Context, _ *TranslationJob) error { return nil })
	defer q.Stop()

	first, created := q.Enqueue(EnqueueRequest{
		Source:    "manual",
		DedupeKey: "done-key",
	})
	require.True(t, created)
	require.NotNil(t, first)

	require.Eventually(t, func() bool {
		got, ok := q.Get(first.ID)
		return ok && got != nil && got.Status == StatusSuccess
	}, time.Second, 10*time.Millisecond)

	second, created := q.Enqueue(EnqueueRequest{
		Source:    "manual",
		DedupeKey: "done-key",
	})
	require.True(t, created)
	require.NotNil(t, second)
	assert.NotEqual(t, first.ID, second.ID)
}
