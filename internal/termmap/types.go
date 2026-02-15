package termmap

// TermMap maps source language terms to target language terms.
type TermMap map[string]string

// MatchResult holds terms that matched against input texts.
type MatchResult struct {
	Matched TermMap
}
