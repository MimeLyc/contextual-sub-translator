package ctxtrans

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Utility functions for library consumers

// GenerateOutputPath generates output file path based on input file and target language
// This utility function helps library consumers generate appropriate output paths
func GenerateOutputPath(inputPath, targetLang string) string {
	ext := filepath.Ext(inputPath)
	base := strings.TrimSuffix(inputPath, ext)
	langCode := strings.ToLower(targetLang)

	// Handle common language codes
	langMap := map[string]string{
		"chinese":  "zh",
		"english":  "en",
		"japanese": "ja",
		"korean":   "ko",
		"french":   "fr",
		"german":   "de",
		"spanish":  "es",
	}

	if mapped, ok := langMap[langCode]; ok {
		langCode = mapped
	}

	return fmt.Sprintf("%s.%s%s", base, langCode, ext)
}

// GetSupportedLanguages returns a list of supported language codes
// This helps library consumers understand what languages are supported
func GetSupportedLanguages() []string {
	return []string{"zh", "en", "ja", "ko", "fr", "de", "es"}
}

// GetLanguageName returns the human-readable name for a language code
// This complements the FormatLanguageCode function from translator.go
func GetLanguageName(code string) string {
	return FormatLanguageCode(code)
}

// DefaultConfig returns the default translator configuration
// This provides library consumers with sensible defaults
func DefaultConfig() TranslatorConfig {
	return TranslatorConfig{
		BatchSize:          50,
		ContextEnabled:     true,
		PreserveFormatting: true,
		BackupOriginal:     true,
		Verbose:            false,
	}
}

// ValidateInputs validates input file paths for basic requirements
// This utility function helps library consumers validate inputs
func ValidateInputs(tvshowNFOPath, subtitlePath string) error {
	if tvshowNFOPath == "" {
		return fmt.Errorf("NFO file path cannot be empty")
	}
	if subtitlePath == "" {
		return fmt.Errorf("subtitle file path cannot be empty")
	}
	return nil
}

// NewEnvironmentConfig creates a TranslatorConfig from environment variables
// This allows library consumers to use environment-based configuration
func NewEnvironmentConfig() TranslatorConfig {
	config := DefaultConfig()

	// Environment variables could be used here if needed
	// For now, returning default config

	return config
}

// BatchConfig provides configuration for batch processing
// This helps library consumers configure batch operations
func BatchConfig() TranslatorConfig {
	config := DefaultConfig()
	config.BatchSize = 30 // Smaller batch size for batch processing
	config.Verbose = true // Enable verbose mode for batch operations
	return config
}
