package termmap

import (
	"strings"
	"unicode"
)

// Match filters the term map to only terms that appear in the given texts.
// Uses case-sensitive word-boundary matching to avoid false positives
// (e.g. "elf" should not match "herself").
func Match(tm TermMap, texts []string) MatchResult {
	matched := make(TermMap)

	for source, target := range tm {
		for _, text := range texts {
			if ContainsWord(text, source) {
				matched[source] = target
				break
			}
		}
	}

	return MatchResult{Matched: matched}
}

// ContainsWord checks if term appears in text with word boundaries on both sides.
// A word boundary is the start/end of string or a non-letter/non-digit character.
// This is case-sensitive.
func ContainsWord(text, term string) bool {
	if term == "" {
		return false
	}
	for i := 0; i <= len(text)-len(term); {
		idx := strings.Index(text[i:], term)
		if idx < 0 {
			return false
		}
		start := i + idx
		end := start + len(term)

		leftOK := start == 0 || !isWordChar(rune(text[start-1]))
		rightOK := end == len(text) || !isWordChar(rune(text[end]))

		if leftOK && rightOK {
			return true
		}
		i = start + 1
	}
	return false
}

// ContainsWordFold is like ContainsWord but case-insensitive.
func ContainsWordFold(text, term string) bool {
	return ContainsWord(strings.ToLower(text), strings.ToLower(term))
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
