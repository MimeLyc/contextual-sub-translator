package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFromEnv_HTTPDefaults(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")

	cfg, err := NewFromEnv()
	require.NoError(t, err)

	assert.Equal(t, ":8080", cfg.HTTP.Addr)
	assert.Equal(t, "/app/web", cfg.HTTP.UIStaticDir)
	assert.True(t, cfg.HTTP.UIEnabled)
}

