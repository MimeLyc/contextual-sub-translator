package library

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/language"
)

type EmbeddedDetector func(mediaPath string) (hasEmbeddedSubtitle bool, hasEmbeddedTargetSubtitle bool, languages []string)

type scannerOptions struct {
	embeddedDetector EmbeddedDetector
	cacheTTL         time.Duration
}

type Option func(*scannerOptions)

func WithEmbeddedDetector(detector EmbeddedDetector) Option {
	return func(o *scannerOptions) {
		o.embeddedDetector = detector
	}
}

func WithCacheTTL(ttl time.Duration) Option {
	return func(o *scannerOptions) {
		o.cacheTTL = ttl
	}
}

type scanCache struct {
	version uint64
	scanned time.Time
	library *Library
}

type Scanner struct {
	sources          []SourceConfig
	targetLanguage   language.Tag
	embeddedDetector EmbeddedDetector

	mu            sync.RWMutex
	cacheTTL      time.Duration
	cache         *scanCache
	configVersion uint64
}

func NewScanner(
	sources []SourceConfig,
	targetLanguage language.Tag,
	opts ...Option,
) *Scanner {
	options := scannerOptions{
		embeddedDetector: func(string) (bool, bool, []string) { return false, false, nil },
		cacheTTL:         5 * time.Second,
	}
	for _, opt := range opts {
		opt(&options)
	}

	return &Scanner{
		sources:          sources,
		targetLanguage:   targetLanguage,
		embeddedDetector: options.embeddedDetector,
		cacheTTL:         options.cacheTTL,
	}
}

func (s *Scanner) TargetLanguage() string {
	s.mu.RLock()
	target := s.targetLanguage
	s.mu.RUnlock()

	base, _ := target.Base()
	return base.String()
}

func (s *Scanner) UpdateTargetLanguage(lang string) error {
	tag, err := language.Parse(lang)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if s.targetLanguage != tag {
		s.targetLanguage = tag
		s.cache = nil
		s.configVersion++
	}
	s.mu.Unlock()
	return nil
}

func (s *Scanner) Invalidate() {
	s.mu.Lock()
	s.cache = nil
	s.configVersion++
	s.mu.Unlock()
}

// resolveSeriesPath walks from the media file's directory upward toward
// sourcePath, looking for a tvshow.nfo file. If found, that directory is the
// series root. Otherwise falls back to the first subdirectory under sourcePath.
func resolveSeriesPath(sourcePath, mediaPath string) string {
	dir := filepath.Dir(mediaPath)
	for dir != sourcePath && strings.HasPrefix(dir, sourcePath) {
		nfo := filepath.Join(dir, "tvshow.nfo")
		if _, err := os.Stat(nfo); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Fallback: first subdirectory under sourcePath.
	// If media is directly in source dir, use source dir itself.
	rel, err := filepath.Rel(sourcePath, filepath.Dir(mediaPath))
	if err != nil || rel == "." {
		return sourcePath
	}
	first := strings.SplitN(rel, string(filepath.Separator), 2)[0]
	return filepath.Join(sourcePath, first)
}

// resolveSeasonName returns the season directory name (e.g. "Season 1")
// if the media file is nested inside a subdirectory of seriesPath.
// Returns "" if media is directly inside seriesPath.
func resolveSeasonName(seriesPath, mediaPath string) string {
	mediaDir := filepath.Dir(mediaPath)
	if mediaDir == seriesPath {
		return ""
	}
	rel, err := filepath.Rel(seriesPath, mediaDir)
	if err != nil || rel == "." {
		return ""
	}
	first := strings.SplitN(rel, string(filepath.Separator), 2)[0]
	return first
}

var sonarrPattern = regexp.MustCompile(`(?i)S\d+E(\d+)`)
var qualitySuffixPattern = regexp.MustCompile(`(?i)\s*[-. ](WEBRip|WEBDL|WEB-DL|BluRay|BDRip|HDRip|DVDRip|HDTV|AMZN|NF|DSNP|HULU|ATVP|PMTP|IT|DDP?\d|AAC|x264|x265|HEVC|H\.?264|H\.?265|10bit|\d{3,4}p).*$`)

// cleanEpisodeName parses Sonarr-style filenames and produces a short display name.
// e.g. "Gachiakuta - S01E15 - Clash! WEBRip-1080p" -> "E15 Clash!"
func cleanEpisodeName(basename string) string {
	m := sonarrPattern.FindStringSubmatchIndex(basename)
	if m == nil {
		return basename
	}
	epNum := basename[m[2]:m[3]]
	// Everything after the S##E## pattern marker
	after := strings.TrimSpace(basename[m[1]:])
	after = strings.TrimLeft(after, "-. ")
	after = strings.TrimSpace(after)
	// Strip quality suffixes
	after = qualitySuffixPattern.ReplaceAllString(after, "")
	after = strings.TrimSpace(after)
	if after != "" {
		return "E" + epNum + " " + after
	}
	return "E" + epNum
}

func (s *Scanner) Scan(ctx context.Context) (*Library, error) {
	s.mu.RLock()
	version := s.configVersion
	cacheTTL := s.cacheTTL
	if s.cache != nil && s.cache.version == version && (cacheTTL <= 0 || time.Since(s.cache.scanned) < cacheTTL) {
		cached := cloneLibrary(s.cache.library)
		s.mu.RUnlock()
		return cached, nil
	}
	sources := append([]SourceConfig(nil), s.sources...)
	targetLanguage := s.targetLanguage
	embeddedDetector := s.embeddedDetector
	s.mu.RUnlock()

	ret := &Library{
		Sources:  make([]Source, 0, len(sources)),
		Items:    make([]Item, 0),
		Episodes: make([]Episode, 0),
	}

	for _, sourceCfg := range sources {
		if sourceCfg.Path == "" {
			continue
		}
		if _, err := os.Stat(sourceCfg.Path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		source := Source{
			ID:   sourceCfg.ID,
			Name: sourceCfg.Name,
			Path: sourceCfg.Path,
		}

		itemIdxByPath := make(map[string]int)

		mediaFiles, err := findMediaFiles(sourceCfg.Path)
		if err != nil {
			return nil, err
		}
		for _, mediaPath := range mediaFiles {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			itemPath := resolveSeriesPath(sourceCfg.Path, mediaPath)
			itemIdx, ok := itemIdxByPath[itemPath]
			if !ok {
				item := Item{
					ID:       sourceCfg.ID + "|" + itemPath,
					SourceID: sourceCfg.ID,
					Name:     filepath.Base(itemPath),
					Path:     itemPath,
				}
				ret.Items = append(ret.Items, item)
				itemIdx = len(ret.Items) - 1
				itemIdxByPath[itemPath] = itemIdx
			}

			baseName := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
			mediaDir := filepath.Dir(mediaPath)
			sourceSubs, targetSubs, extLangs, err := findExternalSubtitles(mediaDir, baseName, targetLanguage)
			if err != nil {
				return nil, err
			}

			hasEmbedded, hasEmbeddedTarget, embeddedLangs := embeddedDetector(mediaPath)
			hasEmbeddedTarget = hasEmbeddedTarget || embeddedLanguagesContainTarget(embeddedLangs, targetLanguage)
			hasSource := len(sourceSubs) > 0 || hasEmbedded
			hasTarget := len(targetSubs) > 0 || hasEmbeddedTarget

			// Merge external and embedded languages (deduplicated, normalized)
			languages := extLangs
			seen := make(map[string]bool, len(extLangs))
			for _, l := range extLangs {
				seen[l] = true
			}
			for _, l := range embeddedLangs {
				normalized := normalizeLangCode(l)
				if normalized == "" {
					continue
				}
				if !seen[normalized] {
					seen[normalized] = true
					languages = append(languages, normalized)
				}
			}

			episode := Episode{
				ID:        mediaPath,
				SourceID:  sourceCfg.ID,
				ItemID:    ret.Items[itemIdx].ID,
				Name:      cleanEpisodeName(baseName),
				Season:    resolveSeasonName(itemPath, mediaPath),
				MediaPath: mediaPath,
				Subtitles: SubtitleStatus{
					HasSourceSubtitle:         hasSource,
					HasTargetSubtitle:         hasTarget,
					HasEmbeddedSubtitle:       hasEmbedded,
					HasEmbeddedTargetSubtitle: hasEmbeddedTarget,
					SourceSubtitleFiles:       sourceSubs,
					TargetSubtitleFiles:       targetSubs,
					Languages:                 languages,
				},
				Translatable: hasSource && !hasTarget,
			}
			ret.Episodes = append(ret.Episodes, episode)
			ret.Items[itemIdx].EpisodeCount++
		}

		source.ItemCount = len(itemIdxByPath)
		ret.Sources = append(ret.Sources, source)
	}

	s.mu.Lock()
	if s.configVersion == version {
		s.cache = &scanCache{
			version: version,
			scanned: time.Now(),
			library: cloneLibrary(ret),
		}
	}
	s.mu.Unlock()

	return ret, nil
}

var subtitleExts = []string{
	".srt", ".ass", ".ssa", ".vtt", ".sub", ".idx", ".sup", ".txt",
}

var mediaExts = []string{
	".mkv", ".mp4", ".m4v", ".mov", ".avi", ".wmv", ".flv", ".webm",
	".ogv", ".3gp", ".3g2", ".f4v", ".asf", ".rm", ".rmvb", ".ts",
	".m2ts", ".mts", ".vob", ".mpg", ".mpeg", ".m2v", ".divx", ".xvid",
}

func findMediaFiles(root string) ([]string, error) {
	ret := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if slices.Contains(mediaExts, ext) {
			ret = append(ret, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func findExternalSubtitles(dir string, mediaBase string, target language.Tag) (sourceSubs []string, targetSubs []string, languages []string, err error) {
	sourceSubs = make([]string, 0)
	targetSubs = make([]string, 0)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, nil, err
	}

	seen := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !slices.Contains(subtitleExts, ext) {
			continue
		}
		stem := strings.TrimSuffix(name, ext)
		if !subtitleMatchesMediaBase(stem, mediaBase) {
			continue
		}

		token := subtitleLangToken(stem, mediaBase)
		if lang := normalizeLangCode(token); lang != "" && !seen[lang] {
			seen[lang] = true
			languages = append(languages, lang)
		}

		fullPath := filepath.Join(dir, name)
		if token != "" && isTargetLanguage(token, target) {
			targetSubs = append(targetSubs, fullPath)
			continue
		}
		sourceSubs = append(sourceSubs, fullPath)
	}

	return sourceSubs, targetSubs, languages, nil
}

func subtitleLangToken(stem, mediaBase string) string {
	remain := strings.TrimPrefix(stem, mediaBase)
	remain = strings.TrimLeft(remain, "._- ")
	if remain == "" {
		return ""
	}

	parts := strings.FieldsFunc(remain, func(r rune) bool {
		return r == '.' || r == '_' || r == ' '
	})
	for i := len(parts) - 1; i >= 0; i-- {
		token := strings.ToLower(parts[i])
		if isLanguageToken(token) {
			return token
		}
	}
	return ""
}

// normalizeLangCode validates a language token and returns its normalized
// ISO 639-1 base code (e.g. "fre"→"fr", "eng"→"en", "chi"→"zh").
// Returns "" if the token is not a recognized language code.
func normalizeLangCode(token string) string {
	if token == "" {
		return ""
	}
	tag, err := language.Parse(token)
	if err != nil {
		return ""
	}
	base, conf := tag.Base()
	if conf == language.No {
		return ""
	}
	return base.String()
}

func isTargetLanguage(token string, target language.Tag) bool {
	token = strings.ToLower(strings.ReplaceAll(token, "_", "-"))
	if token == "" {
		return false
	}

	base, _ := target.Base()
	targetBase := strings.ToLower(base.String())
	if token == targetBase || strings.HasPrefix(token, targetBase+"-") {
		return true
	}

	// common aliases
	switch targetBase {
	case "zh":
		return token == "chi" || token == "chs" || token == "cht"
	case "en":
		return token == "eng"
	}

	return false
}

func subtitleMatchesMediaBase(stem, mediaBase string) bool {
	if stem == mediaBase {
		return true
	}
	if !strings.HasPrefix(stem, mediaBase) || len(stem) <= len(mediaBase) {
		return false
	}
	switch stem[len(mediaBase)] {
	case '.', '_', '-', ' ':
		return true
	default:
		return false
	}
}

func isLanguageToken(token string) bool {
	if token == "" {
		return false
	}
	if normalizeLangCode(token) != "" {
		return true
	}
	switch token {
	case "chs", "cht":
		return true
	default:
		return false
	}
}

func embeddedLanguagesContainTarget(languages []string, target language.Tag) bool {
	for _, lang := range languages {
		if lang == "" {
			continue
		}
		if isTargetLanguage(lang, target) {
			return true
		}
		if normalized := normalizeLangCode(lang); normalized != "" && isTargetLanguage(normalized, target) {
			return true
		}
	}
	return false
}

func cloneLibrary(src *Library) *Library {
	if src == nil {
		return nil
	}

	dst := &Library{
		Sources:  make([]Source, len(src.Sources)),
		Items:    make([]Item, len(src.Items)),
		Episodes: make([]Episode, len(src.Episodes)),
	}
	copy(dst.Sources, src.Sources)
	copy(dst.Items, src.Items)
	copy(dst.Episodes, src.Episodes)

	for i := range dst.Episodes {
		dst.Episodes[i].Subtitles.SourceSubtitleFiles = append([]string(nil), src.Episodes[i].Subtitles.SourceSubtitleFiles...)
		dst.Episodes[i].Subtitles.TargetSubtitleFiles = append([]string(nil), src.Episodes[i].Subtitles.TargetSubtitleFiles...)
		dst.Episodes[i].Subtitles.Languages = append([]string(nil), src.Episodes[i].Subtitles.Languages...)
	}
	return dst
}
