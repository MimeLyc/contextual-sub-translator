package translator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildContextPrompt_HardRulesAndSearchBudget(t *testing.T) {
	t.Parallel()

	translator := &agentTranslator{searchEnabled: true}
	media := MediaMeta{TermMap: map[string]string{"John": "约翰"}}
	prompt := translator.buildContextPrompt(media, "English", "Chinese", true, []string{"John met Neo in Zion."})

	assert.Contains(t, prompt, "MUST use the mapped target term exactly")
	assert.Contains(t, prompt, "MUST preserve the count of %%inline_breaker%%")
	assert.Contains(t, prompt, "Do NOT output literal newline characters in JSON text")
	assert.Contains(t, prompt, "Do NOT merge, split, reorder, or drop lines")
	assert.Contains(t, prompt, "If an input line is empty, output text for that index MUST be an empty string")
	assert.Contains(t, prompt, "TERM MAPPINGS > official localized names > transliteration")
	assert.Contains(t, prompt, "at most 1 web_search call")
	assert.Contains(t, prompt, "index")
}

func TestBuildContextPrompt_NoTermMapUsesDynamicSearchCap(t *testing.T) {
	t.Parallel()

	translator := &agentTranslator{searchEnabled: true}
	prompt := translator.buildContextPrompt(MediaMeta{}, "English", "Chinese", false, []string{"Neo meets Trinity in Zion."})

	assert.Contains(t, prompt, "at most 2 web_search calls")
}

func TestValidateInlineBreakers_Mismatch(t *testing.T) {
	t.Parallel()

	err := validateInlineBreakers(
		[]string{"first%%inline_breaker%%second"},
		[]string{"translated without marker"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inline break")
}

func TestValidateInlineBreakers_OK(t *testing.T) {
	t.Parallel()

	err := validateInlineBreakers(
		[]string{"first%%inline_breaker%%second"},
		[]string{"第一%%inline_breaker%%第二"},
	)
	require.NoError(t, err)
}

func TestValidateTermMappings_MissingMappedTarget(t *testing.T) {
	t.Parallel()

	err := validateTermMappings(
		[]string{"John met Sarah at the station."},
		[]string{"约翰在车站见面了。"},
		map[string]string{"John": "约翰", "Sarah": "莎拉"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Sarah")
}

func TestValidateTermMappings_OK(t *testing.T) {
	t.Parallel()

	err := validateTermMappings(
		[]string{"John met Sarah at the station."},
		[]string{"约翰在车站遇到了莎拉。"},
		map[string]string{"John": "约翰", "Sarah": "莎拉"},
	)
	require.NoError(t, err)
}

func TestParseTranslationOutput_DuplicateIndex(t *testing.T) {
	t.Parallel()

	_, err := parseTranslationOutput(`[{"index":1,"text":"A"},{"index":1,"text":"B"}]`, 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}
