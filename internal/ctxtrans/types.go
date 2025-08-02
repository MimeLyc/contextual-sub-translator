package ctxtrans

import (
	"context"
	"time"
)

// TVShowInfo represents TV show information parsed from tvshow.nfo file
type TVShowInfo struct {
	Title         string   `xml:"title"`         // show title
	OriginalTitle string   `xml:"originaltitle"` // original title
	Plot          string   `xml:"plot"`          // plot summary
	Genre         []string `xml:"genre"`         // genre tags
	Premiered     string   `xml:"premiered"`     // premiere date
	Rating        float32  `xml:"rating"`        // rating
	Studio        string   `xml:"studio"`        // production studio
	Actors        []Actor  `xml:"actor"`         // cast list
	Aired         string   `xml:"aired"`         // air date
	Year          int      `xml:"year"`          // year
	Season        int      `xml:"season"`        // current season
}

// Actor represents actor information
type Actor struct {
	Name  string `xml:"name"`
	Role  string `xml:"role"`
	Order int    `xml:"order"`
}

// SubtitleLine represents a single subtitle line
type SubtitleLine struct {
	Index          int           // subtitle index
	StartTime      time.Duration // start time
	EndTime        time.Duration // end time
	Text           string        // subtitle text
	TranslatedText string        // translated text
}

// SubtitleFile represents subtitle file
type SubtitleFile struct {
	Lines    []SubtitleLine
	Language string
	Format   string // e.g. SRT, ASS, VTT etc
}

// TranslationRequest represents translation request
type TranslationRequest struct {
	TVShowNFOPath      string
	SubtitlePath       string
	TargetLanguage     string
	ContextEnabled     bool
	PreserveFormatting bool
	APIKey             string
	Model              string
}

// TranslationResult represents translation result
type TranslationResult struct {
	OriginalFile   SubtitleFile
	TranslatedFile SubtitleFile
	Metadata       TranslationMetadata
}

// TranslationMetadata contains translation metadata
type TranslationMetadata struct {
	SourceLanguage  string
	TargetLanguage  string
	ModelUsed       string
	ContextSummary  string
	TranslationTime time.Duration
	CharCount       int
}

// LLMClient is the interface for LLM API
type LLMClient interface {
	TranslateWithContext(ctx context.Context, contextInfo TVShowInfo, subtitleLines []SubtitleLine, targetLanguage string) ([]string, error)
}

// NFOReader is the interface for reading NFO files
type NFOReader interface {
	ReadTVShowInfo(path string) (*TVShowInfo, error)
}

// SubtitleReader is the interface for reading subtitle files
type SubtitleReader interface {
	ReadSubtitle(path string) (*SubtitleFile, error)
}

// SubtitleWriter is the interface for writing subtitle files
type SubtitleWriter interface {
	WriteSubtitle(path string, subtitle *SubtitleFile) error
}
