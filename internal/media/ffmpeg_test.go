package media

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"github.com/stretchr/testify/assert"
	"golang.org/x/text/language"
)

// TestFFmpeg_ReadSubtitleDescription tests the ReadSubtitleDescription function
func TestFFmpeg_ReadSubtitleDescription(t *testing.T) {
	tests := []struct {
		name        string
		mockOutput  string
		exitCode    int
		expected    []subtitle.Description
		expectError bool
	}{
		{
			name: "Multiple subtitle streams",
			mockOutput: `{
				"streams": [
					{
						"codec_type": "video",
						"codec_name": "h264",
						"tags": {
							"language": "eng"
						}
					},
					{
						"codec_type": "subtitle",
						"codec_name": "srt",
						"tags": {
							"language": "eng",
							"title": "English SDH"
						}
					},
					{
						"codec_type": "subtitle",
						"codec_name": "ass",
						"tags": {
							"language": "jpn",
							"title": "Japanese Signs/Songs"
						}
					}
				]
			}`,
			exitCode: 0,
			expected: []subtitle.Description{
				{Language: "eng", SubLanguage: "English SDH"},
				{Language: "jpn", SubLanguage: "Japanese Signs/Songs"},
			},
			expectError: false,
		},
		{
			name: "No subtitle streams",
			mockOutput: `{
				"streams": [
					{
						"codec_type": "video",
						"codec_name": "h264",
						"tags": {
							"language": "eng"
						}
					},
					{
						"codec_type": "audio",
						"codec_name": "aac",
						"tags": {
							"language": "eng"
						}
					}
				]
			}`,
			exitCode:    0,
			expected:    []subtitle.Description{},
			expectError: false,
		},
		{
			name: "Subtitle without language tag",
			mockOutput: `{
				"streams": [
					{
						"codec_type": "subtitle",
						"codec_name": "srt",
						"tags": {
							"title": "Forced Subtitles"
						}
					}
				]
			}`,
			exitCode: 0,
			expected: []subtitle.Description{
				{Language: "und", SubLanguage: "Forced Subtitles"},
			},
			expectError: false,
		},
		{
			name: "Empty tags subtitle",
			mockOutput: `{
				"streams": [
					{
						"codec_type": "subtitle",
						"codec_name": "srt"
					}
				]
			}`,
			exitCode: 0,
			expected: []subtitle.Description{
				{Language: "und", SubLanguage: ""},
			},
			expectError: false,
		},
		{
			name:        "Invalid JSON",
			mockOutput:  `{"streams": [invalid json`,
			exitCode:    0,
			expected:    nil,
			expectError: true,
		},
		{
			name: "Valid JSON with non-zero exit",
			mockOutput: `{
				"streams": [
					{
						"codec_type": "subtitle",
						"codec_name": "srt",
						"tags": {
							"language": "eng",
							"title": "English"
						}
					}
				]
			}`,
			exitCode: 1,
			expected: []subtitle.Description{
				{Language: "eng", SubLanguage: "English"},
			},
			expectError: false,
		},
		{
			name:        "Non-zero exit without streams should fail",
			mockOutput:  `{}`,
			exitCode:    1,
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for our mock ffprobe
			mockDir, err := os.MkdirTemp("", "ffmpeg-test")
			assert.NoError(t, err)
			defer os.RemoveAll(mockDir)

			// Create mock ffprobe script
			mockProbe := filepath.Join(mockDir, "ffprobe")
			if runtime.GOOS == "windows" {
				mockProbe += ".bat"
				script := "@echo off\necho " + tt.mockOutput + "\nexit /b " + strconv.Itoa(tt.exitCode)
				err = os.WriteFile(mockProbe, []byte(script), 0755)
			} else {
				script := "#!/bin/sh\necho '" + tt.mockOutput + "'\nexit " + strconv.Itoa(tt.exitCode)
				err = os.WriteFile(mockProbe, []byte(script), 0755)
			}
			assert.NoError(t, err)

			// Add mock directory to PATH
			originalPath := os.Getenv("PATH")
			defer os.Setenv("PATH", originalPath)
			os.Setenv("PATH", mockDir+":"+originalPath)

			// Test the function
			ff := NewFfmpeg("dummy.mp4")
			result, err := ff.ReadSubtitleDescription()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, len(tt.expected))
				for i := range tt.expected {
					assert.Equal(t, tt.expected[i].Language, result[i].Language)
					assert.Equal(t, tt.expected[i].SubLanguage, result[i].SubLanguage)
					if tt.expected[i].Language == "und" {
						assert.Equal(t, language.Und, result[i].LangTag)
					} else {
						assert.NotEqual(t, language.Und, result[i].LangTag)
					}
				}
			}
		})
	}
}

// TestFFmpeg_readProbeArgs tests the readProbeArgs function
func TestFFmpeg_readProbeArgs(t *testing.T) {
	ff := ffmpeg{
		ffmpegCmd:  "ffmpeg",
		ffprobeCmd: "ffprobe",
	}
	args := ff.readProbeArgs("/path/to/video.mp4")

	expected := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-select_streams",
		"s",
		"/path/to/video.mp4",
	}

	assert.Equal(t, expected, args)
}

// TestNewFfmpeg tests the NewFfmpeg function
func TestNewFfmpeg(t *testing.T) {
	ff := NewFfmpeg("")
	assert.Equal(t, "ffmpeg", ff.ffmpegCmd)
	assert.Equal(t, "ffprobe", ff.ffprobeCmd)
}

// TestErrorCases tests error handling
func TestErrorCases(t *testing.T) {
	// Test when ffprobe is not in PATH
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Clear PATH to simulate ffprobe not being available
	os.Setenv("PATH", "")

	ff := NewFfmpeg("test.mp4")
	_, err := ff.ReadSubtitleDescription()
	assert.Error(t, err)

	// Should be exec.LookPath error
	assert.Contains(t, err.Error(), "ffprobe")
}

// generateMockFFProbeOutput is a helper to create structured test data
func generateMockFFProbeOutput(streams []struct {
	CodecType string
	Language  string
	Title     string
}) string {
	output := struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			Tags      struct {
				Language string `json:"language"`
				Title    string `json:"title"`
			} `json:"tags"`
		} `json:"streams"`
	}{}

	for _, stream := range streams {
		var streamData struct {
			CodecType string `json:"codec_type"`
			Tags      struct {
				Language string `json:"language"`
				Title    string `json:"title"`
			} `json:"tags"`
		}
		streamData.CodecType = stream.CodecType
		streamData.Tags.Language = stream.Language
		streamData.Tags.Title = stream.Title
		output.Streams = append(output.Streams, streamData)
	}

	jsonData, _ := json.Marshal(output)
	return string(jsonData)
}

// TestRealFFProbe tests with actual ffprobe if available
func TestRealFFProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test that requires actual ffprobe")
	}

	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available, skipping real test")
	}

	// This is a basic test that ffprobe exists and can be called
	// In real usage, you would want to test with actual video files.
	// Use a guaranteed missing file path to keep behavior deterministic.
	ff := NewFfmpeg(filepath.Join(t.TempDir(), "missing-input.mkv"))
	_, err := ff.ReadSubtitleDescription()
	assert.Error(t, err) // Should fail with file not found

	// The test above will fail, but shows interaction with actual ffprobe
	// Any real testing would need actual test media files
}
