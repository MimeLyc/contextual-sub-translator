package subtitle

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

// DefaultWriter is the default subtitle file writer
type DefaultWriter struct{}

// NewWriter creates a new subtitle file writer
func NewWriter() Writer {
	return &DefaultWriter{}
}

// WriteSubtitle writes subtitle file to specified path
func (w *DefaultWriter) Write(path string, subtitle *File) error {
	if subtitle == nil {
		return fmt.Errorf("subtitle data is empty")
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	for _, line := range subtitle.Lines {
		// write index
		fmt.Fprintf(writer, "%d\n", line.Index)

		// write time
		startTime := formatDuration(line.StartTime)
		endTime := formatDuration(line.EndTime)
		fmt.Fprintf(writer, "%s --> %s\n", startTime, endTime)

		// write text (use translated text, fallback to original if empty)
		text := line.TranslatedText
		if text == "" {
			text = line.Text
		}
		fmt.Fprintf(writer, "%s\n\n", text)
	}

	return nil
}

// formatDuration formats time.Duration to SRT time format
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	milliseconds := int(d.Milliseconds()) % 1000

	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, milliseconds)
}
