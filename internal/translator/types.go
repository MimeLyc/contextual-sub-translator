package translator

import (
	"context"

	"github.com/MimeLyc/contextual-sub-translator/internal/agent"
	"github.com/MimeLyc/contextual-sub-translator/internal/media"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
)

const (
	subtitleLineBreaker      string = "%%line_breaker%%"
	inlineBreakerPlaceholder string = "%%inline_breaker%%"
)

type MediaMeta struct {
	media.TVShowInfo
	media.Actor
	TermMap map[string]string
}

// Translator defines the interface for translating subtitles.
type Translator interface {
	Translate(
		ctx context.Context,
		media MediaMeta,
		subtitleTexts []string,
		sourceLang string,
		targetLang string,
	) ([]string, error)

	BatchTranslate(
		ctx context.Context,
		media MediaMeta,
		subtitleLines []subtitle.Line,
		sourceLanguage string,
		targetLanguage string,
		batchSize int) ([]subtitle.Line, error)
}

// TermDiscoverer defines the interface for accumulating tool calls during translation.
// Implementations can collect web_search results for post-translation term extraction.
type TermDiscoverer interface {
	CollectedToolCalls() []agent.ToolCallRecord
	ResetCollectedToolCalls()
}
