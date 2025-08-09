package ctxtrans

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MimeLyc/contextual-sub-translator/internal/media"
)

// XMLTVShow is the XML structure for NFO files
type XMLTVShow struct {
	Title         string `xml:"title"`
	OriginalTitle string `xml:"originaltitle"`
	Plot          string `xml:"plot"`
	Genres        []struct {
		Genre string `xml:"genre"`
	} `xml:"genre"`
	Premiered string  `xml:"premiered"`
	Rating    float32 `xml:"rating"`
	Studio    string  `xml:"studio"`
	Actors    []struct {
		Name  string `xml:"name"`
		Role  string `xml:"role"`
		Order int    `xml:"order"`
	} `xml:"actor"`
	Aired  string `xml:"aired"`
	Year   int    `xml:"year"`
	Season int    `xml:"season"`
}

// DefaultNFOReader is the default NFO file reader
type DefaultNFOReader struct{}

// NewNFOReader creates a new NFO reader
func NewNFOReader() NFOReader {
	return &DefaultNFOReader{}
}

// ReadTVShowInfo reads TV show information from NFO file
func (r *DefaultNFOReader) ReadTVShowInfo(path string) (*media.TVShowInfo, error) {
	if !strings.HasSuffix(strings.ToLower(path), ".nfo") {
		return nil, fmt.Errorf("file extension must be .nfo: %s", path)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("NFO file does not exist: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read NFO file: %w", err)
	}

	var xmlShow XMLTVShow
	if err := xml.Unmarshal(data, &xmlShow); err != nil {
		return nil, fmt.Errorf("failed to parse XML: %w", err)
	}

	return r.convertToTVShowInfo(xmlShow), nil
}

// convertToTVShowInfo converts XML structure to internal structure
func (r *DefaultNFOReader) convertToTVShowInfo(xmlShow XMLTVShow) *media.TVShowInfo {
	show := &media.TVShowInfo{
		Title:         strings.TrimSpace(xmlShow.Title),
		OriginalTitle: strings.TrimSpace(xmlShow.OriginalTitle),
		Plot:          strings.TrimSpace(xmlShow.Plot),
		Premiered:     strings.TrimSpace(xmlShow.Premiered),
		Rating:        xmlShow.Rating,
		Studio:        strings.TrimSpace(xmlShow.Studio),
		Aired:         strings.TrimSpace(xmlShow.Aired),
		Year:          xmlShow.Year,
		Season:        xmlShow.Season,
	}

	// Process genres
	for _, g := range xmlShow.Genres {
		if genre := strings.TrimSpace(g.Genre); genre != "" {
			show.Genre = append(show.Genre, genre)
		}
	}

	// Process actors
	for _, a := range xmlShow.Actors {
		if name := strings.TrimSpace(a.Name); name != "" {
			show.Actors = append(show.Actors, media.Actor{
				Name:  name,
				Role:  strings.TrimSpace(a.Role),
				Order: a.Order,
			})
		}
	}

	return show
}

// ReadTVShowInfoSafe safely reads NFO file, ignoring some common errors
func ReadTVShowInfoSafe(path string) (*media.TVShowInfo, error) {
	reader := NewNFOReader()

	// Ensure path is absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	// If file doesn't exist, try to find in parent directory
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		// Try to find any .nfo file in parent directory
		dir := filepath.Dir(absPath)
		files, err := filepath.Glob(filepath.Join(dir, "*.nfo"))
		if err == nil && len(files) > 0 {
			absPath = files[0] // Use first found .nfo file
		} else {
			return nil, fmt.Errorf("no NFO file found")
		}
	}

	return reader.ReadTVShowInfo(absPath)
}

// GetContextTextFromTVShow generates context text for LLM from TVShowInfo
func GetContextTextFromTVShow(show *media.TVShowInfo) string {
	if show == nil {
		return ""
	}

	var sb strings.Builder

	// Add title information
	if show.Title != "" {
		sb.WriteString(fmt.Sprintf("Show Title: %s\n", show.Title))
	}
	if show.OriginalTitle != "" && show.OriginalTitle != show.Title {
		sb.WriteString(fmt.Sprintf("Original Title: %s\n", show.OriginalTitle))
	}

	// Add genre information
	if len(show.Genre) > 0 {
		sb.WriteString(fmt.Sprintf("Genres: %s\n", strings.Join(show.Genre, ", ")))
	}

	// Add production information
	if show.Studio != "" {
		sb.WriteString(fmt.Sprintf("Production Studio: %s\n", show.Studio))
	}

	// Add airing information
	if show.Year > 0 {
		sb.WriteString(fmt.Sprintf("Year: %d\n", show.Year))
	}
	if show.Season > 0 {
		sb.WriteString(fmt.Sprintf("Season: %d\n", show.Season))
	}

	// Add cast information
	if len(show.Actors) > 0 {
		sb.WriteString("Main Cast:\n")
		for i, actor := range show.Actors {
			if i >= 5 { // Limit cast count
				break
			}
			if actor.Role != "" {
				sb.WriteString(fmt.Sprintf("- %s as %s\n", actor.Name, actor.Role))
			} else {
				sb.WriteString(fmt.Sprintf("- %s\n", actor.Name))
			}
		}
	}

	// Add plot summary
	if show.Plot != "" {
		sb.WriteString(fmt.Sprintf("\nPlot: %s", show.Plot))
	}

	return sb.String()
}
