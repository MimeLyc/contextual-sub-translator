package subtitle

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abadojack/whatlanggo"
	"golang.org/x/text/language"
)

// DefaultReader is the default subtitle file reader
type DefaultReader struct {
	path string
}

// NewReader creates a new subtitle file reader
func NewReader(
	path string,
) Reader {
	return &DefaultReader{
		path: path,
	}
}

// ReadSubtitle reads subtitle file
func (r *DefaultReader) Read() (*File, error) {
	if !strings.HasSuffix(strings.ToLower(r.path), ".srt") {
		return nil, fmt.Errorf("only SRT format subtitle files are supported: %s", r.path)
	}

	if _, err := os.Stat(r.path); os.IsNotExist(err) {
		return nil, fmt.Errorf("subtitle file does not exist: %s", r.path)
	}

	file, err := os.Open(r.path)
	if err != nil {
		return nil, fmt.Errorf("failed to open subtitle file: %w", err)
	}
	defer file.Close()

	var lines []Line
	scanner := bufio.NewScanner(file)

	currentLine := Line{}
	state := "index" // possible values: "index", "time", "text", "empty"
	var textLines []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		switch state {
		case "index":
			if line == "" {
				continue
			}
			index, err := strconv.Atoi(line)
			if err != nil {
				continue // skip non-index lines
			}
			currentLine.Index = index
			state = "time"

		case "time":
			if line == "" {
				continue
			}
			startTime, endTime, err := parseSRTTime(line)
			if err != nil {
				return nil, fmt.Errorf("failed to parse time: %w", err)
			}
			currentLine.StartTime = startTime
			currentLine.EndTime = endTime
			state = "text"
			textLines = []string{}

		case "text":
			if line == "" {
				// subtitle text ends
				if len(textLines) > 0 {
					currentLine.Text = strings.Join(textLines, "\n")
					lines = append(lines, currentLine)
					currentLine = Line{}
				}
				state = "index"
				textLines = []string{}
			} else {
				textLines = append(textLines, line)
			}
		}
	}

	// handle last subtitle group
	if state == "text" && len(textLines) > 0 {
		currentLine.Text = strings.Join(textLines, "\n")
		lines = append(lines, currentLine)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read subtitle file: %w", err)
	}

	// detect language (simple detection based on text content)
	language := detectLanguage(lines)

	return &File{
		Lines:    lines,
		Language: language,
		Format:   "SRT",
	}, nil
}

// parseSRTTime parses SRT time format
func parseSRTTime(timeString string) (time.Duration, time.Duration, error) {
	// SRT time format: 00:02:16,612 --> 00:02:19,376
	re := regexp.MustCompile(`(\d{2}):(\d{2}):(\d{2}),(\d{3}) --> (\d{2}):(\d{2}):(\d{2}),(\d{3})`)
	matches := re.FindStringSubmatch(timeString)

	if len(matches) != 9 {
		return 0, 0, fmt.Errorf("invalid time format: %s", timeString)
	}

	parseTime := func(hours, minutes, seconds, milliseconds string) (time.Duration, error) {
		h, _ := strconv.Atoi(hours)
		m, _ := strconv.Atoi(minutes)
		s, _ := strconv.Atoi(seconds)
		ms, _ := strconv.Atoi(milliseconds)

		return time.Duration(h)*time.Hour +
			time.Duration(m)*time.Minute +
			time.Duration(s)*time.Second +
			time.Duration(ms)*time.Millisecond, nil
	}

	startTime, err := parseTime(matches[1], matches[2], matches[3], matches[4])
	if err != nil {
		return 0, 0, err
	}

	endTime, err := parseTime(matches[5], matches[6], matches[7], matches[8])
	if err != nil {
		return 0, 0, err
	}

	return startTime, endTime, nil
}

// detectLanguage simple language detection based on common characters
func detectLanguage(lines []Line) language.Tag {
	if len(lines) == 0 {
		return language.Und
	}

	langMap := make(map[string]int)

	for _, line := range lines {
		lang := whatlanggo.DetectLang(line.Text).Iso6391()
		if _, ok := langMap[lang]; !ok {
			langMap[lang] = 0
		}

		langMap[lang]++
	}

	// Get top language
	var topLang string
	var topCount int
	for lang, count := range langMap {
		if count > topCount {
			topLang = lang
			topCount = count
		}
	}

	return language.All.Make(topLang)
}
