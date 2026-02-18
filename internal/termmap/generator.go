package termmap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MimeLyc/contextual-sub-translator/internal/agent"
	"github.com/MimeLyc/contextual-sub-translator/internal/media"
)

// Generator generates term maps using an LLM agent with web search.
type Generator struct {
	agent *agent.LLMAgent
}

// NewGenerator creates a new term map generator.
func NewGenerator(a *agent.LLMAgent) *Generator {
	return &Generator{agent: a}
}

// Generate uses the LLM agent to generate a term map for the given show.
func (g *Generator) Generate(ctx context.Context, showInfo media.TVShowInfo, sourceLang, targetLang string) (TermMap, error) {
	systemPrompt := buildGeneratorPrompt(showInfo, sourceLang, targetLang)
	userMessage := fmt.Sprintf(
		"Research %q and return a JSON object mapping %s terms to %s. "+
			"Respond with ONLY the JSON object, starting with { and ending with }.",
		showInfo.Title, sourceLang, targetLang,
	)

	result, err := g.agent.Execute(ctx, agent.AgentRequest{
		SystemPrompt: systemPrompt,
		UserMessage:  userMessage,
	})
	if err != nil {
		return nil, fmt.Errorf("agent execution failed: %w", err)
	}

	// If the agent gathered search results but returned empty content,
	// make a follow-up call without tools to compile the findings.
	if strings.TrimSpace(result.Content) == "" && len(result.ToolCalls) > 0 {
		result, err = g.compileSearchResults(ctx, showInfo, sourceLang, targetLang, result.ToolCalls)
		if err != nil {
			return nil, fmt.Errorf("compile search results failed: %w", err)
		}
	}

	tm, parseErr := parseTermMapResponse(result.Content)
	if parseErr != nil {
		return nil, fmt.Errorf("%w\nraw response:\n%s", parseErr, result.Content)
	}
	return tm, nil
}

// compileSearchResults takes raw search tool outputs and asks the LLM (without tools)
// to compile them into a JSON term map.
func (g *Generator) compileSearchResults(
	ctx context.Context,
	showInfo media.TVShowInfo,
	sourceLang, targetLang string,
	toolCalls []agent.ToolCallRecord,
) (*agent.AgentResult, error) {
	var searchData strings.Builder
	for _, tc := range toolCalls {
		if !tc.IsError {
			searchData.WriteString(tc.Result)
			searchData.WriteString("\n---\n")
		}
	}

	systemPrompt := fmt.Sprintf(
		"You compile search results into a JSON term map. "+
			"Output ONLY a JSON object mapping %s terms to %s terms for the show %q. "+
			"No markdown, no explanations. Start with { and end with }.",
		sourceLang, targetLang, showInfo.Title,
	)
	userMessage := fmt.Sprintf(
		"Here are search results about %q. Extract all %s -> %s term mappings "+
			"(character names, places, terminology) and return them as a single JSON object. "+
			"Do NOT call any tools; directly produce the JSON object from the provided search results:\n\n%s",
		showInfo.Title, sourceLang, targetLang, searchData.String(),
	)

	// Use a separate agent call without tools so the LLM must produce text.
	return g.agent.Execute(ctx, agent.AgentRequest{
		SystemPrompt:  systemPrompt,
		UserMessage:   userMessage,
		MaxIterations: 3,
	})
}

// ExtractNewTerms extracts new term mappings from tool calls and filters out existing terms.
// It makes a single LLM call (without tools) to parse search results into term mappings.
func (g *Generator) ExtractNewTerms(
	ctx context.Context,
	toolCalls []agent.ToolCallRecord,
	existingTerms TermMap,
	showTitle, sourceLang, targetLang string,
) (TermMap, error) {
	// Collect non-error web_search results
	var searchData strings.Builder
	hasResults := false
	for _, tc := range toolCalls {
		if tc.ToolName == "web_search" && !tc.IsError && tc.Result != "" {
			searchData.WriteString(tc.Result)
			searchData.WriteString("\n---\n")
			hasResults = true
		}
	}

	if !hasResults {
		return TermMap{}, nil
	}

	// Make LLM call to extract term mappings (MaxIterations: 1, no tools)
	systemPrompt := fmt.Sprintf(
		"You extract term mappings from search results. "+
			"Output ONLY a JSON object mapping %s terms to %s terms for the show %q. "+
			"No markdown, no explanations. Start with { and end with }.",
		sourceLang, targetLang, showTitle,
	)
	userMessage := fmt.Sprintf(
		"Here are search results about %q. Extract all %s -> %s term mappings "+
			"(character names, places, terminology) and return them as a single JSON object. "+
			"Do NOT call any tools; directly produce the JSON object from the provided search results:\n\n%s",
		showTitle, sourceLang, targetLang, searchData.String(),
	)

	result, err := g.agent.Execute(ctx, agent.AgentRequest{
		SystemPrompt:  systemPrompt,
		UserMessage:   userMessage,
		MaxIterations: 3,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to extract terms from search results: %w", err)
	}

	// Parse the response into a TermMap
	parsed, err := parseTermMapResponse(result.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse extracted terms: %w", err)
	}

	// Filter out keys already present in existingTerms
	newTerms := make(TermMap)
	for key, value := range parsed {
		if _, exists := existingTerms[key]; !exists {
			newTerms[key] = value
		}
	}

	return newTerms, nil
}

func buildGeneratorPrompt(showInfo media.TVShowInfo, sourceLang, targetLang string) string {
	var prompt strings.Builder

	prompt.WriteString("You are a term-mapping machine. You research a TV show and output ONLY a flat JSON object — no markdown, no tables, no prose, no explanations.\n\n")

	prompt.WriteString("=== SHOW INFORMATION ===\n")
	if showInfo.Title != "" {
		prompt.WriteString(fmt.Sprintf("Title: %s\n", showInfo.Title))
	}
	if showInfo.OriginalTitle != "" {
		prompt.WriteString(fmt.Sprintf("Original Title: %s\n", showInfo.OriginalTitle))
	}
	if len(showInfo.Genre) > 0 {
		prompt.WriteString(fmt.Sprintf("Genre: %s\n", strings.Join(showInfo.Genre, ", ")))
	}
	if showInfo.Year > 0 {
		prompt.WriteString(fmt.Sprintf("Year: %d\n", showInfo.Year))
	}
	if showInfo.Studio != "" {
		prompt.WriteString(fmt.Sprintf("Studio: %s\n", showInfo.Studio))
	}
	if len(showInfo.Actors) > 0 {
		prompt.WriteString("Cast:\n")
		for _, actor := range showInfo.Actors {
			if actor.Role != "" {
				prompt.WriteString(fmt.Sprintf("  - %s as %s\n", actor.Name, actor.Role))
			} else {
				prompt.WriteString(fmt.Sprintf("  - %s\n", actor.Name))
			}
		}
	}

	prompt.WriteString("\n=== TASK ===\n")
	prompt.WriteString("Use web_search to find official " + targetLang + " translations for character names, place names, and key terminology of this show.\n\n")

	prompt.WriteString("=== RESPONSE FORMAT (MANDATORY) ===\n")
	prompt.WriteString("After your research, your final message must contain ONLY a JSON object like this:\n")
	prompt.WriteString("{\"" + sourceLang + " term\": \"" + targetLang + " term\", ...}\n\n")
	prompt.WriteString("RULES:\n")
	prompt.WriteString("- Keys are " + sourceLang + " terms, values are " + targetLang + " terms.\n")
	prompt.WriteString("- One value per key (pick the most widely-used official translation).\n")
	prompt.WriteString("- NO markdown, NO tables, NO bullet lists, NO explanations.\n")
	prompt.WriteString("- The response must start with { and end with }.\n")

	return prompt.String()
}

// parseTermMapResponse parses the LLM response into a TermMap.
// Handles clean JSON, markdown code fences, and JSON embedded in prose.
func parseTermMapResponse(content string) (TermMap, error) {
	content = strings.TrimSpace(content)

	// Try direct parse first
	var tm TermMap
	if err := json.Unmarshal([]byte(content), &tm); err == nil {
		return tm, nil
	}

	// Try extracting from markdown code fences
	if idx := strings.Index(content, "```"); idx >= 0 {
		inner := content[idx+3:]
		// Skip language tag on the same line (e.g., ```json)
		if nl := strings.Index(inner, "\n"); nl >= 0 {
			inner = inner[nl+1:]
		}
		if end := strings.Index(inner, "```"); end >= 0 {
			inner = inner[:end]
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(inner)), &tm); err == nil {
			return tm, nil
		}
	}

	// Try finding the outermost { ... } JSON object in the text
	if extracted := extractJSONObject(content); extracted != "" {
		if err := json.Unmarshal([]byte(extracted), &tm); err == nil {
			return tm, nil
		}
	}

	return nil, fmt.Errorf("failed to parse term map JSON from response: no valid JSON object found")
}

// extractJSONObject finds the outermost balanced { ... } block in s.
func extractJSONObject(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
