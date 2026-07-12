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

func TestParseTranslationOutput_IndexedJSONWithStringIndexAndHTML(t *testing.T) {
	t.Parallel()

	got, err := parseTranslationOutput(`[
		{"index":"1","text":"<font face=\"Trebuchet MS\" size=\"24\">嘿，你听说了吗？</font>"},
		{"index":"2","text":"<font face=\"Trebuchet MS\" size=\"24\">她和那个一见钟情的男人%%inline_breaker%%结婚了。</font>"}
	]`, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{
		`<font face="Trebuchet MS" size="24">嘿，你听说了吗？</font>`,
		`<font face="Trebuchet MS" size="24">她和那个一见钟情的男人%%inline_breaker%%结婚了。</font>`,
	}, got)
}

func TestNormalizeTranslatedLines_StripsHTMLAndPreservesPlainComparisons(t *testing.T) {
	t.Parallel()

	got := normalizeTranslatedLines([]string{
		`<font face="Trebuchet MS"><i>你好</i><br>世界</font>`,
		`&lt;b&gt;已转义&lt;/b&gt;`,
		`5 < 6 && 7 > 3`,
		`第一行%%inline_breaker%%<span class="x">第二行</span>`,
	})

	assert.Equal(t, []string{
		"你好\n世界",
		"已转义",
		`5 < 6 && 7 > 3`,
		"第一行\n第二行",
	}, got)
}

func TestParseTranslationOutput_ObjectTranslationsIndexedObjects(t *testing.T) {
	t.Parallel()

	got, err := parseTranslationOutput(`{"translations":[{"index":1,"text":"你好"},{"index":2,"text":"世界"}]}`, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"你好", "世界"}, got)
}

func TestParseTranslationOutput_ObjectTranslatedLinesWithAliasFields(t *testing.T) {
	t.Parallel()

	got, err := parseTranslationOutput(`{"translated_lines":[{"line":1,"translation":"你好"},{"line":2,"translation":"世界"}]}`, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"你好", "世界"}, got)
}

func TestParseTranslationOutput_NumericKeyedObject(t *testing.T) {
	t.Parallel()

	got, err := parseTranslationOutput(`{"2":"世界","1":"你好"}`, 2)
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
