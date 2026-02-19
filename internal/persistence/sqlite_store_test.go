package persistence

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/MimeLyc/contextual-sub-translator/internal/jobs"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestSQLiteStore_JobsRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "ctxtrans.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	job := &jobs.TranslationJob{
		ID:        "job-1",
		Source:    "manual",
		DedupeKey: "m|s|zh",
		Payload: jobs.JobPayload{
			MediaFile:    "/media/a.mkv",
			SubtitleFile: "/media/a.srt",
		},
		Status:    jobs.StatusPending,
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
		UpdatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, store.UpsertJob(ctx, job))

	all, err := store.LoadJobs(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, job.ID, all[0].ID)
	assert.Equal(t, job.Status, all[0].Status)
	assert.Equal(t, job.Payload.MediaFile, all[0].Payload.MediaFile)
}

func TestSQLiteStore_CheckpointAndCleanup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "ctxtrans.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	jobID := "job-1"
	require.NoError(t, store.SaveBatchCheckpoint(ctx, jobID, 0, 2, []string{"a", "b"}))
	require.NoError(t, store.SaveBatchCheckpoint(ctx, jobID, 2, 4, []string{"c", "d"}))

	cps, err := store.LoadBatchCheckpoints(ctx, jobID)
	require.NoError(t, err)
	require.Len(t, cps, 2)
	assert.Equal(t, 0, cps[0].BatchStart)
	assert.Equal(t, []string{"a", "b"}, cps[0].TranslatedLines)

	require.NoError(t, store.ClearJobTemp(ctx, jobID))
	cps, err = store.LoadBatchCheckpoints(ctx, jobID)
	require.NoError(t, err)
	assert.Empty(t, cps)
}

func TestSQLiteStore_SubtitleCacheRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "ctxtrans.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	entry := SubtitleCacheEntry{
		CacheKey:  "media|s:0",
		MediaPath: "/media/a.mkv",
		JobID:     "job-1",
		File: subtitle.File{
			Path:     "embedded://a",
			Format:   "SRT",
			Language: language.English,
			Lines: []subtitle.Line{
				{Index: 1, Text: "hello"},
			},
		},
		IsTemp: true,
	}
	require.NoError(t, store.PutSubtitleCache(ctx, entry))

	cached, ok, err := store.GetSubtitleCache(ctx, entry.CacheKey)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, entry.File.Format, cached.Format)
	require.Len(t, cached.Lines, 1)
	assert.Equal(t, "hello", cached.Lines[0].Text)

	require.NoError(t, store.ClearJobTemp(ctx, "job-1"))
	_, ok, err = store.GetSubtitleCache(ctx, entry.CacheKey)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestSQLiteStore_MediaMetaCacheTTL(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "ctxtrans.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, store.PutMediaMetaCache(ctx, MediaMetaCache{
		MediaPath:         "/media/a.mkv",
		TargetLanguage:    "zh",
		ExternalLanguages: []string{"en"},
		EmbeddedLanguages: []string{"en"},
		HasTargetExternal: false,
		HasTargetEmbedded: false,
		ExpiresAt:         now.Add(30 * time.Minute),
		UpdatedAt:         now,
	}))

	meta, ok, err := store.GetMediaMetaCache(ctx, "/media/a.mkv", "zh", now)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, []string{"en"}, meta.ExternalLanguages)

	_, ok, err = store.GetMediaMetaCache(ctx, "/media/a.mkv", "zh", now.Add(31*time.Minute))
	require.NoError(t, err)
	assert.False(t, ok)
}
