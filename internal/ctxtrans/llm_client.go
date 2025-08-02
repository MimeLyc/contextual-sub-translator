package ctxtrans

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// LLMConfig represents LLM client configuration
type LLMConfig struct {
	APIKey    string
	BaseURL   string
	Model     string
	Timeout   time.Duration
	MaxTokens int
}

// DefaultLLMConfig returns default LLM configuration
func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		BaseURL:   "https://api.openai.com/v1",
		Model:     "gpt-3.5-turbo",
		Timeout:   30 * time.Second,
		MaxTokens: 2048,
	}
}

// OpenAIClient implements LLMClient interface for OpenAI API
type OpenAIClient struct {
	config LLMConfig
	client *http.Client
}

// NewOpenAIClient creates a new OpenAI client
func NewOpenAIClient(config LLMConfig) *OpenAIClient {
	return &OpenAIClient{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// TranslateWithContext translates subtitle lines with context
func (c *OpenAIClient) TranslateWithContext(ctx context.Context, contextInfo TVShowInfo, subtitleLines []SubtitleLine, targetLanguage string) ([]string, error) {
	// Build context prompt
	contextPrompt := c.buildContextPrompt(contextInfo, targetLanguage)

	// Build subtitle text
	var subtitleTexts []string
	for _, line := range subtitleLines {
		subtitleTexts = append(subtitleTexts, line.Text)
	}

	// Create messages for the LLM
	messages := []map[string]interface{}{
		{
			"role":    "system",
			"content": contextPrompt,
		},
		{
			"role": "user",
			"content": fmt.Sprintf("Please translate the following subtitle content to %s:\n%s",
				formatTargetLanguage(targetLanguage),
				strings.Join(subtitleTexts, "\n")),
		},
	}

	// Make API request
	return c.callAPI(ctx, messages, len(subtitleLines))
}

// BatchTranslateWithContext translates subtitle lines in batches with context
func (c *OpenAIClient) BatchTranslateWithContext(ctx context.Context, contextInfo TVShowInfo, subtitleLines []SubtitleLine, targetLanguage string, batchSize int) ([]string, error) {
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
		translations, err := c.TranslateWithContext(ctx, contextInfo, batch, targetLanguage)
		if err != nil {
			return nil, fmt.Errorf("batch translation failed for lines %d-%d: %w", i+1, end, err)
		}

		allTranslations = append(allTranslations, translations...)
	}

	return allTranslations, nil
}

// buildContextPrompt builds context prompt from TV show information
func (c *OpenAIClient) buildContextPrompt(contextInfo TVShowInfo, targetLanguage string) string {
	var prompt strings.Builder

	prompt.WriteString("You are a professional subtitle translation expert. Please translate the subtitle content into ")
	prompt.WriteString(formatTargetLanguage(targetLanguage))
	prompt.WriteString(" based on the following media information:\n\n")

	if contextInfo.Title != "" {
		prompt.WriteString(fmt.Sprintf("Show Title: %s\n", contextInfo.Title))
	}
	if contextInfo.OriginalTitle != "" && contextInfo.OriginalTitle != contextInfo.Title {
		prompt.WriteString(fmt.Sprintf("Original Title: %s\n", contextInfo.OriginalTitle))
	}
	if len(contextInfo.Genre) > 0 {
		prompt.WriteString(fmt.Sprintf("Genre: %s\n", strings.Join(contextInfo.Genre, ", ")))
	}
	if contextInfo.Year > 0 {
		prompt.WriteString(fmt.Sprintf("Year: %d\n", contextInfo.Year))
	}
	if contextInfo.Studio != "" {
		prompt.WriteString(fmt.Sprintf("Production Studio: %s\n", contextInfo.Studio))
	}
	if len(contextInfo.Actors) > 0 {
		prompt.WriteString("Main Cast:\n")
		for i, actor := range contextInfo.Actors {
			if i >= 3 {
				break
			}
			if actor.Role != "" {
				prompt.WriteString(fmt.Sprintf("- %s as %s\n", actor.Name, actor.Role))
			} else {
				prompt.WriteString(fmt.Sprintf("- %s\n", actor.Name))
			}
		}
	}
	if contextInfo.Plot != "" {
		prompt.WriteString(fmt.Sprintf("\nPlot Summary: %s\n", contextInfo.Plot))
	}

	prompt.WriteString("\nTranslation Requirements:\n")
	prompt.WriteString("1. Maintain colloquial and natural expression of subtitles\n")
	prompt.WriteString("2. Pay attention to the tone and emotional expression of character dialogue\n")
	prompt.WriteString("3. Avoid excessively long sentences, maintain readability\n")
	prompt.WriteString("4. Appropriately localize culturally specific content\n")
	prompt.WriteString("5. Maintain subtitle time synchronization\n")

	return prompt.String()
}

// formatTargetLanguage formats target language for prompt
func formatTargetLanguage(targetLanguage string) string {
	langMap := map[string]string{
		"zh": "中文",
		"en": "英文",
		"ja": "日文",
		"ko": "韩文",
		"fr": "法文",
		"de": "德文",
		"es": "西班牙文",
	}

	if lang, ok := langMap[targetLanguage]; ok {
		return lang
	}
	return targetLanguage
}

// callAPI makes the actual API call to OpenAI
func (c *OpenAIClient) callAPI(ctx context.Context, messages []map[string]interface{}, expectedCount int) ([]string, error) {
	// This is a placeholder implementation
	// In a real implementation, this would make the actual HTTP request to OpenAI API
	// For now, we'll return mock translations

	// Extract text content from the last user message
	var texts []string
	for _, msg := range messages {
		if msg["role"] == "user" {
			content := msg["content"].(string)
			lines := strings.Split(content, "\n")

			// Skip the "请翻译以下字幕内容为..." line
			for i := 1; i < len(lines); i++ {
				if lines[i] != "" {
					texts = append(texts, lines[i])
				}
			}
			break
		}
	}

	// Return mock translations (in real implementation, this would be from OpenAI)
	var translations []string
	for _, text := range texts {
		if text != "" {
			// Simple mock translation - just add [译] prefix
			translations = append(translations, "[译] "+text)
		} else {
			translations = append(translations, "")
		}
	}

	return translations, nil
}

// Request and Response structures for OpenAI API
type OpenAIRequest struct {
	Model       string                   `json:"model"`
	Messages    []map[string]interface{} `json:"messages"`
	MaxTokens   int                      `json:"max_tokens"`
	Temperature float64                  `json:"temperature"`
}

type OpenAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
