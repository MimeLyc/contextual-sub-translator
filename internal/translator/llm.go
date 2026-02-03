package translator

import (
	"context"
	"fmt"
	"strings"

	"github.com/MimeLyc/contextual-sub-translator/internal/agent"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"github.com/MimeLyc/contextual-sub-translator/pkg/log"
)

// agentTranslator is the unified AI layer for all translation
// It uses an agent with tool calling support for enhanced translation quality
type agentTranslator struct {
	agent         *agent.LLMAgent
	searchEnabled bool
}

// NewAgentTranslator creates a new agent-based translator
func NewAgentTranslator(agentInstance *agent.LLMAgent, searchEnabled bool) Translator {
	return &agentTranslator{
		agent:         agentInstance,
		searchEnabled: searchEnabled,
	}
}

func (t *agentTranslator) Translate(
	ctx context.Context,
	media MediaMeta,
	subtitleTexts []string,
	sourceLang string,
	targetLang string,
) ([]string, error) {
	// Build context prompt
	systemPrompt := t.buildContextPrompt(media, sourceLang, targetLang)
	userMessage := strings.Join(subtitleTexts, subtitleLineBreaker)

	// Execute via agent
	result, err := t.agent.Execute(ctx, agent.AgentRequest{
		SystemPrompt: systemPrompt,
		UserMessage:  userMessage,
	})
	if err != nil {
		return nil, fmt.Errorf("agent execution failed: %w", err)
	}

	// Log tool usage
	if len(result.ToolCalls) > 0 {
		log.Info("Agent used %d tool calls in %d iterations", len(result.ToolCalls), result.Iterations)
		for _, tc := range result.ToolCalls {
			log.Info("  - Tool: %s, Error: %v", tc.ToolName, tc.IsError)
		}
	}

	// Parse response
	content := result.Content
	content = strings.ReplaceAll(content, inlineBreakerPlaceholder, "\n")
	return strings.Split(content, subtitleLineBreaker), nil
}

func (t *agentTranslator) BatchTranslate(
	ctx context.Context,
	media MediaMeta,
	subtitleLines []subtitle.Line,
	sourceLanguage string,
	targetLanguage string,
	batchSize int) ([]subtitle.Line, error) {
	if batchSize <= 0 {
		batchSize = 50
	}

	allTranslations, err := t.batchTranslate(ctx, media, subtitleLines, sourceLanguage, targetLanguage, batchSize, 0, len(subtitleLines))
	if err != nil {
		return nil, err
	}

	for i, line := range subtitleLines {
		subtitleLines[i] = subtitle.Line{
			Index:          line.Index,
			StartTime:      line.StartTime,
			EndTime:        line.EndTime,
			Text:           line.Text,
			TranslatedText: allTranslations[i],
		}
	}

	return subtitleLines, nil
}

func (t *agentTranslator) batchTranslate(
	ctx context.Context,
	media MediaMeta,
	subtitleLines []subtitle.Line,
	sourceLanguage string,
	targetLanguage string,
	batchSize int,
	startIncluded int,
	endExcluded int,
) ([]string, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("batch size must be greater than 0")
	}

	var allTranslations []string

	for i := startIncluded; i < endExcluded; i += batchSize {
		end := min(i+batchSize, endExcluded, len(subtitleLines))

		batch := subtitleLines[i:end]

		var subtitleTexts []string
		for _, line := range batch {
			// Deal with original line breaker in subtitle file to avoid LLM misunderstanding
			formattedText := strings.ReplaceAll(line.Text, "\n", inlineBreakerPlaceholder)
			subtitleTexts = append(subtitleTexts, formattedText)
		}

		translations, err := t.Translate(ctx, media, subtitleTexts, sourceLanguage, targetLanguage)
		if err != nil {
			return nil, fmt.Errorf("batch translation failed for lines %d-%d: %w", i+1, end, err)
		}

		if len(translations) != len(subtitleTexts) {
			log.Error("batch translation failed for lines %d-%d: translation count mismatch, retry range with size %d", i+1, end, batchSize/2)
			if translations, err = t.batchTranslate(ctx, media, subtitleLines, sourceLanguage, targetLanguage, batchSize/2, i, end); err != nil {
				return nil, fmt.Errorf("retry batch translation failed for lines %d-%d: %w", i+1, end, err)
			}
		}
		allTranslations = append(allTranslations, translations...)
	}

	return allTranslations, nil
}

// buildContextPrompt builds the system prompt for translation
// Enhanced to instruct the agent to use web_search tool when available
func (t *agentTranslator) buildContextPrompt(
	media MediaMeta,
	sourceLanguage string,
	targetLanguage string,
) string {
	var prompt strings.Builder

	prompt.WriteString("You are a professional subtitle translation expert specializing in cross-language media localization. Translate subtitles from " + sourceLanguage + " to " + targetLanguage + " using comprehensive bidirectional research and precise name mapping.\n\n")

	prompt.WriteString("=== MEDIA INFORMATION ===\n")
	if media.Title != "" {
		prompt.WriteString(fmt.Sprintf("Show Title: %s\n", media.Title))
	}
	if media.OriginalTitle != "" {
		prompt.WriteString(fmt.Sprintf("Original Title: %s\n", media.OriginalTitle))
	}
	if len(media.Genre) > 0 {
		prompt.WriteString(fmt.Sprintf("Genre: %s\n", strings.Join(media.Genre, ", ")))
	}
	if media.Year > 0 {
		prompt.WriteString(fmt.Sprintf("Year: %d\n", media.Year))
	}
	if media.Studio != "" {
		prompt.WriteString(fmt.Sprintf("Production Studio: %s\n", media.Studio))
	}
	if media.Plot != "" {
		prompt.WriteString(fmt.Sprintf("Plot Summary: %s\n", media.Plot))
	}

	// Add tool usage instructions if search is enabled
	if t.searchEnabled {
		prompt.WriteString("\n=== WEB SEARCH TOOL ===\n")
		prompt.WriteString("You have access to a web_search tool. BEFORE translating, use it to:\n")
		prompt.WriteString("1. Search for official " + targetLanguage + " character names for this show\n")
		prompt.WriteString("2. Find official localized place names and terminology\n")
		prompt.WriteString("3. Verify translations against authoritative sources\n")
		prompt.WriteString("\nExample searches:\n")
		if media.Title != "" {
			prompt.WriteString(fmt.Sprintf("- \"%s %s official character names\"\n", media.Title, targetLanguage))
			prompt.WriteString(fmt.Sprintf("- \"%s %s localization wiki\"\n", media.Title, targetLanguage))
		}
		prompt.WriteString("\nUse the discovered official names consistently throughout your translation.\n")
	}

	prompt.WriteString("\n=== TRANSLATION GUIDELINES ===\n")
	prompt.WriteString("1. Apply discovered name mappings consistently across all content\n")
	prompt.WriteString("2. Maintain character voice and relationship dynamics\n")
	prompt.WriteString("3. Ensure " + targetLanguage + " flows naturally while preserving meaning\n")
	prompt.WriteString("4. Keep subtitle length appropriate for screen reading\n")
	prompt.WriteString("5. Preserve original line structure and " + subtitleLineBreaker + " line separators\n")
	prompt.WriteString("6. Preserve " + inlineBreakerPlaceholder + " inline break markers\n")

	prompt.WriteString("\n=== OUTPUT FORMAT ===\n")
	prompt.WriteString("Return ONLY the translated subtitles, one per line, separated by " + subtitleLineBreaker + "\n")
	prompt.WriteString("Do not include any explanations, notes, or additional text.\n")
	prompt.WriteString("The number of output lines must exactly match the number of input lines.\n")

	return prompt.String()
}
