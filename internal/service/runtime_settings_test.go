package service

import (
	"context"
	"testing"

	"github.com/MimeLyc/contextual-sub-translator/internal/config"
	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestTransService_ApplyRuntimeSettings_ReschedulesCronAndUpdatesConfig(t *testing.T) {
	cronEngine := cron.New()
	svc := NewRunnableTransService(
		config.Config{
			LLM: config.LLMConfig{
				APIKey: "old-ak",
				APIURL: "https://old.example/v1",
				Model:  "old-model",
			},
			Translate: config.TranslateConfig{
				TargetLanguage: language.Chinese,
				CronExpr:       "0 0 * * *",
			},
		},
		cronEngine,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, svc.Schedule(ctx))
	require.Len(t, cronEngine.Entries(), 1)

	err := svc.ApplyRuntimeSettings(config.RuntimeSettings{
		LLMAPIURL:      "https://new.example/v1",
		LLMAPIKey:      "new-ak",
		LLMModel:       "new-model",
		CronExpr:       "*/10 * * * *",
		TargetLanguage: "en",
	})
	require.NoError(t, err)

	assert.Equal(t, "*/10 * * * *", svc.cronExpr)
	assert.Equal(t, "new-ak", svc.cfg.LLM.APIKey)
	assert.Equal(t, "https://new.example/v1", svc.cfg.LLM.APIURL)
	assert.Equal(t, "new-model", svc.cfg.LLM.Model)
	assert.Equal(t, language.English, svc.cfg.Translate.TargetLanguage)
	require.Len(t, cronEngine.Entries(), 1)
}
