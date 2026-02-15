package translator

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTranslationUserMessage_IndexedLines(t *testing.T) {
	t.Parallel()

	payload, err := buildTranslationUserMessage([]string{"line-1", "line-2"})
	require.NoError(t, err)

	var decoded struct {
		Lines []struct {
			Index int    `json:"index"`
			Text  string `json:"text"`
		} `json:"lines"`
	}
	require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
	require.Len(t, decoded.Lines, 2)
	assert.Equal(t, 1, decoded.Lines[0].Index)
	assert.Equal(t, "line-1", decoded.Lines[0].Text)
	assert.Equal(t, 2, decoded.Lines[1].Index)
	assert.Equal(t, "line-2", decoded.Lines[1].Text)
}

func TestParseTranslationOutput_IndexedJSONReordered(t *testing.T) {
	t.Parallel()

	got, err := parseTranslationOutput(`[{"index":2,"text":"世界"},{"index":1,"text":"你好"}]`, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"你好", "世界"}, got)
}

func TestParseTranslationOutput_StringArrayFallback(t *testing.T) {
	t.Parallel()

	got, err := parseTranslationOutput(`["你好","世界"]`, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"你好", "世界"}, got)
}

func TestParseTranslationOutput_RejectLegacyFallback(t *testing.T) {
	t.Parallel()

	legacy := "你好" + subtitleLineBreaker + "世界"
	_, err := parseTranslationOutput(legacy, 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "json")
}

func TestParseTranslationOutput_EmptyContent(t *testing.T) {
	t.Parallel()

	_, err := parseTranslationOutput("   ", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}
