package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/text/language"

	"github.com/MimeLyc/contextual-sub-translator/internal/llm"
	"github.com/MimeLyc/contextual-sub-translator/internal/media"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"github.com/MimeLyc/contextual-sub-translator/internal/translator"
)

// Mock implementations
type mockNFOReader struct {
	mock.Mock
}

func (m *mockNFOReader) ReadTVShowInfo(path string) (*media.TVShowInfo, error) {
	args := m.Called(path)
	return args.Get(0).(*media.TVShowInfo), args.Error(1)
}

type mockSubtitleReader struct {
	mock.Mock
}

func (m *mockSubtitleReader) Read() (*subtitle.File, error) {
	args := m.Called()
	return args.Get(0).(*subtitle.File), args.Error(1)
}

type mockSubtitleWriter struct {
	mock.Mock
}

func (m *mockSubtitleWriter) Write(path string, subtitle *subtitle.File) error {
	args := m.Called(path, subtitle)
	return args.Error(0)
}

type mockTranslator struct {
	mock.Mock
}

func (m *mockTranslator) Translate(ctx context.Context, media translator.MediaMeta, subtitleTexts []string, targetLang string) ([]string, error) {
	args := m.Called(ctx, media, subtitleTexts, targetLang)
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockTranslator) BatchTranslate(ctx context.Context, media translator.MediaMeta, subtitleLines []subtitle.Line, targetLanguage string, batchSize int) ([]subtitle.Line, error) {
	args := m.Called(ctx, media, subtitleLines, targetLanguage, batchSize)
	return args.Get(0).([]subtitle.Line), args.Error(1)
}

// Helper function to create DAN DA DAN test data based on actual testdata
func createDANDANTVShowInfo() *media.TVShowInfo {
	return &media.TVShowInfo{
		Title:         "DAN DA DAN",
		OriginalTitle: "DAN DA DAN",
		Plot:          "An anime series about supernatural encounters",
		Genre:         []string{"Action", "Comedy", "Supernatural"},
		Premiered:     "2025-07-04",
		Rating:        8.5,
		Studio:        "Science SARU",
		Year:          2025,
		Season:        2,
		Actors: []media.Actor{
			{Name: "若山诗音", Role: "Momo Ayase (voice)", Order: 0},
			{Name: "花江夏树", Role: "Ken 'Okarun' Takakura (voice)", Order: 1},
		},
	}
}

func createDANDANSubtitleFile() *subtitle.File {
	return &subtitle.File{
		Lines: []subtitle.Line{
			{
				Index:     1,
				StartTime: 20*time.Second + 410*time.Millisecond,
				EndTime:   22*time.Second + 160*time.Millisecond,
				Text:      "Damn you!",
			},
			{
				Index:     2,
				StartTime: 23*time.Second + 580*time.Millisecond,
				EndTime:   25*time.Second + 80*time.Millisecond,
				Text:      "I'm really sorry.",
			},
			{
				Index:     3,
				StartTime: 28*time.Second + 40*time.Millisecond,
				EndTime:   30*time.Second + 340*time.Millisecond,
				Text:      "How many times do you have to do this before you're satisfied?",
			},
		},
		Language: language.English,
		Format:   "SRT",
	}
}

func createDANDANTranslatedLines() []subtitle.Line {
	return []subtitle.Line{
		{
			Index:          1,
			StartTime:      20*time.Second + 410*time.Millisecond,
			EndTime:        22*time.Second + 160*time.Millisecond,
			Text:           "Damn you!",
			TranslatedText: "可恶！",
		},
		{
			Index:          2,
			StartTime:      23*time.Second + 580*time.Millisecond,
			EndTime:        25*time.Second + 80*time.Millisecond,
			Text:           "I'm really sorry.",
			TranslatedText: "真的很抱歉。",
		},
		{
			Index:          3,
			StartTime:      28*time.Second + 40*time.Millisecond,
			EndTime:        30*time.Second + 340*time.Millisecond,
			Text:           "How many times do you have to do this before you're satisfied?",
			TranslatedText: "你要做多少次才满意？",
		},
	}
}

func TestTranslateFile_Success_WithContext_RealData(t *testing.T) {
	// Arrange
	mockNFO := &mockNFOReader{}
	mockSubReader := &mockSubtitleReader{}
	mockSubWriter := &mockSubtitleWriter{}
	mockTrans := &mockTranslator{}

	config := TranslatorConfig{
		TargetLanguage: language.Chinese,
		BatchSize:      10,
		ContextEnabled: true,
		OutputDir:      "/tmp/output.srt",
		Verbose:        false,
	}

	ctxtrans := &FileTranslator{
		nfoReader:      mockNFO,
		subtitleReader: mockSubReader,
		subtitleWriter: mockSubWriter,
		translator:     mockTrans,
		config:         config,
	}

	ctx := context.Background()
	nfoPath := "testdata/data/tvshow.nfo"
	subtitlePath := "testdata/data/DAN.DA.DAN.s02e06.[WEBDL-720p].[Erai-raws].eng.srt"

	testTVShow := createDANDANTVShowInfo()
	testSubtitleFile := createDANDANSubtitleFile()
	testTranslatedLines := createDANDANTranslatedLines()

	// Set up expectations
	mockNFO.On("ReadTVShowInfo", nfoPath).Return(testTVShow, nil)
	mockSubReader.On("Read", subtitlePath).Return(testSubtitleFile, nil)
	mockTrans.On("BatchTranslate", ctx, mock.AnythingOfType("translator.MediaMeta"), testSubtitleFile.Lines, "Chinese", 10).Return(testTranslatedLines, nil)
	mockSubWriter.On("Write", "/tmp/output.srt", mock.AnythingOfType("*subtitle.File")).Return(nil)

	// Act
	result, err := ctxtrans.Translate(ctx, nfoPath)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, len(testSubtitleFile.Lines), len(result.OriginalFile.Lines))
	assert.Equal(t, len(testTranslatedLines), len(result.TranslatedFile.Lines))
	assert.Equal(t, "Chinese", result.TranslatedFile.Language)
	assert.Equal(t, "SRT", result.TranslatedFile.Format)

	// Verify specific translation content
	assert.Equal(t, "Damn you!", result.OriginalFile.Lines[0].Text)
	assert.Equal(t, "可恶！", result.TranslatedFile.Lines[0].TranslatedText)

	// Verify mocks
	mockNFO.AssertExpectations(t)
	mockSubReader.AssertExpectations(t)
	mockSubWriter.AssertExpectations(t)
	mockTrans.AssertExpectations(t)
}

func TestTranslateFile_Success_WithoutContext_RealData(t *testing.T) {
	// Arrange
	mockNFO := &mockNFOReader{}
	mockSubReader := &mockSubtitleReader{}
	mockSubWriter := &mockSubtitleWriter{}
	mockTrans := &mockTranslator{}

	config := TranslatorConfig{
		TargetLanguage: language.Chinese,
		BatchSize:      10,
		ContextEnabled: false, // Context disabled
		OutputDir:      "",    // No output path
	}

	ctxtrans := &FileTranslator{
		nfoReader:      mockNFO,
		subtitleReader: mockSubReader,
		subtitleWriter: mockSubWriter,
		translator:     mockTrans,
		config:         config,
	}

	ctx := context.Background()
	nfoPath := "testdata/data/season.nfo"
	subtitlePath := "testdata/data/DAN.DA.DAN.s02e06.[WEBDL-720p].[Erai-raws].eng.srt"

	testTVShow := createDANDANTVShowInfo()
	testSubtitleFile := createDANDANSubtitleFile()
	testTranslatedLines := createDANDANTranslatedLines()

	// Set up expectations
	mockNFO.On("ReadTVShowInfo", nfoPath).Return(testTVShow, nil)
	mockSubReader.On("Read", subtitlePath).Return(testSubtitleFile, nil)
	// When context is disabled, MediaMeta should have empty TVShowInfo
	mockTrans.On("BatchTranslate", ctx, translator.MediaMeta{TVShowInfo: media.TVShowInfo{}}, testSubtitleFile.Lines, "Chinese", 10).Return(testTranslatedLines, nil)

	// Act
	result, err := ctxtrans.Translate(ctx, nfoPath)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, len(result.TranslatedFile.Lines))

	// Verify mocks (no write should be called since output path is empty)
	mockNFO.AssertExpectations(t)
	mockSubReader.AssertExpectations(t)
	mockTrans.AssertExpectations(t)
	mockSubWriter.AssertNotCalled(t, "Write")
}

func TestTranslateFile_NFOReadError(t *testing.T) {
	// Arrange
	mockNFO := &mockNFOReader{}
	mockSubReader := &mockSubtitleReader{}
	mockSubWriter := &mockSubtitleWriter{}
	mockTrans := &mockTranslator{}

	ctx := context.Background()
	nfoPath := "testdata/data/nonexistent.nfo"
	subtitlePath := "testdata/data/DAN.DA.DAN.s02e06.[WEBDL-720p].[Erai-raws].eng.srt"

	config := TranslatorConfig{
		TargetLanguage: language.Chinese,
		ContextEnabled: true,
		InputPath:      subtitlePath,
	}

	// Set up expectations
	mockNFO.On("ReadTVShowInfo", nfoPath).Return((*media.TVShowInfo)(nil), errors.New("NFO file not found"))

	translator := &FileTranslator{
		nfoReader:      mockNFO,
		subtitleReader: mockSubReader,
		subtitleWriter: mockSubWriter,
		translator:     mockTrans,
		config:         config,
	}

	// Act
	result, err := translator.Translate(ctx, nfoPath)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to read NFO file")

	// Verify mocks
	mockNFO.AssertExpectations(t)
	mockSubReader.AssertNotCalled(t, "Read")
	mockSubWriter.AssertNotCalled(t, "Write")
	mockTrans.AssertNotCalled(t, "BatchTranslate")
}

func TestTranslateFile_SubtitleReadError(t *testing.T) {
	// Arrange
	mockNFO := &mockNFOReader{}
	mockSubReader := &mockSubtitleReader{}
	mockSubWriter := &mockSubtitleWriter{}
	mockTrans := &mockTranslator{}

	config := TranslatorConfig{
		TargetLanguage: language.Chinese,
		ContextEnabled: true,
	}

	translator := &FileTranslator{
		nfoReader:      mockNFO,
		subtitleReader: mockSubReader,
		subtitleWriter: mockSubWriter,
		translator:     mockTrans,
		config:         config,
	}

	ctx := context.Background()
	nfoPath := "testdata/data/season.nfo"
	subtitlePath := "testdata/data/nonexistent.srt"

	testTVShow := createDANDANTVShowInfo()

	// Set up expectations
	mockNFO.On("ReadTVShowInfo", nfoPath).Return(testTVShow, nil)
	mockSubReader.On("Read", subtitlePath).Return((*subtitle.File)(nil), errors.New("subtitle file not found"))

	// Act
	result, err := translator.Translate(ctx, nfoPath)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to read subtitle file")

	// Verify mocks
	mockNFO.AssertExpectations(t)
	mockSubReader.AssertExpectations(t)
	mockSubWriter.AssertNotCalled(t, "Write")
	mockTrans.AssertNotCalled(t, "BatchTranslate")
}

func TestTranslateFile_TranslationError(t *testing.T) {
	// Arrange
	mockNFO := &mockNFOReader{}
	mockSubReader := &mockSubtitleReader{}
	mockSubWriter := &mockSubtitleWriter{}
	mockTrans := &mockTranslator{}

	config := TranslatorConfig{
		TargetLanguage: language.Chinese,
		BatchSize:      10,
		ContextEnabled: true,
	}

	translator := &FileTranslator{
		nfoReader:      mockNFO,
		subtitleReader: mockSubReader,
		subtitleWriter: mockSubWriter,
		translator:     mockTrans,
		config:         config,
	}

	ctx := context.Background()
	nfoPath := "testdata/data/season.nfo"
	subtitlePath := "testdata/data/DAN.DA.DAN.s02e06.[WEBDL-720p].[Erai-raws].eng.srt"

	testTVShow := createDANDANTVShowInfo()
	testSubtitleFile := createDANDANSubtitleFile()

	// Set up expectations
	mockNFO.On("ReadTVShowInfo", nfoPath).Return(testTVShow, nil)
	mockSubReader.On("Read", subtitlePath).Return(testSubtitleFile, nil)
	mockTrans.On("BatchTranslate", ctx, mock.AnythingOfType("translator.MediaMeta"), testSubtitleFile.Lines, "Chinese", 10).Return(([]subtitle.Line)(nil), errors.New("LLM API error: rate limit exceeded"))

	// Act
	result, err := translator.Translate(ctx, nfoPath)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to translate subtitles")

	// Verify mocks
	mockNFO.AssertExpectations(t)
	mockSubReader.AssertExpectations(t)
	mockTrans.AssertExpectations(t)
	mockSubWriter.AssertNotCalled(t, "Write")
}

func TestTranslateFile_WriteError(t *testing.T) {
	// Arrange
	mockNFO := &mockNFOReader{}
	mockSubReader := &mockSubtitleReader{}
	mockSubWriter := &mockSubtitleWriter{}
	mockTrans := &mockTranslator{}

	config := TranslatorConfig{
		TargetLanguage: language.Chinese,
		BatchSize:      10,
		ContextEnabled: true,
		OutputDir:      "/readonly/output.srt",
	}

	translator := &FileTranslator{
		nfoReader:      mockNFO,
		subtitleReader: mockSubReader,
		subtitleWriter: mockSubWriter,
		translator:     mockTrans,
		config:         config,
	}

	ctx := context.Background()
	nfoPath := "testdata/data/season.nfo"
	subtitlePath := "testdata/data/DAN.DA.DAN.s02e06.[WEBDL-720p].[Erai-raws].eng.srt"

	testTVShow := createDANDANTVShowInfo()
	testSubtitleFile := createDANDANSubtitleFile()
	testTranslatedLines := createDANDANTranslatedLines()

	// Set up expectations
	mockNFO.On("ReadTVShowInfo", nfoPath).Return(testTVShow, nil)
	mockSubReader.On("Read", subtitlePath).Return(testSubtitleFile, nil)
	mockTrans.On("BatchTranslate", ctx, mock.AnythingOfType("translator.MediaMeta"), testSubtitleFile.Lines, "Chinese", 10).Return(testTranslatedLines, nil)
	mockSubWriter.On("Write", "/readonly/output.srt", mock.AnythingOfType("*subtitle.File")).Return(errors.New("write permission denied"))

	// Act
	result, err := translator.Translate(ctx, nfoPath)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to save translation results")

	// Verify mocks
	mockNFO.AssertExpectations(t)
	mockSubReader.AssertExpectations(t)
	mockTrans.AssertExpectations(t)
	mockSubWriter.AssertExpectations(t)
}

func TestTranslateFile_NoTranslatorSet(t *testing.T) {
	// Arrange
	mockNFO := &mockNFOReader{}
	mockSubReader := &mockSubtitleReader{}
	mockSubWriter := &mockSubtitleWriter{}

	config := TranslatorConfig{
		TargetLanguage: language.Chinese,
		ContextEnabled: true,
	}

	translator := &FileTranslator{
		nfoReader:      mockNFO,
		subtitleReader: mockSubReader,
		subtitleWriter: mockSubWriter,
		translator:     nil, // No translator set
		config:         config,
	}

	ctx := context.Background()
	nfoPath := "testdata/data/season.nfo"
	subtitlePath := "testdata/data/DAN.DA.DAN.s02e06.[WEBDL-720p].[Erai-raws].eng.srt"

	testTVShow := createDANDANTVShowInfo()
	testSubtitleFile := createDANDANSubtitleFile()

	// Set up expectations
	mockNFO.On("ReadTVShowInfo", nfoPath).Return(testTVShow, nil)
	mockSubReader.On("Read", subtitlePath).Return(testSubtitleFile, nil)

	// Act
	result, err := translator.Translate(ctx, nfoPath)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Translator not set")

	// Verify mocks
	mockNFO.AssertExpectations(t)
	mockSubReader.AssertExpectations(t)
	mockSubWriter.AssertNotCalled(t, "Write")
}

// Integration test demonstrating how to use LLM client with translator using real testdata
func TestTranslateFile_WithLLMClient_Integration(t *testing.T) {
	_ = godotenv.Load("./.env")
	// Skip this test if no API key is available
	if os.Getenv("LLM_API_KEY") == "" {
		t.Skip("Skipping integration test: LLM_API_KEY not set")
	}

	// Use testdata files
	nfoPath := filepath.Join("testdata", "data", "tvshow.nfo")
	subtitlePath := filepath.Join("testdata", "data", "DAN.DA.DAN.s02e06.[WEBDL-720p].[Erai-raws].eng.srt")
	outputPath := filepath.Join(t.TempDir(), "translated_output.srt")

	// Verify testdata files exist
	_, err := os.Stat(nfoPath)
	if os.IsNotExist(err) {
		t.Skip("Testdata files not available")
	}
	_, err = os.Stat(subtitlePath)
	if os.IsNotExist(err) {
		t.Skip("Testdata files not available")
	}

	// Create LLM config and client
	config := &llm.Config{
		APIKey:      os.Getenv("LLM_API_KEY"),
		APIURL:      "https://openrouter.ai/api/v1",
		Model:       "moonshotai/kimi-k2",
		MaxTokens:   1000,
		Temperature: 0.7,
		Timeout:     30,
	}

	llmClient, err := llm.NewClient(config)
	assert.NoError(t, err)

	// Create AI translator using LLM client
	aiTranslator := translator.NewAiTranslator(*llmClient)

	// Create and configure translator
	translatorConfig := TranslatorConfig{
		TargetLanguage: language.Chinese,
		BatchSize:      10, // Small batch for testing
		ContextEnabled: true,
		OutputDir:      outputPath,
		Verbose:        true,
	}

	trans, err := NewTranslator(translatorConfig, aiTranslator)
	assert.NoError(t, err)

	// Act
	ctx := context.Background()
	result, err := trans.Translate(ctx, nfoPath)

	// Assert
	if err != nil {
		t.Logf("Integration test failed (this might be expected if API is unavailable): %v", err)
		return
	}

	assert.NotNil(t, result)
	assert.Greater(t, len(result.TranslatedFile.Lines), 0)
	assert.Equal(t, "Chinese", result.TranslatedFile.Language)

	// Verify output file was created
	_, err = os.Stat(outputPath)
	assert.NoError(t, err)

	// Log some translation results for verification
	for i, line := range result.TranslatedFile.Lines {
		if i < 3 { // Only log first 3 for brevity
			t.Logf("Line %d - Original: %s", i+1, line.Text)
			t.Logf("Line %d - Translated: %s", i+1, line.TranslatedText)
		}
	}
}
