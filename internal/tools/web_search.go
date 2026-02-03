package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebSearchTool implements web search using Tavily API
type WebSearchTool struct {
	apiKey     string
	apiURL     string
	httpClient *http.Client
}

// WebSearchArgs represents the arguments for web search
type WebSearchArgs struct {
	Query          string `json:"query"`
	ShowName       string `json:"show_name,omitempty"`
	TargetLanguage string `json:"target_language,omitempty"`
	SearchType     string `json:"search_type,omitempty"` // terminology, characters, places, all
}

// TavilyRequest represents a request to Tavily API
type TavilyRequest struct {
	APIKey           string   `json:"api_key"`
	Query            string   `json:"query"`
	SearchDepth      string   `json:"search_depth,omitempty"`
	IncludeAnswer    bool     `json:"include_answer,omitempty"`
	IncludeRawContent bool    `json:"include_raw_content,omitempty"`
	MaxResults       int      `json:"max_results,omitempty"`
	IncludeDomains   []string `json:"include_domains,omitempty"`
}

// TavilyResponse represents a response from Tavily API
type TavilyResponse struct {
	Query   string         `json:"query"`
	Answer  string         `json:"answer,omitempty"`
	Results []TavilyResult `json:"results"`
}

// TavilyResult represents a single search result
type TavilyResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// NewWebSearchTool creates a new web search tool
func NewWebSearchTool(apiKey, apiURL string) *WebSearchTool {
	if apiURL == "" {
		apiURL = "https://api.tavily.com/search"
	}
	return &WebSearchTool{
		apiKey: apiKey,
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (t *WebSearchTool) Name() string {
	return "web_search"
}

func (t *WebSearchTool) Description() string {
	return `Search the web for official terminology, character names, and place names in the target language.
Use this tool to find:
- Official localized character names for TV shows, movies, and anime
- Official place name translations
- Domain-specific terminology in the target language
- Verified translations from authoritative sources

The tool returns search results that can help ensure translation accuracy and consistency.`
}

func (t *WebSearchTool) Parameters() json.RawMessage {
	schema := `{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "The search query to find terminology or names. Be specific and include the show/movie name and target language."
			},
			"show_name": {
				"type": "string",
				"description": "The name of the show, movie, or media (optional, helps contextualize search)"
			},
			"target_language": {
				"type": "string",
				"description": "The target language for finding localized names (e.g., 'Chinese', 'Japanese', 'Korean')"
			},
			"search_type": {
				"type": "string",
				"enum": ["terminology", "characters", "places", "all"],
				"description": "Type of search: 'terminology' for technical terms, 'characters' for character names, 'places' for location names, 'all' for comprehensive search"
			}
		},
		"required": ["query"]
	}`
	return json.RawMessage(schema)
}

func (t *WebSearchTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	var searchArgs WebSearchArgs
	if err := json.Unmarshal(args, &searchArgs); err != nil {
		return ToolResult{
			Content: fmt.Sprintf("Failed to parse search arguments: %v", err),
			IsError: true,
		}, nil
	}

	// Build the search query
	query := t.buildQuery(searchArgs)

	// Make the API request
	results, err := t.search(ctx, query)
	if err != nil {
		return ToolResult{
			Content: fmt.Sprintf("Search failed: %v", err),
			IsError: true,
		}, nil
	}

	// Format results
	content := t.formatResults(results)
	return ToolResult{
		Content: content,
		IsError: false,
	}, nil
}

func (t *WebSearchTool) buildQuery(args WebSearchArgs) string {
	query := args.Query

	// Enhance query based on search type and context
	if args.ShowName != "" && args.TargetLanguage != "" {
		switch args.SearchType {
		case "characters":
			query = fmt.Sprintf("%s %s official character names %s localization", args.ShowName, args.TargetLanguage, query)
		case "places":
			query = fmt.Sprintf("%s %s official place names locations %s", args.ShowName, args.TargetLanguage, query)
		case "terminology":
			query = fmt.Sprintf("%s %s official terminology translation %s", args.ShowName, args.TargetLanguage, query)
		default:
			query = fmt.Sprintf("%s %s official translation %s", args.ShowName, args.TargetLanguage, query)
		}
	}

	return query
}

func (t *WebSearchTool) search(ctx context.Context, query string) (*TavilyResponse, error) {
	request := TavilyRequest{
		APIKey:        t.apiKey,
		Query:         query,
		SearchDepth:   "basic",
		IncludeAnswer: true,
		MaxResults:    5,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", t.apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var tavilyResp TavilyResponse
	if err := json.Unmarshal(body, &tavilyResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &tavilyResp, nil
}

func (t *WebSearchTool) formatResults(resp *TavilyResponse) string {
	var result bytes.Buffer

	result.WriteString(fmt.Sprintf("Search Query: %s\n\n", resp.Query))

	if resp.Answer != "" {
		result.WriteString(fmt.Sprintf("Summary: %s\n\n", resp.Answer))
	}

	if len(resp.Results) == 0 {
		result.WriteString("No results found.\n")
		return result.String()
	}

	result.WriteString("Search Results:\n")
	for i, r := range resp.Results {
		result.WriteString(fmt.Sprintf("\n%d. %s\n", i+1, r.Title))
		result.WriteString(fmt.Sprintf("   URL: %s\n", r.URL))
		// Truncate content if too long
		content := r.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		result.WriteString(fmt.Sprintf("   Content: %s\n", content))
	}

	return result.String()
}
