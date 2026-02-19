package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFromEnv_DataDirDefault(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("DATA_DIR", "")

	cfg, err := NewFromEnv()
	require.NoError(t, err)

	assert.Equal(t, "/app/data", cfg.System.DataDir)
	assert.Equal(t, filepath.Join("/app/data", "ctxtrans.db"), cfg.DBPath())
}

func TestNewFromEnv_DataDirFromEnv(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("DATA_DIR", "/tmp/ctx-data")

	cfg, err := NewFromEnv()
	require.NoError(t, err)

	assert.Equal(t, "/tmp/ctx-data", cfg.System.DataDir)
	assert.Equal(t, filepath.Join("/tmp/ctx-data", "ctxtrans.db"), cfg.DBPath())
}
