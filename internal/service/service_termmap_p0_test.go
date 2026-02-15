package service

import (
	"testing"

	"github.com/MimeLyc/contextual-sub-translator/internal/termmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveMergedTermMap_PreservesExistingTerms(t *testing.T) {
	t.Parallel()

	tmPath := termmap.FilePath(t.TempDir(), "en", "zh")
	existing := map[string]string{
		"Okarun": "奥卡伦",
	}
	newTerms := termmap.TermMap{
		"Momo Ayase": "绫濑桃",
	}

	merged, err := saveMergedTermMap(tmPath, existing, newTerms)
	require.NoError(t, err)

	assert.Equal(t, "奥卡伦", merged["Okarun"])
	assert.Equal(t, "绫濑桃", merged["Momo Ayase"])

	loaded, err := termmap.Load(tmPath)
	require.NoError(t, err)
	assert.Equal(t, termmap.TermMap(merged), loaded)
}
