package ctxtrans

import (
	"context"
	"fmt"

	"github.com/MimeLyc/contextual-sub-translator/internal/llm"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"github.com/MimeLyc/contextual-sub-translator/internal/translator"
)

// Library provides the main interface for using ctxtrans as a library
// This is the primary entry point for external packages
var Library = &ctxtransLibrary{}

// ctxtransLibrary implements the library interface
// This struct provides methods for configuring and using the translation functionality
type ctxtransLibrary struct {
	defaultConfig TranslatorConfig
	aiConf        llm.Config
}

// NewTranslator creates a new translator instance with the provided configuration
// This is the main constructor for the library
func (l *ctxtransLibrary) NewTranslator(config TranslatorConfig) (*Translator, error) {
	return NewTranslator(config)
}

// NewTranslatorWithDefaults creates a new translator with sensible defaults
// This is a convenience constructor for common use cases
func (l *ctxtransLibrary) NewTranslatorWithDefaults(language string) (*Translator, error) {
	config := TranslatorConfig{
		TargetLanguage:     language,
		BatchSize:          50,
		ContextEnabled:     true,
		PreserveFormatting: true,
		BackupOriginal:     true,
		Verbose:            false,
	}
	return NewTranslator(config)
}

// NewOpenAIClient creates a new OpenAI client with the provided configuration
// This helper function creates a properly configured LLM client
func (l *ctxtransLibrary) NewOpenAIClient(apiKey string) *OpenAIClient {
	config := DefaultLLMConfig()
	config.APIKey = apiKey
	return NewOpenAIClient(config)
}

// NewOpenAIClientWithConfig creates a new OpenAI client with custom configuration
// This allows for more advanced configuration options
func (l *ctxtransLibrary) NewOpenAIClientWithConfig(config LLMConfig) *OpenAIClient {
	return NewOpenAIClient(config)
}

// TranslateFile performs a complete translation of a single subtitle file
// This is the main high-level interface for single file translation
func (l *ctxtransLibrary) TranslateFile(
	ctx context.Context,
	tvshowNFOPath,
	subtitlePath,
	targetLanguage,
	apiKey string) (*TranslationResult, error) {
	// Create translator with default configuration
	transSvc, err := l.NewTranslatorWithDefaults(targetLanguage)
	if err != nil {
		return nil, fmt.Errorf("failed to create translator: %w", err)
	}

	aiCli, err := llm.NewClient(&l.aiConf)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	// Create and set up LLM client
	client := translator.NewAiTranslator(*aiCli)
	transSvc.SetTranslator(client)

	// Perform translation
	return transSvc.TranslateFile(ctx, tvshowNFOPath, subtitlePath)
}

// QuickTranslate provides a simplified interface for quick single-use translations
// This is designed for simple use cases with minimal configuration
func (l *ctxtransLibrary) QuickTranslate(ctx context.Context, tvshowNFOPath, subtitlePath, targetLanguage, apiKey string) error {
	result, err := l.TranslateFile(ctx, tvshowNFOPath, subtitlePath, targetLanguage, apiKey)
	if err != nil {
		return err
	}

	// Save result to default location
	outputPath := l.generateOutputPath(subtitlePath, targetLanguage)
	writer := subtitle.NewWriter()
	return writer.Write(outputPath, &result.TranslatedFile)
}

// GetLibraryInfo returns information about the library version and capabilities
// This provides metadata for users of the library
func (l *ctxtransLibrary) GetLibraryInfo() LibraryInfo {
	return LibraryInfo{
		Name:        "ctxtrans",
		Version:     "1.0.0",
		Description: "Context-based subtitle translation library",
		Features: []string{
			"Context-aware subtitle translation",
			"Batch processing support",
			"Multiple subtitle formats",
			"Custom LLM configuration",
			"Format preservation",
		},
		SupportedLanguages: []string{"zh", "en", "ja", "ko", "fr", "de", "es"},
		DefaultConfig:      l.defaultConfig,
	}
}

// LibraryInfo contains metadata about the library
// This is useful for library consumers to understand capabilities
type LibraryInfo struct {
	Name               string
	Version            string
	Description        string
	Features           []string
	SupportedLanguages []string
	DefaultConfig      TranslatorConfig
}

// generateOutputPath generates output file path based on input and language
// This is an internal utility function for the library
func (l *ctxtransLibrary) generateOutputPath(inputPath, targetLang string) string {
	// Implementation similar to CLI.generateOutputPath
	// This would handle the same logic but as a library internal function
	// For brevity, using a simple implementation here
	ext := ".srt"
	base := inputPath[:len(inputPath)-len(ext)]
	return base + "." + targetLang + ext
}

// init sets up the default configuration for the library
func init() {
	Library.defaultConfig = TranslatorConfig{
		BatchSize:          50,
		ContextEnabled:     true,
		PreserveFormatting: true,
		BackupOriginal:     true,
		Verbose:            false,
	}
}
