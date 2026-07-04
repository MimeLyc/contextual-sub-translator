package subtitle

import (
	"path/filepath"
	"strings"

	"golang.org/x/text/language"
)

// DefaultTranslatedOutputName builds a media-friendly sidecar subtitle filename.
// It prefers the media basename when available; otherwise it falls back to a
// cleaned subtitle basename with legacy tool suffixes and trailing language
// tokens removed.
func DefaultTranslatedOutputName(mediaPath, inputPath, targetLanguage string) string {
	ext := filepath.Ext(filepath.Base(inputPath))
	if ext == "" {
		ext = ".srt"
	}

	stem := translatedOutputStem(mediaPath, inputPath)
	if stem == "" {
		stem = "subtitle"
	}

	targetLanguage = strings.TrimSpace(targetLanguage)
	if targetLanguage == "" {
		targetLanguage = "zh"
	}

	return stem + "." + targetLanguage + ext
}

func translatedOutputStem(mediaPath, inputPath string) string {
	if mediaStem := trimmedBaseStem(mediaPath); mediaStem != "" {
		return mediaStem
	}
	if inputPath == "" {
		return ""
	}

	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if idx := strings.Index(stem, "_ctxtrans"); idx >= 0 {
		stem = stem[:idx]
	}
	stem = strings.TrimRight(stem, "._- ")
	if stem == "" {
		return ""
	}
	if trimmed, ok := trimTrailingLanguageToken(stem); ok {
		stem = trimmed
	}
	return strings.TrimRight(stem, "._- ")
}

func trimmedBaseStem(path string) string {
	base := filepath.Base(strings.TrimSpace(path))
	if base == "" || base == "." {
		return ""
	}
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func trimTrailingLanguageToken(stem string) (string, bool) {
	idx := strings.LastIndexAny(stem, "._- ")
	if idx <= 0 || idx >= len(stem)-1 {
		return stem, false
	}

	token := strings.ToLower(strings.ReplaceAll(stem[idx+1:], "_", "-"))
	if !isOutputLanguageToken(token) {
		return stem, false
	}

	trimmed := strings.TrimRight(stem[:idx], "._- ")
	if trimmed == "" {
		return stem, false
	}
	return trimmed, true
}

func isOutputLanguageToken(token string) bool {
	switch token {
	case "chs", "cht":
		return true
	}

	tag, err := language.Parse(token)
	if err != nil || tag == language.Und {
		return false
	}
	_, conf := tag.Base()
	return conf != language.No
}
