package termmap

import "strings"

// Match filters the term map to only terms that appear in the given texts.
// Uses case-sensitive substring matching (correct for proper nouns).
func Match(tm TermMap, texts []string) MatchResult {
	matched := make(TermMap)

	for source, target := range tm {
		for _, text := range texts {
			if strings.Contains(text, source) {
				matched[source] = target
				break
			}
		}
	}

	return MatchResult{Matched: matched}
}
