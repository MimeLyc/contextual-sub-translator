package translator

import (
	"strings"
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

func TestFixInlineBreakers_AlreadyCorrect(t *testing.T) {
	t.Parallel()

	translated := []string{"第一%%inline_breaker%%第二"}
	fixInlineBreakers(
		[]string{"first%%inline_breaker%%second"},
		translated,
	)
	assert.Equal(t, "第一%%inline_breaker%%第二", translated[0])
}

func TestFixInlineBreakers_InsertMissing(t *testing.T) {
	t.Parallel()

	translated := []string{"翻译后的完整文本"}
	fixInlineBreakers(
		[]string{"first%%inline_breaker%%second"},
		translated,
	)
	assert.Equal(t, 1, strings.Count(translated[0], "%%inline_breaker%%"))
}

func TestFixInlineBreakers_RemoveExtra(t *testing.T) {
	t.Parallel()

	translated := []string{"第一%%inline_breaker%%第二%%inline_breaker%%第三"}
	fixInlineBreakers(
		[]string{"first and second"},
		translated,
	)
	assert.Equal(t, 0, strings.Count(translated[0], "%%inline_breaker%%"))
}

func TestFixInlineBreakers_MultipleLines(t *testing.T) {
	t.Parallel()

	source := []string{
		"a%%inline_breaker%%b",
		"no breakers here",
		"x%%inline_breaker%%y%%inline_breaker%%z",
	}
	translated := []string{
		"甲乙",                                   // missing 1
		"没有换行%%inline_breaker%%多了",          // extra 1
		"一%%inline_breaker%%二%%inline_breaker%%三", // correct
	}
	fixInlineBreakers(source, translated)
	assert.Equal(t, 1, strings.Count(translated[0], "%%inline_breaker%%"))
	assert.Equal(t, 0, strings.Count(translated[1], "%%inline_breaker%%"))
	assert.Equal(t, 2, strings.Count(translated[2], "%%inline_breaker%%"))
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

func TestValidateTermMappings_SubstringNotFalsePositive(t *testing.T) {
	t.Parallel()

	// "elf" inside "herself" should NOT trigger the term mapping check
	err := validateTermMappings(
		[]string{"She found herself alone in the dark."},
		[]string{"她发现自己独自处于黑暗之中。"},
		map[string]string{"elf": "精灵"},
	)
	require.NoError(t, err)
}

func TestValidateTermMappings_StandaloneTermMatches(t *testing.T) {
	t.Parallel()

	// "elf" as a standalone word SHOULD trigger the check
	err := validateTermMappings(
		[]string{"The elf cast a powerful spell."},
		[]string{"那个战士施放了强力法术。"},
		map[string]string{"elf": "精灵"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "elf")
}

func TestParseTranslationOutput_DuplicateIndex(t *testing.T) {
	t.Parallel()

	_, err := parseTranslationOutput(`[{"index":1,"text":"A"},{"index":1,"text":"B"}]`, 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}
