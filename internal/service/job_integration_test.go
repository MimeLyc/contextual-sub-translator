package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MimeLyc/contextual-sub-translator/internal/config"
	"github.com/MimeLyc/contextual-sub-translator/internal/jobs"
	"github.com/MimeLyc/contextual-sub-translator/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestLoadSubtitleForJobPersistsOriginalSnapshot(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	store, err := persistence.NewSQLiteStore(filepath.Join(tmp, "state.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	subtitlePath := filepath.Join(tmp, "episode01.srt")
	require.NoError(t, os.WriteFile(subtitlePath, []byte("1\n00:00:01,000 --> 00:00:02,000\nOriginal line\n"), 0o644))

	job := &jobs.TranslationJob{
		ID: "job-1",
		Payload: jobs.JobPayload{
			MediaFile:    filepath.Join(tmp, "episode01.mkv"),
			SubtitleFile: subtitlePath,
		},
	}
	svc := transService{store: store}

	loaded, err := svc.loadSubtitleForJob(context.Background(), job)
	require.NoError(t, err)
	require.Len(t, loaded.Lines, 1)

	cached, ok, err := store.GetSubtitleCache(context.Background(), persistence.JobOriginalSubtitleCacheKey(job.ID))
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, cached.Lines, 1)
	assert.Equal(t, "Original line", cached.Lines[0].Text)
}

func TestManualAndCron_UseSameEnqueueDedup(t *testing.T) {
	q := jobs.NewQueue(1, nil)
	svc := transService{
		cfg: config.Config{
			Translate: config.TranslateConfig{
				TargetLanguage: language.Chinese,
			},
		},
		jobQueue: q,
	}

	bundle := MediaPathBundle{
		MediaFile:     "/media/episode01.mkv",
		SubtitleFiles: []string{"/media/episode01.srt"},
	}

	jobFromCron, createdCron, err := svc.enqueueCronBundle(bundle)
	require.NoError(t, err)
	require.NotNil(t, jobFromCron)
	require.True(t, createdCron)
	assert.Equal(t, "/media/episode01.mkv", jobFromCron.Payload.MediaFile)
	assert.Equal(t, "/media/episode01.srt", jobFromCron.Payload.SubtitleFile)

	jobFromManual, createdManual, err := svc.enqueueManualBundle(bundle)
	require.NoError(t, err)
	require.NotNil(t, jobFromManual)
	require.False(t, createdManual)

	assert.Equal(t, jobFromCron.ID, jobFromManual.ID)
	assert.Len(t, q.List(), 1)
}
