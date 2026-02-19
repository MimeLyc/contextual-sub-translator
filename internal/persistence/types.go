package persistence

import (
	"time"

	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
)

type BatchCheckpoint struct {
	JobID           string
	BatchStart      int
	BatchEnd        int
	TranslatedLines []string
	UpdatedAt       time.Time
}

type SubtitleCacheEntry struct {
	CacheKey  string
	MediaPath string
	JobID     string
	File      subtitle.File
	IsTemp    bool
	UpdatedAt time.Time
}

type MediaMetaCache struct {
	MediaPath         string
	TargetLanguage    string
	ExternalLanguages []string
	EmbeddedLanguages []string
	HasTargetExternal bool
	HasTargetEmbedded bool
	ExpiresAt         time.Time
	UpdatedAt         time.Time
}
