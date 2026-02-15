package termmap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilename(t *testing.T) {
	tests := []struct {
		name       string
		sourceLang string
		targetLang string
		expected   string
	}{
		{"simple codes", "en", "zh", "term_map.en-zh.json"},
		{"BCP47 tags", "zh-CN", "en-US", "term_map.zh-en.json"},
		{"mixed", "en", "zh-CN", "term_map.en-zh.json"},
		{"Japanese", "en", "ja", "term_map.en-ja.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Filename(tt.sourceLang, tt.targetLang)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilePath(t *testing.T) {
	result := FilePath("/media/shows/DanDaDan", "en", "zh")
	assert.Equal(t, filepath.Join("/media/shows/DanDaDan", "term_map.en-zh.json"), result)
}

func TestFindInAncestors(t *testing.T) {
	// Create temp directory structure:
	// root/
	//   term_map.en-zh.json
	//   season1/
	//     episode1/
	root := t.TempDir()
	season1 := filepath.Join(root, "season1")
	episode1 := filepath.Join(season1, "episode1")
	require.NoError(t, os.MkdirAll(episode1, 0755))

	// Place term_map at root level
	tmPath := filepath.Join(root, "term_map.en-zh.json")
	require.NoError(t, os.WriteFile(tmPath, []byte(`{"hello":"world"}`), 0644))

	// Search from episode1 should find it at root
	found := FindInAncestors(episode1, "en", "zh")
	assert.Equal(t, tmPath, found)

	// Search from season1 should also find it
	found = FindInAncestors(season1, "en", "zh")
	assert.Equal(t, tmPath, found)

	// Search from root should find it directly
	found = FindInAncestors(root, "en", "zh")
	assert.Equal(t, tmPath, found)

	// Search for non-existent language pair should return empty
	found = FindInAncestors(episode1, "en", "ja")
	assert.Empty(t, found)
}

func TestFindInAncestors_ClosestWins(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	require.NoError(t, os.MkdirAll(child, 0755))

	// Place term_map in both root and child
	rootTm := filepath.Join(root, "term_map.en-zh.json")
	childTm := filepath.Join(child, "term_map.en-zh.json")
	require.NoError(t, os.WriteFile(rootTm, []byte(`{"a":"b"}`), 0644))
	require.NoError(t, os.WriteFile(childTm, []byte(`{"c":"d"}`), 0644))

	// Search from child should find child's version (closest)
	found := FindInAncestors(child, "en", "zh")
	assert.Equal(t, childTm, found)
}

func TestLoadAndSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "term_map.en-zh.json")

	original := TermMap{
		"Momo Ayase": "绫濑桃",
		"Okarun":     "奥卡轮",
		"Turbo Granny": "涡轮婆婆",
	}

	// Save
	err := Save(path, original)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Load
	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, original, loaded)
}

func TestLoad_NonExistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/term_map.json")
	assert.Error(t, err)
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0644))

	_, err := Load(path)
	assert.Error(t, err)
}

func TestNormalizeLanguageCode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"en", "en"},
		{"zh-CN", "zh"},
		{"en-US", "en"},
		{"ja", "ja"},
		{"pt-BR", "pt"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeLanguageCode(tt.input))
		})
	}
}
