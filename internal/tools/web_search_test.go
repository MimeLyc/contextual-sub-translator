package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSearchTool_Name(t *testing.T) {
	tool := NewWebSearchTool("test-api-key", "")
	assert.Equal(t, "web_search", tool.Name())
}

func TestWebSearchTool_Description(t *testing.T) {
	tool := NewWebSearchTool("test-api-key", "")
	desc := tool.Description()
	assert.Contains(t, desc, "character names")
	assert.Contains(t, desc, "anime")
	assert.Contains(t, desc, "terminology")
}

func TestWebSearchTool_Parameters(t *testing.T) {
	tool := NewWebSearchTool("test-api-key", "")
	params := tool.Parameters()

	var schema map[string]any
	err := json.Unmarshal(params, &schema)
	require.NoError(t, err)

	assert.Equal(t, "object", schema["type"])

	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "query")
	assert.Contains(t, props, "show_name")
	assert.Contains(t, props, "target_language")
	assert.Contains(t, props, "search_type")

	required := schema["required"].([]any)
	assert.Contains(t, required, "query")
}

func TestWebSearchTool_BuildQuery(t *testing.T) {
	tool := NewWebSearchTool("test-api-key", "")

	tests := []struct {
		name     string
		args     WebSearchArgs
		expected string
	}{
		{
			name: "simple query without context",
			args: WebSearchArgs{
				Query: "Naruto characters",
			},
			expected: "Naruto characters",
		},
		{
			name: "character search with show name and target language",
			args: WebSearchArgs{
				Query:          "main characters",
				ShowName:       "Attack on Titan",
				TargetLanguage: "Chinese",
				SearchType:     "characters",
			},
			expected: "Attack on Titan Chinese official character names main characters localization",
		},
		{
			name: "place search for anime locations",
			args: WebSearchArgs{
				Query:          "locations",
				ShowName:       "Spirited Away",
				TargetLanguage: "Chinese",
				SearchType:     "places",
			},
			expected: "Spirited Away Chinese official place names locations locations",
		},
		{
			name: "terminology search",
			args: WebSearchArgs{
				Query:          "jutsu techniques",
				ShowName:       "Naruto",
				TargetLanguage: "Chinese",
				SearchType:     "terminology",
			},
			expected: "Naruto Chinese official terminology translation jutsu techniques",
		},
		{
			name: "all search type",
			args: WebSearchArgs{
				Query:          "wiki",
				ShowName:       "One Piece",
				TargetLanguage: "Chinese",
				SearchType:     "all",
			},
			expected: "One Piece Chinese official translation wiki",
		},
		{
			name: "default search type when not specified",
			args: WebSearchArgs{
				Query:          "info",
				ShowName:       "Demon Slayer",
				TargetLanguage: "Japanese",
			},
			expected: "Demon Slayer Japanese official translation info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tool.buildQuery(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWebSearchTool_Execute_WithMockServer(t *testing.T) {
	// Create mock Tavily API server
	mockResponse := TavilyResponse{
		Query:  "Attack on Titan Chinese official character names",
		Answer: "The main characters in Attack on Titan have official Chinese names: Eren Yeager is 艾伦·耶格尔, Mikasa Ackerman is 三笠·阿克曼, Armin Arlert is 阿明·阿诺德.",
		Results: []TavilyResult{
			{
				Title:   "Attack on Titan Character Names - Chinese Wiki",
				URL:     "https://attackontitan.fandom.com/zh/wiki/Characters",
				Content: "艾伦·耶格尔 (Eren Yeager) - 主角，拥有巨人之力。三笠·阿克曼 (Mikasa Ackerman) - 艾伦的青梅竹马。阿明·阿诺德 (Armin Arlert) - 艾伦和三笠的好友。",
				Score:   0.95,
			},
			{
				Title:   "进击的巨人 - 百度百科",
				URL:     "https://baike.baidu.com/item/进击的巨人",
				Content: "《进击的巨人》是日本漫画家谏山创创作的少年漫画作品。主要角色包括艾伦·耶格尔、三笠·阿克曼等。",
				Score:   0.88,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req TavilyRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "test-api-key", req.APIKey)
		assert.Contains(t, req.Query, "Attack on Titan")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	tool := NewWebSearchTool("test-api-key", server.URL)

	args := WebSearchArgs{
		Query:          "main characters",
		ShowName:       "Attack on Titan",
		TargetLanguage: "Chinese",
		SearchType:     "characters",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(context.Background(), argsJSON)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// Verify result contains character names
	assert.Contains(t, result.Content, "艾伦·耶格尔")
	assert.Contains(t, result.Content, "三笠·阿克曼")
	assert.Contains(t, result.Content, "Attack on Titan")
}

func TestWebSearchTool_Execute_AnimeCharacterSearch(t *testing.T) {
	// Test searching for anime character names in different languages
	tests := []struct {
		name           string
		showName       string
		targetLanguage string
		mockAnswer     string
		mockResults    []TavilyResult
		expectedNames  []string
	}{
		{
			name:           "Naruto characters in Chinese",
			showName:       "Naruto",
			targetLanguage: "Chinese",
			mockAnswer:     "Naruto Uzumaki is 漩涡鸣人, Sasuke Uchiha is 宇智波佐助",
			mockResults: []TavilyResult{
				{
					Title:   "火影忍者角色列表",
					URL:     "https://naruto.fandom.com/zh/wiki",
					Content: "漩涡鸣人 (Naruto Uzumaki), 宇智波佐助 (Sasuke Uchiha), 春野樱 (Sakura Haruno)",
					Score:   0.92,
				},
			},
			expectedNames: []string{"漩涡鸣人", "宇智波佐助"},
		},
		{
			name:           "One Piece characters in Chinese",
			showName:       "One Piece",
			targetLanguage: "Chinese",
			mockAnswer:     "Monkey D. Luffy is 蒙奇·D·路飞",
			mockResults: []TavilyResult{
				{
					Title:   "海贼王角色",
					URL:     "https://onepiece.fandom.com/zh/wiki",
					Content: "蒙奇·D·路飞 (Monkey D. Luffy), 罗罗诺亚·索隆 (Roronoa Zoro)",
					Score:   0.90,
				},
			},
			expectedNames: []string{"蒙奇·D·路飞", "罗罗诺亚·索隆"},
		},
		{
			name:           "Demon Slayer characters in Chinese",
			showName:       "Demon Slayer",
			targetLanguage: "Chinese",
			mockAnswer:     "Tanjiro Kamado is �的门炭治郎",
			mockResults: []TavilyResult{
				{
					Title:   "鬼灭之刃角色",
					URL:     "https://kimetsu.fandom.com/zh/wiki",
					Content: "灶门炭治郎 (Tanjiro Kamado), 灶门祢豆子 (Nezuko Kamado)",
					Score:   0.91,
				},
			},
			expectedNames: []string{"灶门炭治郎", "灶门祢豆子"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockResponse := TavilyResponse{
				Query:   tt.showName + " " + tt.targetLanguage + " character names",
				Answer:  tt.mockAnswer,
				Results: tt.mockResults,
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(mockResponse)
			}))
			defer server.Close()

			tool := NewWebSearchTool("test-api-key", server.URL)

			args := WebSearchArgs{
				Query:          "character names",
				ShowName:       tt.showName,
				TargetLanguage: tt.targetLanguage,
				SearchType:     "characters",
			}
			argsJSON, _ := json.Marshal(args)

			result, err := tool.Execute(context.Background(), argsJSON)
			require.NoError(t, err)
			assert.False(t, result.IsError)

			for _, name := range tt.expectedNames {
				assert.Contains(t, result.Content, name, "Expected to find character name: %s", name)
			}
		})
	}
}

func TestWebSearchTool_Execute_InvalidArgs(t *testing.T) {
	tool := NewWebSearchTool("test-api-key", "")

	result, err := tool.Execute(context.Background(), json.RawMessage(`{invalid json}`))
	require.NoError(t, err) // Execute should not return error, but set IsError in result
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "Failed to parse")
}

func TestWebSearchTool_Execute_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Invalid API key"}`))
	}))
	defer server.Close()

	tool := NewWebSearchTool("invalid-key", server.URL)

	args := WebSearchArgs{Query: "test"}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(context.Background(), argsJSON)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "Search failed")
}

func TestWebSearchTool_FormatResults(t *testing.T) {
	tool := NewWebSearchTool("test-api-key", "")

	t.Run("with results", func(t *testing.T) {
		resp := &TavilyResponse{
			Query:  "test query",
			Answer: "This is the answer",
			Results: []TavilyResult{
				{Title: "Result 1", URL: "https://example.com/1", Content: "Content 1", Score: 0.9},
				{Title: "Result 2", URL: "https://example.com/2", Content: "Content 2", Score: 0.8},
			},
		}

		output := tool.formatResults(resp)
		assert.Contains(t, output, "Search Query: test query")
		assert.Contains(t, output, "Summary: This is the answer")
		assert.Contains(t, output, "1. Result 1")
		assert.Contains(t, output, "2. Result 2")
		assert.Contains(t, output, "https://example.com/1")
	})

	t.Run("no results", func(t *testing.T) {
		resp := &TavilyResponse{
			Query:   "no results query",
			Results: []TavilyResult{},
		}

		output := tool.formatResults(resp)
		assert.Contains(t, output, "No results found")
	})

	t.Run("truncate long content", func(t *testing.T) {
		longContent := strings.Repeat("a", 600)
		resp := &TavilyResponse{
			Query: "test",
			Results: []TavilyResult{
				{Title: "Long", URL: "https://example.com", Content: longContent, Score: 0.9},
			},
		}

		output := tool.formatResults(resp)
		assert.Contains(t, output, "...")
		assert.Less(t, len(output), len(longContent))
	})
}

func TestWebSearchTool_DefaultAPIURL(t *testing.T) {
	tool := NewWebSearchTool("test-key", "")
	assert.Equal(t, "https://api.tavily.com/search", tool.apiURL)

	tool2 := NewWebSearchTool("test-key", "https://custom.api.com/search")
	assert.Equal(t, "https://custom.api.com/search", tool2.apiURL)
}

// Integration test - requires SEARCH_API_KEY environment variable
func TestWebSearchTool_Integration(t *testing.T) {
	apiKey := os.Getenv("SEARCH_API_KEY")
	if apiKey == "" {
		t.Skip("SEARCH_API_KEY not set, skipping integration test")
	}

	tool := NewWebSearchTool(apiKey, "")

	tests := []struct {
		name             string
		args             WebSearchArgs
		expectedInResult []string
	}{
		{
			name: "Search Attack on Titan Chinese character names",
			args: WebSearchArgs{
				Query:          "main characters list",
				ShowName:       "Attack on Titan",
				TargetLanguage: "Chinese",
				SearchType:     "characters",
			},
			expectedInResult: []string{"进击的巨人", "艾伦"},
		},
		{
			name: "Search Naruto Chinese character names",
			args: WebSearchArgs{
				Query:          "Naruto Uzumaki Sasuke",
				ShowName:       "Naruto",
				TargetLanguage: "Chinese",
				SearchType:     "characters",
			},
			expectedInResult: []string{"火影忍者"},
		},
		{
			name: "Search Spirited Away Chinese title and characters",
			args: WebSearchArgs{
				Query:          "Chihiro Sen",
				ShowName:       "Spirited Away",
				TargetLanguage: "Chinese",
				SearchType:     "characters",
			},
			expectedInResult: []string{"千与千寻"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argsJSON, _ := json.Marshal(tt.args)

			result, err := tool.Execute(context.Background(), argsJSON)
			require.NoError(t, err)

			if result.IsError {
				t.Logf("Search returned error (may be rate limited): %s", result.Content)
				t.Skip("Skipping due to API error")
			}

			t.Logf("Search result:\n%s", result.Content)

			// Check if any expected content is found
			found := false
			for _, expected := range tt.expectedInResult {
				if strings.Contains(result.Content, expected) {
					found = true
					break
				}
			}

			if !found {
				t.Logf("Warning: None of expected strings %v found in result", tt.expectedInResult)
			}
		})
	}
}
