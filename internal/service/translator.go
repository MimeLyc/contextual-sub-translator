package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/MimeLyc/contextual-sub-translator/internal/media"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"github.com/MimeLyc/contextual-sub-translator/internal/translator"
	"golang.org/x/text/language"
)

// TranslatorConfig contains translator configuration
type TranslatorConfig struct {
	TargetLanguage language.Tag
	BatchSize      int
	ContextEnabled bool
	InputPath      string
	SubtitleFile   *subtitle.File

	OutputDir  string
	OutputName string
	// BackupOriginal bool
	Verbose bool
}

func (c TranslatorConfig) OutputPath() string {
	outputDir := c.OutputDir
	if outputDir == "" {
		outputDir = filepath.Dir(c.InputPath)
	}
	outputName := c.OutputName
	if outputName == "" {
		base := filepath.Base(c.InputPath)
		ext := filepath.Ext(c.InputPath)
		outputName = filepath.Join(base, ".ctxtrans."+c.TargetLanguage.String()+ext)
	}
	return filepath.Join(outputDir, outputName)
}

type SubTranslator struct {
	nfoReader NFOReader

	subtitleWriter subtitle.Writer
	translator     translator.Translator
	config         TranslatorConfig
	file           *subtitle.File
}

// Translate translates a single subtitle file
func (t *SubTranslator) Translate(
	ctx context.Context,
	tvshowNFOPath string,
) (*TranslationResult, error) {
	// setup outputPath
	outputPath := t.config.OutputPath()

	// Read NFO file
	tvShowInfo, err := t.nfoReader.ReadTVShowInfo(tvshowNFOPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read NFO file: %w", err)
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
		}, t.file.Lines)
	if err != nil {
		return nil, fmt.Errorf("failed to translate subtitles: %w", err)
	}

	// Update translation results
	translatedFile := &subtitle.File{
		Lines:    translations,
		Language: t.config.TargetLanguage,
		Format:   t.file.Format,
	}

	// Save translation results if output path is specified
	if outputPath != "" {
		if err := t.subtitleWriter.Write(t.config.OutputDir, translatedFile); err != nil {
			return nil, fmt.Errorf("failed to save translation results: %w", err)
		}
	}

	// Create results
	result := &TranslationResult{
		OriginalFile:   *t.file,
		TranslatedFile: *translatedFile,
		Metadata: TranslationMetadata{
			SourceLanguage: t.file.Language,
			TargetLanguage: t.config.TargetLanguage,
			// ModelUsed:      "gpt-3.5-turbo", // can be obtained from LLM client
			ContextSummary: GetContextTextFromTVShow(tvShowInfo),
			// TranslationTime: time.Since(startTime),
			CharCount: countCharacters(t.file.Lines),
		},
	}

	return result, nil
}

// translateSubtitleLines translates subtitle lines
func (t *SubTranslator) translateSubtitleLines(
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
		t.config.TargetLanguage.String(),
		t.config.BatchSize)
}

// FileTranslator is the core structure for subtitle translator
type FileTranslator struct {
	nfoReader      NFOReader
	subtitleReader subtitle.Reader
	subtitleWriter subtitle.Writer
	translator     translator.Translator
	config         TranslatorConfig
}

// Translate translates a single subtitle file
func (t *FileTranslator) Translate(
	ctx context.Context,
	tvshowNFOPath string,
) (*TranslationResult, error) {
	// Read subtitle file
	subtitleFile, err := t.subtitleReader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read subtitle file: %w", err)
	}

	t.config.SubtitleFile = subtitleFile
	subTrans, err := NewTranslator(
		t.config,
		t.translator,
	)
	return subTrans.Translate(ctx, tvshowNFOPath)
}

// PrintTranslationReport prints translation report
func (t *FileTranslator) PrintTranslationReport(result *TranslationResult) {
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
func (t *FileTranslator) GetTranslationPreview(result *TranslationResult, lines int) string {
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

// NewTranslator creates a new translator instance
// TODO validation
func NewTranslator(
	config TranslatorConfig,
	cli translator.Translator,
) (Translator, error) {
	if config.SubtitleFile != nil {
		return &SubTranslator{
			nfoReader:      NewNFOReader(),
			subtitleWriter: subtitle.NewWriter(),
			config:         config,
			translator:     cli,
		}, nil
	}
	return &FileTranslator{
		nfoReader:      NewNFOReader(),
		subtitleReader: subtitle.NewReader(config.InputPath),
		subtitleWriter: subtitle.NewWriter(),
		config:         config,
		translator:     cli,
	}, nil
}

// countCharacters calculates total subtitle characters
func countCharacters(lines []subtitle.Line) int {
	total := 0
	for _, line := range lines {
		total += len(line.Text)
	}
	return total
}
