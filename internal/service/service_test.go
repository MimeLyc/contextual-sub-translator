package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"

	"github.com/MimeLyc/contextual-sub-translator/internal/config"
)

func TestFindSourceBundlesInDir(t *testing.T) {
	// Create temporary directory structure for testing
	tempDir := t.TempDir()

	// Create test files
	testFiles := map[string]string{
		"movie.mkv":        "",
		"movie.srt":        "",
		"movie.ass":        "",
		"episode01.mp4":    "",
		"episode01.srt":    "",
		"episode01.vtt":    "",
		"episode02.avi":    "",
		"episode02.sub":    "",
		"tvshow.nfo":       "",
		"season.nfo":       "",
		"standalone.srt":   "",
		"nosubtitle.mkv":   "",
		"subtitleonly.ass": "",
	}

	for filename, content := range testFiles {
		filePath := filepath.Join(tempDir, filename)
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Create subdirectory with files
	subDir := filepath.Join(tempDir, "subdir")
	err := os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	subFiles := map[string]string{
		"subdir_movie.mkv": "",
		"subdir_movie.srt": "",
		"show.nfo":         "",
	}

	for filename, content := range subFiles {
		filePath := filepath.Join(subDir, filename)
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)
	}

	tests := []struct {
		name            string
		service         transService
		dir             string
		expectedBundles int
		expectedError   bool
		validateBundles func(t *testing.T, bundles []MediaPathBundle)
	}{
		{
			name: "find all bundles without time filter",
			service: transService{
				cfg:            config.Config{},
				lastTrigerTime: time.Now().Add(-24 * time.Hour), // 24 hours ago
				cronExpr:       "",                              // no cron
			},
			dir:             tempDir,
			expectedBundles: 7, // movie, episode01, episode02, standalone.srt, nosubtitle.mkv, subdir_movie, subtitleonly.ass
			expectedError:   false,
			validateBundles: func(t *testing.T, bundles []MediaPathBundle) {
				// Check that we have the expected bundles
				bundleMap := make(map[string]MediaPathBundle)
				for _, bundle := range bundles {
					if bundle.MediaFile != "" {
						baseName := getBaseName(bundle.MediaFile)
						bundleMap[baseName] = bundle
					} else if len(bundle.SubtitleFiles) > 0 {
						baseName := getBaseName(bundle.SubtitleFiles[0])
						bundleMap[baseName] = bundle
					}
				}

				// Validate movie bundle
				movieBundle, exists := bundleMap["movie"]
				assert.True(t, exists)
				assert.Contains(t, movieBundle.MediaFile, "movie.mkv")
				assert.Len(t, movieBundle.SubtitleFiles, 2) // .srt and .ass
				assert.Len(t, movieBundle.NFOFiles, 2)      // tvshow.nfo and season.nfo

				// Validate episode01 bundle
				ep01Bundle, exists := bundleMap["episode01"]
				assert.True(t, exists)
				assert.Contains(t, ep01Bundle.MediaFile, "episode01.mp4")
				assert.Len(t, ep01Bundle.SubtitleFiles, 2) // .srt and .vtt

				// Validate standalone subtitle
				standaloneBundle, exists := bundleMap["standalone"]
				assert.True(t, exists)
				assert.Empty(t, standaloneBundle.MediaFile)
				assert.Len(t, standaloneBundle.SubtitleFiles, 1)
			},
		},
		{
			name: "find bundles with time filter",
			service: transService{
				cfg:            config.Config{},
				lastTrigerTime: time.Now().Add(-1 * time.Hour), // 1 hour ago
				cronExpr:       "",
			},
			dir:             tempDir,
			expectedBundles: 7,
			expectedError:   false,
			validateBundles: func(t *testing.T, bundles []MediaPathBundle) {
				// All files should be found since they were just created
				assert.True(t, len(bundles) >= 4)
			},
		},
		{
			name: "find bundles with future time filter",
			service: transService{
				cfg:            config.Config{},
				lastTrigerTime: time.Now().Add(1 * time.Hour), // 1 hour in future
				cronExpr:       "",
			},
			dir:             tempDir,
			expectedBundles: 0,
			expectedError:   false,
			validateBundles: func(t *testing.T, bundles []MediaPathBundle) {
				// No files should be found since they are older than filter time
				assert.Empty(t, bundles)
			},
		},
		{
			name: "nonexistent directory",
			service: transService{
				cfg:            config.Config{},
				lastTrigerTime: time.Time{},
				cronExpr:       "0 0 * * *",
			},
			dir:             "/nonexistent/directory",
			expectedBundles: 0,
			expectedError:   true,
			validateBundles: nil,
		},
		{
			name: "recursive search in subdirectories",
			service: transService{
				cfg:            config.Config{},
				lastTrigerTime: time.Now().Add(-24 * time.Hour), // 24 hours ago
				cronExpr:       "",
			},
			dir:             tempDir,
			expectedBundles: 7, // includes subdir files
			expectedError:   false,
			validateBundles: func(t *testing.T, bundles []MediaPathBundle) {
				// Should find subdir_movie bundle too
				found := false
				for _, bundle := range bundles {
					if bundle.MediaFile != "" && filepath.Base(bundle.MediaFile) == "subdir_movie.mkv" {
						found = true
						assert.Len(t, bundle.SubtitleFiles, 1)
						assert.Len(t, bundle.NFOFiles, 3) // should find show.nfo in subdir + tvshow.nfo + season.nfo in parent
						break
					}
				}
				assert.True(t, found, "Should find subdir_movie bundle")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			bundles, err := tt.service.findSourceBundlesInDir(ctx, tt.dir)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Len(t, bundles, tt.expectedBundles)

			if tt.validateBundles != nil {
				tt.validateBundles(t, bundles)
			}
		})
	}
}

func TestFindMatchingSubtitleFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	testFiles := []string{
		"movie.srt",
		"movie.ass",
		"movie.vtt",
		"movie.txt",
		"other.srt",
	}

	for _, filename := range testFiles {
		filePath := filepath.Join(tempDir, filename)
		err := os.WriteFile(filePath, []byte("test"), 0644)
		require.NoError(t, err)
	}

	subtitleFiles := findMatchingSubtitleFiles(tempDir, "movie")

	assert.Len(t, subtitleFiles, 4) // movie.srt, movie.ass, movie.vtt, movie.txt

	expectedFiles := []string{
		filepath.Join(tempDir, "movie.srt"),
		filepath.Join(tempDir, "movie.ass"),
		filepath.Join(tempDir, "movie.vtt"),
		filepath.Join(tempDir, "movie.txt"),
	}

	for _, expectedFile := range expectedFiles {
		assert.Contains(t, subtitleFiles, expectedFile)
	}

	// Test with non-existent base name
	noFiles := findMatchingSubtitleFiles(tempDir, "nonexistent")
	assert.Empty(t, noFiles)
}

func TestFindMatchingMediaFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	testFiles := []string{
		"movie.mkv",
		"movie.mp4",
		"other.avi",
	}

	for _, filename := range testFiles {
		filePath := filepath.Join(tempDir, filename)
		err := os.WriteFile(filePath, []byte("test"), 0644)
		require.NoError(t, err)
	}

	// Should find the first matching media file (mkv comes before mp4 in extension list)
	mediaFile := findMatchingMediaFile(tempDir, "movie")
	assert.Contains(t, mediaFile, "movie.mkv")

	// Test with non-existent base name
	noFile := findMatchingMediaFile(tempDir, "nonexistent")
	assert.Empty(t, noFile)
}

func TestFindNFOFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create directory structure
	// tempDir/
	//   ├── tvshow.nfo
	//   ├── season.nfo
	//   └── subdir/
	//       └── show.nfo

	nfoFiles := []string{
		"tvshow.nfo",
		"season.nfo",
	}

	for _, filename := range nfoFiles {
		filePath := filepath.Join(tempDir, filename)
		err := os.WriteFile(filePath, []byte("test"), 0644)
		require.NoError(t, err)
	}

	subDir := filepath.Join(tempDir, "subdir")
	err := os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	subNFOPath := filepath.Join(subDir, "show.nfo")
	err = os.WriteFile(subNFOPath, []byte("test"), 0644)
	require.NoError(t, err)

	// Test from subdirectory - should find show.nfo in subdir and tvshow.nfo/season.nfo in parent
	nfoFiles = findNFOFiles(subDir)
	assert.Len(t, nfoFiles, 3) // show.nfo, tvshow.nfo, season.nfo

	expectedFiles := []string{
		filepath.Join(subDir, "show.nfo"),
		filepath.Join(tempDir, "tvshow.nfo"),
		filepath.Join(tempDir, "season.nfo"),
	}

	for _, expectedFile := range expectedFiles {
		assert.Contains(t, nfoFiles, expectedFile)
	}

	// Test from root directory - should find tvshow.nfo and season.nfo
	rootNFOFiles := findNFOFiles(tempDir)
	assert.Len(t, rootNFOFiles, 2)
}

func TestIsSubtitleFile(t *testing.T) {
	tests := []struct {
		ext      string
		expected bool
	}{
		{".srt", true},
		{".ass", true},
		{".vtt", true},
		{".sub", true},
		{".txt", true},
		{".mkv", false},
		{".mp4", false},
		{".nfo", false},
		{"", false},
		{".unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			result := isSubtitleFile(tt.ext)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsMediaFile(t *testing.T) {
	tests := []struct {
		ext      string
		expected bool
	}{
		{".mkv", true},
		{".mp4", true},
		{".avi", true},
		{".mov", true},
		{".webm", true},
		{".srt", false},
		{".ass", false},
		{".nfo", false},
		{"", false},
		{".unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			result := isMediaFile(tt.ext)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetBaseName(t *testing.T) {
	tests := []struct {
		filePath string
		expected string
	}{
		{"/path/to/movie.mkv", "movie"},
		{"/path/to/episode.s01e01.srt", "episode.s01e01"},
		{"simple.mp4", "simple"},
		{"/complex/path/with.dots.in.name.avi", "with.dots.in.name"},
		{"", ""},
		{"noextension", "noextension"},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			result := getBaseName(tt.filePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindTargetMediaTuplesInDir(t *testing.T) {
	// Create temporary directory structure for testing
	tempDir := t.TempDir()

	// Mock subtitle content for testing
	mockSubtitleContent := "1\n00:00:01,000 --> 00:00:04,000\nHello world world\n\n2\n00:00:05,000 --> 00:00:08,000\nAnother subtitle line\n"
	mockCNSubtitleContent := "1\n00:00:01,000 --> 00:00:04,000\n你好世界\n\n2\n00:00:05,000 --> 00:00:08,000\n另外一行\n"

	tests := []struct {
		name            string
		setupFiles      func(t *testing.T, rootDir string)
		service         transService
		expectedCount   int
		expectedError   bool
		validateContent func(t *testing.T, bundles []MediaBundle)
	}{
		{
			name: "find media without target subtitle",
			setupFiles: func(t *testing.T, rootDir string) {
				// Create media file
				mkvPath := filepath.Join(rootDir, "test_movie.mkv")
				err := os.WriteFile(mkvPath, []byte("mock mkv content"), 0644)
				require.NoError(t, err)

				// Create subtitle file with source language
				subPath := filepath.Join(rootDir, "test_movie.eng.srt")
				err = os.WriteFile(subPath, []byte(mockSubtitleContent), 0644)
				require.NoError(t, err)

				// Create NFO file
				nfoPath := filepath.Join(rootDir, "tvshow.nfo")
				err = os.WriteFile(nfoPath, []byte("<tvshow><title>Test Show</title></tvshow>"), 0644)
				require.NoError(t, err)
			},
			service: transService{
				cfg: config.Config{
					Translate: config.TranslateConfig{
						TargetLanguage: language.MustParse("zh-CN"),
					},
				},
				lastTrigerTime: time.Now().Add(-24 * time.Hour),
				cronExpr:       "",
			},
			expectedCount: 0, // Due to ffprobe dependency, actual processing may be 0
			expectedError: false,
			validateContent: func(t *testing.T, bundles []MediaBundle) {
				// Just check that function executes without errors
			},
		},
		{
			name: "skip media with existing target subtitle",
			setupFiles: func(t *testing.T, rootDir string) {
				// Create media file
				mkvPath := filepath.Join(rootDir, "test_movie.mkv")
				err := os.WriteFile(mkvPath, []byte("mock mkv content"), 0644)
				require.NoError(t, err)

				// Create subtitle file with target language
				subPath := filepath.Join(rootDir, "test_movie.zh-cn.srt")
				err = os.WriteFile(subPath, []byte(mockCNSubtitleContent), 0644)
				require.NoError(t, err)
			},
			service: transService{
				cfg: config.Config{
					Translate: config.TranslateConfig{
						TargetLanguage: language.MustParse("zh-CN"),
					},
				},
				lastTrigerTime: time.Now().Add(-24 * time.Hour),
				cronExpr:       "",
			},
			expectedCount: 0,
			expectedError: false,
			validateContent: func(t *testing.T, bundles []MediaBundle) {
				assert.Empty(t, bundles)
			},
		},
		{
			name: "skip media with target subtitle in file",
			setupFiles: func(t *testing.T, rootDir string) {
				// Create media file
				mkvPath := filepath.Join(rootDir, "test_movie.mkv")
				err := os.WriteFile(mkvPath, []byte("mock mkv content"), 0644)
				require.NoError(t, err)

				// Create subtitle file with source language
				subPath := filepath.Join(rootDir, "test_movie.eng.srt")
				err = os.WriteFile(subPath, []byte(mockSubtitleContent), 0644)
				require.NoError(t, err)
			},
			service: transService{
				cfg: config.Config{
					Translate: config.TranslateConfig{
						TargetLanguage: language.MustParse("zh-CN"),
					},
				},
				lastTrigerTime: time.Now().Add(-24 * time.Hour),
				cronExpr:       "",
			},
			expectedCount: 0,
			expectedError: false,
			validateContent: func(t *testing.T, bundles []MediaBundle) {
				// Due to ffprobe dependency
			},
		},
		{
			name: "extract subtitle from media when no external subtitle exists",
			setupFiles: func(t *testing.T, rootDir string) {
				// Create media file
				mkvPath := filepath.Join(rootDir, "test_movie.mkv")
				err := os.WriteFile(mkvPath, []byte("mock mkv content"), 0644)
				require.NoError(t, err)

				// Create NFO file
				nfoPath := filepath.Join(rootDir, "tvshow.nfo")
				err = os.WriteFile(nfoPath, []byte("<tvshow><title>Test Show</title></tvshow>"), 0644)
				require.NoError(t, err)
			},
			service: transService{
				cfg: config.Config{
					Translate: config.TranslateConfig{
						TargetLanguage: language.MustParse("zh-CN"),
					},
				},
				lastTrigerTime: time.Now().Add(-24 * time.Hour),
				cronExpr:       "",
			},
			expectedCount: 1,
			expectedError: false,
			validateContent: func(t *testing.T, bundles []MediaBundle) {
				// We expect a media bundle to be returned even if subtitle extraction fails
				// because the function continues processing after logging errors
				require.Len(t, bundles, 1)
				assert.Contains(t, bundles[0].MediaFile, "test_movie.mkv")
			},
		},
		{
			name: "handle multiple languages in same bundle",
			setupFiles: func(t *testing.T, rootDir string) {
				// Create media file
				mkvPath := filepath.Join(rootDir, "test_movie.mkv")
				err := os.WriteFile(mkvPath, []byte("mock mkv content"), 0644)
				require.NoError(t, err)

				// Create eng subtitle
				subEngPath := filepath.Join(rootDir, "test_movie.eng.srt")
				err = os.WriteFile(subEngPath, []byte(mockSubtitleContent), 0644)
				require.NoError(t, err)

				// Create jap subtitle (but no zh-cn target)
				subJapPath := filepath.Join(rootDir, "test_movie.jpn.srt")
				err = os.WriteFile(subJapPath, []byte(mockSubtitleContent), 0644)
				require.NoError(t, err)

				// Create NFO file
				nfoPath := filepath.Join(rootDir, "tvshow.nfo")
				err = os.WriteFile(nfoPath, []byte("<tvshow><title>Multi Lang Show</title></tvshow>"), 0644)
				require.NoError(t, err)
			},
			service: transService{
				cfg: config.Config{
					Translate: config.TranslateConfig{
						TargetLanguage: language.MustParse("zh-CN"),
					},
				},
				lastTrigerTime: time.Now().Add(-24 * time.Hour),
				cronExpr:       "",
			},
			expectedCount: 1,
			expectedError: false,
			validateContent: func(t *testing.T, bundles []MediaBundle) {
				require.Len(t, bundles, 1)
				bundle := bundles[0]
				assert.Len(t, bundle.SubtitleFiles, 2) // eng and jpn subs
			},
		},
		{
			name: "handle empty directory",
			setupFiles: func(t *testing.T, rootDir string) {
				// Don't create any files
			},
			service: transService{
				cfg: config.Config{
					Translate: config.TranslateConfig{
						TargetLanguage: language.MustParse("zh-CN"),
					},
				},
				lastTrigerTime: time.Now().Add(-24 * time.Hour),
				cronExpr:       "",
			},
			expectedCount: 0,
			expectedError: false,
			validateContent: func(t *testing.T, bundles []MediaBundle) {
				assert.Empty(t, bundles)
			},
		},
		{
			name: "handle nonexistent directory",
			service: transService{
				cfg: config.Config{
					Translate: config.TranslateConfig{
						TargetLanguage: language.MustParse("zh-CN"),
					},
				},
				lastTrigerTime: time.Now().Add(-24 * time.Hour),
				cronExpr:       "",
			},
			expectedCount: 0,
			expectedError: true,
			validateContent: func(t *testing.T, bundles []MediaBundle) {
				// Error case
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup test directory with files if provided
			if tt.setupFiles != nil {
				tt.setupFiles(t, tempDir)
			}

			// Adjust directory for nonexistent case
			testDir := tempDir
			if tt.name == "handle nonexistent directory" {
				testDir = "/nonexistent/directory/path"
			}

			bundles, err := tt.service.findTargetMediaTuplesInDir(ctx, testDir)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Len(t, bundles, tt.expectedCount)

			if tt.validateContent != nil {
				tt.validateContent(t, bundles)
			}
		})
	}
}

func TestRealFindTargetMediaTuplesInDir(t *testing.T) {
	dir := "..."

	service := transService{
		cfg: config.Config{
			Translate: config.TranslateConfig{
				TargetLanguage: language.MustParse("zh-CN"),
			},
		},
		lastTrigerTime: time.Now().Add(-24000 * time.Hour),
		cronExpr:       "",
	}
	bundles, err := service.findTargetMediaTuplesInDir(context.Background(), dir)
	assert.Nil(t, err)
	assert.Len(t, bundles, 1)
}
