package subtitle

import "time"

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
	Language string
	Format   string // e.g. SRT, ASS, VTT etc
}
