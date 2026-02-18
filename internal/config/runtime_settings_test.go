package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeSettings_Validate(t *testing.T) {
	valid := RuntimeSettings{
		LLMAPIURL:      "https://example.test/v1",
		LLMAPIKey:      "ak-test",
		LLMModel:       "model-test",
		CronExpr:       "*/5 * * * *",
		TargetLanguage: "zh",
	}
	require.NoError(t, valid.Validate())

	invalid := valid
	invalid.CronExpr = "bad cron"
	require.Error(t, invalid.Validate())

	invalidLang := valid
	invalidLang.TargetLanguage = ""
	require.Error(t, invalidLang.Validate())
}

func TestRuntimeSettingsFile_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "settings", "runtime.json")
	input := RuntimeSettings{
		LLMAPIURL:      "https://example.test/v1",
		LLMAPIKey:      "ak-test",
		LLMModel:       "model-test",
		CronExpr:       "0 0 * * *",
		TargetLanguage: "zh",
	}

	require.NoError(t, WriteRuntimeSettingsFile(filePath, input))

	got, err := LoadRuntimeSettingsFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, input, got)

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}

func TestWithRuntimeSettings_OverridesConfig(t *testing.T) {
	t.Setenv("LLM_API_KEY", "env-key")
	t.Setenv("LLM_API_URL", "https://env.example/v1")
	t.Setenv("LLM_MODEL", "env-model")
	t.Setenv("CRON_EXPR", "0 1 * * *")

	override := RuntimeSettings{
		LLMAPIURL:      "https://file.example/v1",
		LLMAPIKey:      "file-key",
		LLMModel:       "file-model",
		CronExpr:       "*/30 * * * *",
		TargetLanguage: "ja",
	}

	cfg, err := NewFromEnv(WithRuntimeSettings(override))
	require.NoError(t, err)
	assert.Equal(t, override.LLMAPIURL, cfg.LLM.APIURL)
	assert.Equal(t, override.LLMAPIKey, cfg.LLM.APIKey)
	assert.Equal(t, override.LLMModel, cfg.LLM.Model)
	assert.Equal(t, override.CronExpr, cfg.Translate.CronExpr)
	assert.Equal(t, "ja", cfg.Translate.TargetLanguage.String())
}

func TestRuntimeSettingsStore_UpdatePersistsFile(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "runtime-settings.json")
	initial := RuntimeSettings{
		LLMAPIURL:      "https://old.example/v1",
		LLMAPIKey:      "old-ak",
		LLMModel:       "old-model",
		CronExpr:       "0 0 * * *",
		TargetLanguage: "zh",
	}

	store, err := NewRuntimeSettingsStore(filePath, initial)
	require.NoError(t, err)

	next := RuntimeSettings{
		LLMAPIURL:      "https://new.example/v1",
		LLMAPIKey:      "new-ak",
		LLMModel:       "new-model",
		CronExpr:       "*/10 * * * *",
		TargetLanguage: "en",
	}
	got, err := store.UpdateRuntimeSettings(next)
	require.NoError(t, err)
	assert.Equal(t, next, got)

	loaded, err := LoadRuntimeSettingsFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, next, loaded)
}
