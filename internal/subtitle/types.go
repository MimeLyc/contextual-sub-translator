package subtitle

import (
	"strings"
	"time"

	"golang.org/x/text/language"
)

// Reader is the interface for reading subtitle files
type Reader interface {
	Read(path string) (*File, error)
}

// Writer is the interface for writing subtitle files
type Writer interface {
	Write(path string, subtitle *File) error
}

// SubtitleLine represents a single subtitle line
type Line struct {
	Index          int           // subtitle index
	StartTime      time.Duration // start time
	EndTime        time.Duration // end time
	Text           string        // subtitle text
	TranslatedText string        // translated text
}

// File represents subtitle file
type File struct {
	Lines    []Line
	Language language.Tag
	Format   string // e.g. SRT, ASS, VTT etc
}

type Description struct {
	Language    string
	SubLanguage string
	LangTag     language.Tag
}

type Descriptions []Description

func (d Descriptions) HasLanguage(lang language.Tag) bool {
	for _, desc := range d {
		if desc.LangTag == lang {
			return true
		}
	}
	return false
}

// FormatLanguageCode formats language code
func FormatLanguageCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	switch code {
	case "zh", "zh-cn", "zh_cn", "chi":
		return "Chinese"
	case "en":
		return "English"
	case "ja":
		return "Japanese"
	case "ko":
		return "Korean"
	case "fr":
		return "French"
	case "de":
		return "German"
	case "es":
		return "Spanish"
	default:
		return strings.ToUpper(code)
	}
}
