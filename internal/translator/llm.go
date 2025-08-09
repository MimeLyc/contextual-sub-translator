package translator

import (
	"context"
	"fmt"
	"strings"

	"github.com/MimeLyc/contextual-sub-translator/internal/llm"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
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
	targetLang string,
) ([]string, error) {
	// Build context prompt
	contextPrompt := c.buildContextPrompt(media, targetLang)

	return c.call(
		ctx,
		[]llm.Message{
			{
				Role:    "system",
				Content: contextPrompt,
			},
			{
				Role:    "user",
				Content: strings.Join(subtitleTexts, "%line_breaker%"),
			},
		})
}

func (c aiTranslator) BatchTranslate(
	ctx context.Context,
	media MediaMeta,
	subtitleLines []subtitle.Line,
	targetLanguage string,
	batchSize int) ([]subtitle.Line, error) {
	if batchSize <= 0 {
		batchSize = 50
	}

	var allTranslations []string

	for i := 0; i < len(subtitleLines); i += batchSize {
		end := i + batchSize
		if end > len(subtitleLines) {
			end = len(subtitleLines)
		}

		batch := subtitleLines[i:end]

		var subtitleTexts []string
		for _, line := range batch {
			// Deal with original line breaker in subtitle file to avoid LLM misunderstanding
			formattedText := strings.ReplaceAll(line.Text, "\n", "$$legacy_lb$$")
			subtitleTexts = append(subtitleTexts, formattedText)
		}

		translations, err := c.Translate(ctx, media, subtitleTexts, targetLanguage)
		if err != nil {
			return nil, fmt.Errorf("batch translation failed for lines %d-%d: %w", i+1, end, err)
		}

		if len(translations) != len(subtitleTexts) {
			return nil, fmt.Errorf("batch translation failed for lines %d-%d: translation count mismatch", i+1, end)
		}
		allTranslations = append(allTranslations, translations...)
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

func (l aiTranslator) buildContextPrompt(
	media MediaMeta,
	targetLanguage string,
) string {
	var prompt strings.Builder

	prompt.WriteString("You are a professional subtitle translation expert. Please translate the subtitle content into ")
	prompt.WriteString(targetLanguage)
	prompt.WriteString(" based on the following media information:\n\n")

	if media.Title != "" {
		prompt.WriteString(fmt.Sprintf("Show Title: %s\n", media.Title))
	}
	if media.OriginalTitle != "" && media.OriginalTitle != media.Title {
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
		prompt.WriteString(fmt.Sprintf("\nPlot Summary: %s\n", media.Plot))
	}

	prompt.WriteString("\nTranslation Requirements:\n")
	prompt.WriteString("1. Maintain colloquial and natural expression of subtitles\n")
	prompt.WriteString("2. Pay attention to the tone and emotional expression of character dialogue\n")
	prompt.WriteString("3. Avoid excessively long sentences, maintain readability\n")
	prompt.WriteString("4. Appropriately localize culturally specific content\n")
	prompt.WriteString("5. Maintain subtitle time synchronization\n")
	prompt.WriteString("6. Keep the line break format of the text I sent, leave **any** words surrounded by '%' and '$$' unchanged' .\n")

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
	content = strings.ReplaceAll(content, "$$legacy_lb$$", "\n")
	return strings.Split(content, "%line_breaker%"), nil
}

func (l aiTranslator) buildMetaFile(
	ctx context.Context,
	messages []llm.Message,
) (llm.File, error) {
	return llm.File{}, nil
}
