package termmap

import (
	"encoding/json"
	"os"
	"path/filepath"

	"golang.org/x/text/language"
)

// Filename returns the term map filename for the given source and target languages.
// Uses 2-letter language base codes (e.g., "en", "zh").
func Filename(sourceLang, targetLang string) string {
	src := normalizeLanguageCode(sourceLang)
	tgt := normalizeLanguageCode(targetLang)
	return "term_map." + src + "-" + tgt + ".json"
}

// FilePath returns the full path to the term map file in the given directory.
func FilePath(dir, sourceLang, targetLang string) string {
	return filepath.Join(dir, Filename(sourceLang, targetLang))
}

// FindInAncestors walks up from startDir looking for a term_map file.
// Returns the first found path or empty string.
func FindInAncestors(startDir, sourceLang, targetLang string) string {
	filename := Filename(sourceLang, targetLang)
	currentDir := startDir

	for {
		candidate := filepath.Join(currentDir, filename)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			break
		}
		currentDir = parentDir
	}

	return ""
}

// Load reads a term map from a JSON file.
func Load(path string) (TermMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var tm TermMap
	if err := json.Unmarshal(data, &tm); err != nil {
		return nil, err
	}

	return tm, nil
}

// Save writes a term map to a JSON file with indentation.
func Save(path string, tm TermMap) error {
	data, err := json.MarshalIndent(tm, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// normalizeLanguageCode parses a language string and returns its 2-letter base code.
func normalizeLanguageCode(lang string) string {
	tag, err := language.Parse(lang)
	if err != nil {
		return lang
	}
	base, _ := tag.Base()
	return base.String()
}
