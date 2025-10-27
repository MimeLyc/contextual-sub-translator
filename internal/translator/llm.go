package translator

import (
	"context"
	"fmt"
	"strings"

	"github.com/MimeLyc/contextual-sub-translator/internal/llm"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"github.com/MimeLyc/contextual-sub-translator/pkg/log"
)

type aiTranslator struct {
	cli llm.Client
}

func NewAiTranslator(cli llm.Client) Translator {
	return aiTranslator{cli: cli}
}

func (c aiTranslator) Translate(
	ctx context.Context,
	media MediaMeta,
	subtitleTexts []string,
	sourceLang string,
	targetLang string,
) ([]string, error) {
	// Build context prompt
	contextPrompt := c.buildContextPrompt(media, sourceLang, targetLang)

	return c.call(
		ctx,
		[]llm.Message{
			{
				Role:    "system",
				Content: contextPrompt,
			},
			{
				Role:    "user",
				Content: strings.Join(subtitleTexts, subtitleLineBreaker),
			},
		})
}

func (c aiTranslator) BatchTranslate(
	ctx context.Context,
	media MediaMeta,
	subtitleLines []subtitle.Line,
	sourceLanguage string,
	targetLanguage string,
	batchSize int) ([]subtitle.Line, error) {
	if batchSize <= 0 {
		batchSize = 50
	}

	allTranslations, err := c.batchTranslate(ctx, media, subtitleLines, sourceLanguage, targetLanguage, batchSize, 0, len(subtitleLines))
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

func (c aiTranslator) batchTranslate(
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

		translations, err := c.Translate(ctx, media, subtitleTexts, sourceLanguage, targetLanguage)
		if err != nil {
			return nil, fmt.Errorf("batch translation failed for lines %d-%d: %w", i+1, end, err)
		}

		if len(translations) != len(subtitleTexts) {
			log.Error("batch translation failed for lines %d-%d: translation count mismatch, retry range with size %d", i+1, end, batchSize/2)
			if translations, err = c.batchTranslate(ctx, media, subtitleLines, sourceLanguage, targetLanguage, batchSize/2, i, end); err != nil {
				return nil, fmt.Errorf("retry batch translation failed for lines %d-%d: %w", i+1, end, err)
			}
		}
		allTranslations = append(allTranslations, translations...)
	}

	return allTranslations, nil
}

// Build context prompt
// This is where magic happens to enhance the translation quality.
func (l aiTranslator) buildContextPrompt(
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

	prompt.WriteString("\n=== AUTONOMOUS DISCOVERY SYSTEM ===\n")
	prompt.WriteString("Dynamically discover and search appropriate databases for both languages:\n")
	prompt.WriteString("\nDiscovery Protocol:\n")
	prompt.WriteString("- Source Language: Search autonomous databases for " + sourceLanguage + " terms\n")
	prompt.WriteString("- Target Language: Identify official localization through discovered sources\n")
	prompt.WriteString("- Cross-reference mapping: Build precise name mappings from discovered databases\n")
	prompt.WriteString("- Validation: Verify through multiple authoritative sources\n")

	prompt.WriteString("\n=== CROSS-LANGUAGE VERIFICATION PROCESS ===\n")
	prompt.WriteString("1. Primary Search: Find official " + targetLanguage + " versions on specified platforms\n")
	prompt.WriteString("2. Cross-Validation: Verify names/terms across multiple " + targetLanguage + " sources\n")
	prompt.WriteString("3. Back-Verification: Check if mapped names still make sense in original context\n")
	prompt.WriteString("4. Consistency Check: Ensure all instances use the standardized mapping\n")

	prompt.WriteString("\n=== TRANSLATION EXECUTION ===\n")
	prompt.WriteString("When translating:\n")
	prompt.WriteString("1. Apply discovered name mappings consistently across all content\n")
	prompt.WriteString("2. Maintain character voice and relationship dynamics\n")
	prompt.WriteString("3. Ensure " + targetLanguage + " flows naturally while preserving meaning\n")
	prompt.WriteString("4. Keep subtitle length appropriate for screen reading\n")
	prompt.WriteString("5. Preserve original line structure and %%special%% formatting\n")

	return prompt.String()
}

func (l aiTranslator) call(
	ctx context.Context,
	messages []llm.Message,
) ([]string, error) {
	resp, err := l.cli.ChatCompletion(ctx, messages, nil)
	if err != nil {
		return nil, err
	}
	content := resp.Choices[0].Message.Content
	content = strings.ReplaceAll(content, inlineBreakerPlaceholder, "\n")
	return strings.Split(content, subtitleLineBreaker), nil
}
