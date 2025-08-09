package ctxtrans

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/MimeLyc/contextual-sub-translator/internal/media"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"github.com/MimeLyc/contextual-sub-translator/internal/translator"
)

// Translator is the core structure for subtitle translator
type Translator struct {
	nfoReader      NFOReader
	subtitleReader subtitle.Reader
	subtitleWriter subtitle.Writer
	translator     translator.Translator
	config         TranslatorConfig
}

// TranslatorConfig contains translator configuration
type TranslatorConfig struct {
	TargetLanguage     string
	BatchSize          int
	ContextEnabled     bool
	PreserveFormatting bool
	OutputPath         string
	BackupOriginal     bool
	Verbose            bool
}

// NewTranslator creates a new translator instance
func NewTranslator(config TranslatorConfig) (*Translator, error) {
	return &Translator{
		nfoReader:      NewNFOReader(),
		subtitleReader: subtitle.NewReader(),
		subtitleWriter: subtitle.NewWriter(),
		config:         config,
	}, nil
}

// SetLLMClient sets the LLM client
func (t *Translator) SetTranslator(cli translator.Translator) {
	t.translator = cli
}

// TranslateFile translates a single subtitle file
func (t *Translator) TranslateFile(ctx context.Context, tvshowNFOPath, subtitlePath string) (*TranslationResult, error) {
	// startTime := time.Now()

	// Read NFO file
	tvShowInfo, err := t.nfoReader.ReadTVShowInfo(tvshowNFOPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read NFO file: %w", err)
	}

	// Read subtitle file
	subtitleFile, err := t.subtitleReader.Read(subtitlePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read subtitle file: %w", err)
	}

	// Use empty info if context is not enabled
	var contextInfo media.TVShowInfo
	if t.config.ContextEnabled {
		contextInfo = *tvShowInfo
	}
	// Perform translation
	translations, err := t.translateSubtitleLines(
		ctx, translator.MediaMeta{
			TVShowInfo: contextInfo,
		}, subtitleFile.Lines)
	if err != nil {
		return nil, fmt.Errorf("failed to translate subtitles: %w", err)
	}

	// Update translation results
	translatedFile := &subtitle.File{
		Lines:    translations,
		Language: t.config.TargetLanguage,
		Format:   subtitleFile.Format,
	}

	// Save translation results if output path is specified
	if t.config.OutputPath != "" {
		if err := t.subtitleWriter.Write(t.config.OutputPath, translatedFile); err != nil {
			return nil, fmt.Errorf("failed to save translation results: %w", err)
		}
	}

	// Create results
	result := &TranslationResult{
		OriginalFile:   *subtitleFile,
		TranslatedFile: *translatedFile,
		// Metadata: TranslationMetadata{
		// 	SourceLanguage:  subtitleFile.Language,
		// 	TargetLanguage:  t.config.TargetLanguage,
		// 	ModelUsed:       "gpt-3.5-turbo", // can be obtained from LLM client
		// 	ContextSummary:  GetContextTextFromTVShow(tvShowInfo),
		// 	TranslationTime: time.Since(startTime),
		// 	CharCount:       countCharacters(subtitleFile.Lines),
		// },
	}

	return result, nil
}

// translateSubtitleLines translates subtitle lines
func (t *Translator) translateSubtitleLines(
	ctx context.Context,
	media translator.MediaMeta,
	lines []subtitle.Line) ([]subtitle.Line, error) {
	if t.translator == nil {
		return nil, fmt.Errorf("Translator not set")
	}

	if len(lines) == 0 {
		return nil, nil
	}

	return t.translator.BatchTranslate(
		ctx,
		media,
		lines,
		t.config.TargetLanguage,
		t.config.BatchSize)
}

// TranslateMultiple translates multiple subtitle files
func (t *Translator) TranslateMultiple(ctx context.Context, tvshowNFOPath string, subtitlePaths []string) ([]*TranslationResult, error) {
	var results []*TranslationResult

	for _, subtitlePath := range subtitlePaths {
		if t.config.Verbose {
			log.Printf("Translating file: %s", subtitlePath)
		}

		result, err := t.TranslateFile(ctx, tvshowNFOPath, subtitlePath)
		if err != nil {
			if t.config.Verbose {
				log.Printf("Failed to translate file %s: %v", subtitlePath, err)
			}
			continue
		}

		results = append(results, result)

		if t.config.Verbose {
			log.Printf("Successfully translated %s: %d subtitle lines", subtitlePath, len(result.TranslatedFile.Lines))
		}
	}

	return results, nil
}

// ValidateInputs validates input file existence
func (t *Translator) ValidateInputs(tvshowNFOPath, subtitlePath string) error {
	if tvshowNFOPath == "" {
		return fmt.Errorf("NFO file path cannot be empty")
	}
	if subtitlePath == "" {
		return fmt.Errorf("subtitle file path cannot be empty")
	}

	return nil
}

// BackupFile backs up original file
func (t *Translator) BackupFile(originalPath string) (string, error) {
	if !t.config.BackupOriginal {
		return "", nil
	}

	backupPath := originalPath + ".backup"
	// Should implement actual file backup logic here
	// Simplified version returns backup path only
	return backupPath, nil
}

// PrintTranslationReport prints translation report
func (t *Translator) PrintTranslationReport(result *TranslationResult) {
	fmt.Println("=== Translation Report ===")
	fmt.Printf("Source Language: %s\n", result.Metadata.SourceLanguage)
	fmt.Printf("Target Language: %s\n", result.Metadata.TargetLanguage)
	fmt.Printf("Translation Time: %v\n", result.Metadata.TranslationTime)
	fmt.Printf("Character Count: %d\n", result.Metadata.CharCount)
	fmt.Printf("Subtitle Lines: %d\n", len(result.TranslatedFile.Lines))

	if t.config.ContextEnabled {
		fmt.Println("\n=== Context Information Used ===")
		fmt.Println("Context translation enabled")
	}
}

// GetTranslationPreview gets translation preview (first 5 lines)
func (t *Translator) GetTranslationPreview(result *TranslationResult, lines int) string {
	if lines <= 0 {
		lines = 5
	}

	var sb strings.Builder
	sb.WriteString("=== Translation Preview ===\n")

	showLines := lines
	if len(result.TranslatedFile.Lines) < showLines {
		showLines = len(result.TranslatedFile.Lines)
	}

	for i := 0; i < showLines; i++ {
		original := result.OriginalFile.Lines[i].Text
		translated := result.TranslatedFile.Lines[i].TranslatedText

		sb.WriteString(fmt.Sprintf("Original: %s\n", original))
		sb.WriteString(fmt.Sprintf("Translated: %s\n\n", translated))
	}

	return sb.String()
}

// countCharacters calculates total subtitle characters
func countCharacters(lines []subtitle.Line) int {
	total := 0
	for _, line := range lines {
		total += len(line.Text)
	}
	return total
}

// FormatLanguageCode formats language code
func FormatLanguageCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	switch code {
	case "zh", "zh-cn", "zh_cn":
		return "Chinese"
	case "en":
		return "English"
	case "ja":
		return "Japanese"
	case "ko":
		return "Korean"
	case "fr":
		return "French"
	case "de":
		return "German"
	case "es":
		return "Spanish"
	default:
		return strings.ToUpper(code)
	}
}
