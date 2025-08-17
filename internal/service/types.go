package service

import (
	"time"

	"github.com/MimeLyc/contextual-sub-translator/internal/media"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"golang.org/x/text/language"
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
	OriginalFile   subtitle.File
	TranslatedFile subtitle.File
	Metadata       TranslationMetadata
}

// TranslationMetadata contains translation metadata
type TranslationMetadata struct {
	SourceLanguage  language.Tag
	TargetLanguage  language.Tag
	ModelUsed       string
	ContextSummary  string
	TranslationTime time.Duration
	CharCount       int
}

// NFOReader is the interface for reading NFO files
type NFOReader interface {
	ReadTVShowInfo(path string) (*media.TVShowInfo, error)
}

type MediaBundle struct {
	MediaFile     string
	NFOFiles      []media.TVShowInfo
	SubtitleFiles []subtitle.File
}

type MediaPathBundle struct {
	MediaFile     string
	NFOFiles      []string
	SubtitleFiles []string
}

func (b MediaPathBundle) ExistTargetSubtitle(lang string) int {
	for i, v := range b.SubtitleFiles {
		if v == lang {
			return i
		}
	}
	return -1

}

type SourceBundles []MediaPathBundle

var subtitleExts = []string{
	".srt",  // SubRip
	".ass",  // Advanced SubStation Alpha
	".ssa",  // SubStation Alpha
	".vtt",  // WebVTT
	".sub",  // MicroDVD/SubViewer
	".idx",  // VobSub index
	".sup",  // Blu-ray PGS
	".usf",  // Universal Subtitle Format
	".ttml", // Timed Text Markup Language
	".dfxp", // Distribution Format Exchange Profile
	".sbv",  // YouTube
	".lrc",  // LyRiCs
	".rt",   // RealText
	".smi",  // SAMI
	".stl",  // Spruce subtitle format
	".txt",  // Plain text (sometimes used for subtitles)
}

var mediaExts = []string{
	// Container formats that support embedded subtitles
	".mkv",  // Matroska Video
	".mp4",  // MPEG-4 Part 14
	".m4v",  // iTunes Video
	".mov",  // QuickTime Movie
	".avi",  // Audio Video Interleave
	".wmv",  // Windows Media Video
	".flv",  // Flash Video
	".webm", // WebM
	".ogv",  // Ogg Video
	".3gp",  // 3GPP
	".3g2",  // 3GPP2
	".f4v",  // Flash MP4 Video
	".asf",  // Advanced Systems Format
	".rm",   // RealMedia
	".rmvb", // RealMedia Variable Bitrate
	".ts",   // MPEG Transport Stream
	".m2ts", // Blu-ray BDAV Transport Stream
	".mts",  // AVCHD Transport Stream
	".vob",  // DVD Video Object
	".mpg",  // MPEG Video
	".mpeg", // MPEG Video
	".m2v",  // MPEG-2 Video
	".divx", // DivX Video
	".xvid", // Xvid Video
}
