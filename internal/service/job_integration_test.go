package service

import (
	"testing"

	"github.com/MimeLyc/contextual-sub-translator/internal/config"
	"github.com/MimeLyc/contextual-sub-translator/internal/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

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
