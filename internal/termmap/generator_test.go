package termmap

import (
	"context"
	"os"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MimeLyc/contextual-sub-translator/internal/agent"
	"github.com/MimeLyc/contextual-sub-translator/internal/media"
	"github.com/MimeLyc/contextual-sub-translator/internal/tools"
)

func TestParseTermMapResponse_CleanJSON(t *testing.T) {
	input := `{"Momo Ayase": "绫濑桃", "Okarun": "奥卡轮"}`

	tm, err := parseTermMapResponse(input)
	require.NoError(t, err)
	assert.Len(t, tm, 2)
	assert.Equal(t, "绫濑桃", tm["Momo Ayase"])
	assert.Equal(t, "奥卡轮", tm["Okarun"])
}

func TestParseTermMapResponse_CodeFencedJSON(t *testing.T) {
	input := "```json\n{\"Momo Ayase\": \"绫濑桃\", \"Okarun\": \"奥卡轮\"}\n```"

	tm, err := parseTermMapResponse(input)
	require.NoError(t, err)
	assert.Len(t, tm, 2)
	assert.Equal(t, "绫濑桃", tm["Momo Ayase"])
}

func TestParseTermMapResponse_CodeFenceNoLang(t *testing.T) {
	input := "```\n{\"hello\": \"world\"}\n```"

	tm, err := parseTermMapResponse(input)
	require.NoError(t, err)
	assert.Len(t, tm, 1)
	assert.Equal(t, "world", tm["hello"])
}

func TestParseTermMapResponse_WithWhitespace(t *testing.T) {
	input := "  \n{\"hello\": \"world\"}\n  "

	tm, err := parseTermMapResponse(input)
	require.NoError(t, err)
	assert.Len(t, tm, 1)
}

func TestParseTermMapResponse_JSONEmbeddedInProse(t *testing.T) {
	input := `Based on my research, here is the term mapping:

{"Momo Ayase": "绫濑桃", "Okarun": "奥卡轮"}

Hope this helps!`

	tm, err := parseTermMapResponse(input)
	require.NoError(t, err)
	assert.Len(t, tm, 2)
	assert.Equal(t, "绫濑桃", tm["Momo Ayase"])
	assert.Equal(t, "奥卡轮", tm["Okarun"])
}

func TestParseTermMapResponse_CodeFenceInProse(t *testing.T) {
	input := "Here are the results:\n\n```json\n{\"hello\": \"world\"}\n```\n\nLet me know if you need more."

	tm, err := parseTermMapResponse(input)
	require.NoError(t, err)
	assert.Len(t, tm, 1)
	assert.Equal(t, "world", tm["hello"])
}

func TestParseTermMapResponse_NestedBraces(t *testing.T) {
	// Ensure the extractor handles escaped quotes in values
	input := `Some text {"key": "value with \"braces\" inside"} more text`

	tm, err := parseTermMapResponse(input)
	require.NoError(t, err)
	assert.Equal(t, "value with \"braces\" inside", tm["key"])
}

func TestParseTermMapResponse_InvalidJSON(t *testing.T) {
	_, err := parseTermMapResponse("not json at all")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no valid JSON object found")
}

func TestParseTermMapResponse_EmptyObject(t *testing.T) {
	tm, err := parseTermMapResponse("{}")
	require.NoError(t, err)
	assert.Empty(t, tm)
}

func TestBuildGeneratorPrompt(t *testing.T) {
	showInfo := media.TVShowInfo{
		Title:         "DAN DA DAN",
		OriginalTitle: "ダンダダン",
		Genre:         []string{"Action", "Comedy"},
		Year:          2024,
		Studio:        "Science SARU",
		Actors: []media.Actor{
			{Name: "若山诗音", Role: "Momo Ayase (voice)"},
			{Name: "花江夏树", Role: "Okarun (voice)"},
		},
	}

	prompt := buildGeneratorPrompt(showInfo, "English", "Chinese")

	assert.Contains(t, prompt, "DAN DA DAN")
	assert.Contains(t, prompt, "ダンダダン")
	assert.Contains(t, prompt, "Action, Comedy")
	assert.Contains(t, prompt, "2024")
	assert.Contains(t, prompt, "Science SARU")
	assert.Contains(t, prompt, "若山诗音 as Momo Ayase (voice)")
	assert.Contains(t, prompt, "web_search")
	assert.Contains(t, prompt, "English")
	assert.Contains(t, prompt, "Chinese")
	assert.Contains(t, prompt, "JSON")
	assert.Contains(t, prompt, "start with {")
	assert.Contains(t, prompt, "NO markdown")
}

func TestGenerate_Integration(t *testing.T) {
	_ = godotenv.Load(".env")

	apiKey := os.Getenv("LLM_API_KEY")
	searchAPIKey := os.Getenv("SEARCH_API_KEY")
	if apiKey == "" || searchAPIKey == "" {
		t.Skip("Skipping integration test: LLM_API_KEY or SEARCH_API_KEY not set")
	}

	apiURL := os.Getenv("LLM_API_URL")
	if apiURL == "" {
		apiURL = "https://openrouter.ai/api/v1"
	}
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "moonshotai/kimi-k2.5"
	}

	llmConfig := agent.LLMConfig{
		APIKey:      apiKey,
		APIURL:      apiURL,
		Model:       model,
		MaxTokens:   5000,
		Temperature: 0.5,
		Timeout:     120,
	}

	registry := tools.NewRegistry()
	webSearch := tools.NewWebSearchTool(searchAPIKey, os.Getenv("SEARCH_API_URL"))
	require.NoError(t, registry.Register(webSearch))

	llmAgent, err := agent.NewLLMAgent(llmConfig, registry, 10)
	require.NoError(t, err)
	gen := NewGenerator(llmAgent)

	showInfo := media.TVShowInfo{
		Title:         "DAN DA DAN",
		OriginalTitle: "ダンダダン",
		Genre:         []string{"Action", "Comedy", "Supernatural"},
		Year:          2024,
		Studio:        "Science SARU",
		Actors: []media.Actor{
			{Name: "若山诗音", Role: "Momo Ayase (voice)", Order: 0},
			{Name: "花江夏树", Role: "Ken 'Okarun' Takakura (voice)", Order: 1},
		},
	}

	tm, err := gen.Generate(context.Background(), showInfo, "English", "Chinese")
	require.NoError(t, err)

	t.Logf("Generated %d term mappings:", len(tm))
	for src, tgt := range tm {
		t.Logf("  %s -> %s", src, tgt)
	}

	assert.Greater(t, len(tm), 0, "should generate at least one term mapping")
}

func TestBuildGeneratorPrompt_MinimalInfo(t *testing.T) {
	showInfo := media.TVShowInfo{
		Title: "Test Show",
	}

	prompt := buildGeneratorPrompt(showInfo, "en", "zh")

	assert.Contains(t, prompt, "Test Show")
	assert.Contains(t, prompt, "web_search")
	assert.NotContains(t, prompt, "Original Title")
	assert.NotContains(t, prompt, "Genre")
	assert.NotContains(t, prompt, "Cast")
}

func TestExtractNewTerms_NoResults(t *testing.T) {
	// Test with empty tool calls
	gen := NewGenerator(nil)
	result, err := gen.ExtractNewTerms(context.Background(), nil, TermMap{}, "Test Show", "en", "zh")
	require.NoError(t, err)
	assert.Empty(t, result)

	// Test with no web_search results
	toolCalls := []agent.ToolCallRecord{
		{ToolName: "other_tool", Result: "some result", IsError: false},
	}
	result, err = gen.ExtractNewTerms(context.Background(), toolCalls, TermMap{}, "Test Show", "en", "zh")
	require.NoError(t, err)
	assert.Empty(t, result)

	// Test with only error results
	toolCalls = []agent.ToolCallRecord{
		{ToolName: "web_search", Result: "error", IsError: true},
	}
	result, err = gen.ExtractNewTerms(context.Background(), toolCalls, TermMap{}, "Test Show", "en", "zh")
	require.NoError(t, err)
	assert.Empty(t, result)
}
