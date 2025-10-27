package translator

import (
	"context"

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
}

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
