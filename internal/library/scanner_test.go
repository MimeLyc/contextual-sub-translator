package library

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestScanner_EpisodeSubtitleFlags(t *testing.T) {
	tmp := t.TempDir()
	showDir := filepath.Join(tmp, "tvshows", "The Show", "Season 1")
	require.NoError(t, os.MkdirAll(showDir, 0o755))

	mediaPath := filepath.Join(showDir, "episode01.mkv")
	srcSub := filepath.Join(showDir, "episode01.srt")
	tgtSub := filepath.Join(showDir, "episode01.zh.srt")

	require.NoError(t, os.WriteFile(mediaPath, []byte("media"), 0o644))
	require.NoError(t, os.WriteFile(srcSub, []byte("source"), 0o644))
	require.NoError(t, os.WriteFile(tgtSub, []byte("target"), 0o644))

	scanner := NewScanner(
		[]SourceConfig{
			{
				ID:   "tvshows",
				Name: "TV Shows",
				Path: filepath.Join(tmp, "tvshows"),
			},
		},
		language.Chinese,
		WithEmbeddedDetector(func(string) (bool, bool, []string) {
			return false, false, nil
		}),
	)

	lib, err := scanner.Scan(context.Background())
	require.NoError(t, err)

	require.Len(t, lib.Sources, 1)
	require.Len(t, lib.Items, 1)
	require.Len(t, lib.Episodes, 1)

	// Item should resolve to series dir "The Show", not "Season 1"
	assert.Equal(t, "The Show", lib.Items[0].Name)
	assert.Equal(t, filepath.Join(tmp, "tvshows", "The Show"), lib.Items[0].Path)

	ep := lib.Episodes[0]
	assert.Equal(t, "Season 1", ep.Season)
	assert.True(t, ep.Subtitles.HasSourceSubtitle)
	assert.True(t, ep.Subtitles.HasTargetSubtitle)
	assert.False(t, ep.Subtitles.HasEmbeddedSubtitle)
	assert.False(t, ep.Subtitles.HasEmbeddedTargetSubtitle)
	assert.False(t, ep.Translatable)
	// "zh" recognized as valid language; plain .srt has no language token
	assert.Equal(t, []string{"zh"}, ep.Subtitles.Languages)
}

func TestScanner_SeriesResolutionWithNFO(t *testing.T) {
	tmp := t.TempDir()
	seriesDir := filepath.Join(tmp, "animations", "Gachiakuta")
	seasonDir := filepath.Join(seriesDir, "Season 1")
	require.NoError(t, os.MkdirAll(seasonDir, 0o755))

	// Place tvshow.nfo at series level
	require.NoError(t, os.WriteFile(filepath.Join(seriesDir, "tvshow.nfo"), []byte("<tvshow/>"), 0o644))

	mediaPath := filepath.Join(seasonDir, "Gachiakuta - S01E15 - Clash! WEBRip-1080p.mkv")
	require.NoError(t, os.WriteFile(mediaPath, []byte("media"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(seasonDir, "Gachiakuta - S01E15 - Clash! WEBRip-1080p.srt"),
		[]byte("sub"), 0o644))

	scanner := NewScanner(
		[]SourceConfig{{ID: "anims", Name: "Animations", Path: filepath.Join(tmp, "animations")}},
		language.Chinese,
	)
	lib, err := scanner.Scan(context.Background())
	require.NoError(t, err)

	require.Len(t, lib.Items, 1)
	assert.Equal(t, "Gachiakuta", lib.Items[0].Name)
	assert.Equal(t, seriesDir, lib.Items[0].Path)

	require.Len(t, lib.Episodes, 1)
	ep := lib.Episodes[0]
	assert.Equal(t, "Season 1", ep.Season)
	assert.Equal(t, "E15 Clash!", ep.Name)
}

func TestScanner_MediaDirectlyInSeriesDir(t *testing.T) {
	tmp := t.TempDir()
	seriesDir := filepath.Join(tmp, "movies", "MyMovie")
	require.NoError(t, os.MkdirAll(seriesDir, 0o755))

	mediaPath := filepath.Join(seriesDir, "movie.mkv")
	require.NoError(t, os.WriteFile(mediaPath, []byte("media"), 0o644))

	scanner := NewScanner(
		[]SourceConfig{{ID: "movies", Name: "Movies", Path: filepath.Join(tmp, "movies")}},
		language.Chinese,
	)
	lib, err := scanner.Scan(context.Background())
	require.NoError(t, err)

	require.Len(t, lib.Items, 1)
	assert.Equal(t, "MyMovie", lib.Items[0].Name)

	require.Len(t, lib.Episodes, 1)
	assert.Equal(t, "", lib.Episodes[0].Season)
}

func TestScanner_MultipleSeasons(t *testing.T) {
	tmp := t.TempDir()
	seriesDir := filepath.Join(tmp, "tv", "Show")
	season1 := filepath.Join(seriesDir, "Season 1")
	season2 := filepath.Join(seriesDir, "Season 2")
	require.NoError(t, os.MkdirAll(season1, 0o755))
	require.NoError(t, os.MkdirAll(season2, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(seriesDir, "tvshow.nfo"), []byte("<tvshow/>"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(season1, "ep01.mkv"), []byte("m"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(season2, "ep01.mkv"), []byte("m"), 0o644))

	scanner := NewScanner(
		[]SourceConfig{{ID: "tv", Name: "TV", Path: filepath.Join(tmp, "tv")}},
		language.Chinese,
	)
	lib, err := scanner.Scan(context.Background())
	require.NoError(t, err)

	// Both episodes should be grouped under one item
	require.Len(t, lib.Items, 1)
	assert.Equal(t, "Show", lib.Items[0].Name)
	assert.Equal(t, 2, lib.Items[0].EpisodeCount)

	// Each episode has its own season
	seasons := map[string]bool{}
	for _, ep := range lib.Episodes {
		seasons[ep.Season] = true
	}
	assert.True(t, seasons["Season 1"])
	assert.True(t, seasons["Season 2"])
}

func TestScanner_LanguageFiltering(t *testing.T) {
	tmp := t.TempDir()
	showDir := filepath.Join(tmp, "shows", "Anime")
	require.NoError(t, os.MkdirAll(showDir, 0o755))

	mediaPath := filepath.Join(showDir, "ep01.mkv")
	require.NoError(t, os.WriteFile(mediaPath, []byte("m"), 0o644))
	// _ctxtrans is a tool suffix, not a language — must be excluded
	require.NoError(t, os.WriteFile(filepath.Join(showDir, "ep01_ctxtrans.srt"), []byte("s"), 0o644))
	// fre (ISO 639-2 for French) — must normalize to "fr"
	require.NoError(t, os.WriteFile(filepath.Join(showDir, "ep01.fre.srt"), []byte("s"), 0o644))
	// eng → "en"
	require.NoError(t, os.WriteFile(filepath.Join(showDir, "ep01.eng.srt"), []byte("s"), 0o644))

	scanner := NewScanner(
		[]SourceConfig{{ID: "shows", Name: "Shows", Path: filepath.Join(tmp, "shows")}},
		language.Chinese,
	)
	lib, err := scanner.Scan(context.Background())
	require.NoError(t, err)
	require.Len(t, lib.Episodes, 1)

	langs := lib.Episodes[0].Subtitles.Languages
	assert.Contains(t, langs, "fr")
	assert.Contains(t, langs, "en")
	assert.NotContains(t, langs, "ctxtrans")
	assert.NotContains(t, langs, "fre")
	assert.NotContains(t, langs, "eng")
	assert.NotContains(t, langs, "unknown")
}

func TestCleanEpisodeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Gachiakuta - S01E15 - Clash! WEBRip-1080p", "E15 Clash!"},
		{"Show - S02E03 - The Title", "E03 The Title"},
		{"Show - S01E01 - Pilot HDTV-720p", "E01 Pilot"},
		{"Show.S01E05.Episode.Name.1080p.WEB-DL", "E05 Episode.Name"},
		{"S01E01", "E01"},
		{"no-match-here", "no-match-here"},
		{"Show - S01E12 - Title x264-GROUP", "E12 Title"},
		{"Show - S01E08 - Title BluRay-1080p", "E08 Title"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, cleanEpisodeName(tt.input))
		})
	}
}

func TestResolveSeriesPath(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "source")
	seriesDir := filepath.Join(sourcePath, "MySeries")
	seasonDir := filepath.Join(seriesDir, "Season 1")
	require.NoError(t, os.MkdirAll(seasonDir, 0o755))

	t.Run("with tvshow.nfo", func(t *testing.T) {
		nfoPath := filepath.Join(seriesDir, "tvshow.nfo")
		require.NoError(t, os.WriteFile(nfoPath, []byte("<tvshow/>"), 0o644))
		defer os.Remove(nfoPath)

		mediaPath := filepath.Join(seasonDir, "ep01.mkv")
		got := resolveSeriesPath(sourcePath, mediaPath)
		assert.Equal(t, seriesDir, got)
	})

	t.Run("without tvshow.nfo fallback to first subdir", func(t *testing.T) {
		mediaPath := filepath.Join(seasonDir, "ep01.mkv")
		got := resolveSeriesPath(sourcePath, mediaPath)
		assert.Equal(t, seriesDir, got)
	})

	t.Run("media directly in source dir", func(t *testing.T) {
		mediaPath := filepath.Join(sourcePath, "movie.mkv")
		got := resolveSeriesPath(sourcePath, mediaPath)
		assert.Equal(t, sourcePath, got)
	})
}

func TestResolveSeasonName(t *testing.T) {
	tests := []struct {
		name       string
		seriesPath string
		mediaPath  string
		want       string
	}{
		{"nested season", "/tv/Show", "/tv/Show/Season 1/ep01.mkv", "Season 1"},
		{"no season", "/tv/Show", "/tv/Show/ep01.mkv", ""},
		{"deeply nested", "/tv/Show", "/tv/Show/Season 2/extras/ep01.mkv", "Season 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveSeasonName(tt.seriesPath, tt.mediaPath))
		})
	}
}

func TestNormalizeLangCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"en", "en"},
		{"eng", "en"},
		{"fre", "fr"},
		{"fra", "fr"},
		{"chi", "zh"},
		{"zh", "zh"},
		{"ja", "ja"},
		{"jpn", "ja"},
		{"ko", "ko"},
		{"ctxtrans", ""},
		{"forced", ""},
		{"sdh", "sdh"}, // ISO 639-3 Shehri — valid language code
		{"default", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeLangCode(tt.input))
		})
	}
}

func TestScanner_Scan_UsesCacheUntilInvalidate(t *testing.T) {
	tmp := t.TempDir()
	showDir := filepath.Join(tmp, "shows", "Anime")
	require.NoError(t, os.MkdirAll(showDir, 0o755))

	mediaPath := filepath.Join(showDir, "ep01.mkv")
	require.NoError(t, os.WriteFile(mediaPath, []byte("m"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(showDir, "ep01.eng.srt"), []byte("s"), 0o644))

	var detectorCalls atomic.Int32
	scanner := NewScanner(
		[]SourceConfig{{ID: "shows", Name: "Shows", Path: filepath.Join(tmp, "shows")}},
		language.Chinese,
		WithEmbeddedDetector(func(string) (bool, bool, []string) {
			detectorCalls.Add(1)
			return false, false, nil
		}),
		WithCacheTTL(10*time.Second),
	)

	_, err := scanner.Scan(context.Background())
	require.NoError(t, err)
	_, err = scanner.Scan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), detectorCalls.Load())

	scanner.Invalidate()
	_, err = scanner.Scan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(2), detectorCalls.Load())
}

func TestScanner_SubtitleMatchRequiresBoundaryAfterMediaBase(t *testing.T) {
	tmp := t.TempDir()
	sourceDir := filepath.Join(tmp, "shows", "Series")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "ep1.mkv"), []byte("m"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "ep10.zh.srt"), []byte("s"), 0o644))

	scanner := NewScanner(
		[]SourceConfig{{ID: "shows", Name: "Shows", Path: filepath.Join(tmp, "shows")}},
		language.Chinese,
	)

	lib, err := scanner.Scan(context.Background())
	require.NoError(t, err)
	require.Len(t, lib.Episodes, 1)

	ep := lib.Episodes[0]
	assert.Equal(t, filepath.Join(sourceDir, "ep1.mkv"), ep.MediaPath)
	assert.False(t, ep.Subtitles.HasSourceSubtitle)
	assert.False(t, ep.Subtitles.HasTargetSubtitle)
	assert.False(t, ep.Translatable)
}

func TestScanner_SubtitleMatchAllowsMediaSuffixNoise(t *testing.T) {
	tmp := t.TempDir()
	sourceDir := filepath.Join(tmp, "shows", "Series")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))

	mediaName := "Show.S01E01.1080p.WEB-DL.mkv"
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, mediaName), []byte("m"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "Show.S01E01.srt"), []byte("s"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "Show.S01E01.zh.srt"), []byte("s"), 0o644))

	scanner := NewScanner(
		[]SourceConfig{{ID: "shows", Name: "Shows", Path: filepath.Join(tmp, "shows")}},
		language.Chinese,
	)

	lib, err := scanner.Scan(context.Background())
	require.NoError(t, err)
	require.Len(t, lib.Episodes, 1)

	ep := lib.Episodes[0]
	assert.Equal(t, filepath.Join(sourceDir, mediaName), ep.MediaPath)
	assert.True(t, ep.Subtitles.HasSourceSubtitle)
	assert.True(t, ep.Subtitles.HasTargetSubtitle)
	assert.False(t, ep.Translatable)
}

func TestScanner_ScanSources(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "tvshows")
	require.NoError(t, os.MkdirAll(filepath.Join(sourcePath, "Show1"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(sourcePath, "Show2"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(sourcePath, ".hidden"), 0o755))

	scanner := NewScanner(
		[]SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: sourcePath},
		},
		language.Chinese,
	)

	sources, err := scanner.ScanSources(context.Background())
	require.NoError(t, err)
	require.Len(t, sources, 1)
	assert.Equal(t, "tvshows", sources[0].ID)
	assert.Equal(t, "TV Shows", sources[0].Name)
	assert.Equal(t, 2, sources[0].ItemCount) // hidden dir excluded
}

func TestScanner_ScanSources_MultipleSources(t *testing.T) {
	tmp := t.TempDir()
	src1 := filepath.Join(tmp, "tv")
	src2 := filepath.Join(tmp, "movies")
	require.NoError(t, os.MkdirAll(filepath.Join(src1, "Show"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(src2, "Movie"), 0o755))

	scanner := NewScanner(
		[]SourceConfig{
			{ID: "tv", Name: "TV", Path: src1},
			{ID: "movies", Name: "Movies", Path: src2},
		},
		language.Chinese,
	)

	sources, err := scanner.ScanSources(context.Background())
	require.NoError(t, err)
	require.Len(t, sources, 2)
	assert.Equal(t, 1, sources[0].ItemCount)
	assert.Equal(t, 1, sources[1].ItemCount)
}

func TestScanner_ScanSources_NonexistentPath(t *testing.T) {
	scanner := NewScanner(
		[]SourceConfig{
			{ID: "missing", Name: "Missing", Path: "/nonexistent/path/xyz"},
		},
		language.Chinese,
	)

	sources, err := scanner.ScanSources(context.Background())
	require.NoError(t, err)
	require.Len(t, sources, 0)
}

func TestScanner_ScanItems(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "tvshows")
	showDir := filepath.Join(sourcePath, "The Show")
	require.NoError(t, os.MkdirAll(showDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(showDir, "ep01.mkv"), []byte("m"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(showDir, "ep02.mkv"), []byte("m"), 0o644))

	// Empty dir should be skipped
	emptyDir := filepath.Join(sourcePath, "EmptyShow")
	require.NoError(t, os.MkdirAll(emptyDir, 0o755))

	scanner := NewScanner(
		[]SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: sourcePath},
		},
		language.Chinese,
	)

	items, err := scanner.ScanItems(context.Background(), "tvshows")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "The Show", items[0].Name)
	assert.Equal(t, 2, items[0].EpisodeCount)
	assert.Equal(t, "tvshows", items[0].SourceID)
	assert.Equal(t, "tvshows|"+showDir, items[0].ID)
}

func TestScanner_ScanItems_UnknownSource(t *testing.T) {
	scanner := NewScanner(
		[]SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: "/tmp"},
		},
		language.Chinese,
	)

	items, err := scanner.ScanItems(context.Background(), "nonexistent")
	require.NoError(t, err)
	require.Nil(t, items)
}

func TestScanner_ScanEpisodesByItem(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "tvshows")
	showDir := filepath.Join(sourcePath, "The Show")
	seasonDir := filepath.Join(showDir, "Season 1")
	require.NoError(t, os.MkdirAll(seasonDir, 0o755))

	mediaPath := filepath.Join(seasonDir, "episode01.mkv")
	srcSub := filepath.Join(seasonDir, "episode01.srt")
	tgtSub := filepath.Join(seasonDir, "episode01.zh.srt")
	require.NoError(t, os.WriteFile(mediaPath, []byte("media"), 0o644))
	require.NoError(t, os.WriteFile(srcSub, []byte("source"), 0o644))
	require.NoError(t, os.WriteFile(tgtSub, []byte("target"), 0o644))

	scanner := NewScanner(
		[]SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: sourcePath},
		},
		language.Chinese,
		WithEmbeddedDetector(func(string) (bool, bool, []string) {
			return false, false, nil
		}),
	)

	itemID := "tvshows|" + showDir
	episodes, err := scanner.ScanEpisodesByItem(context.Background(), itemID)
	require.NoError(t, err)
	require.Len(t, episodes, 1)

	ep := episodes[0]
	assert.Equal(t, mediaPath, ep.ID)
	assert.Equal(t, "tvshows", ep.SourceID)
	assert.Equal(t, itemID, ep.ItemID)
	assert.Equal(t, "Season 1", ep.Season)
	assert.True(t, ep.Subtitles.HasSourceSubtitle)
	assert.True(t, ep.Subtitles.HasTargetSubtitle)
	assert.False(t, ep.Translatable)
}

func TestScanner_ScanEpisodesByItem_InvalidID(t *testing.T) {
	scanner := NewScanner(
		[]SourceConfig{{ID: "tv", Name: "TV", Path: "/tmp"}},
		language.Chinese,
	)

	episodes, err := scanner.ScanEpisodesByItem(context.Background(), "no-pipe-separator")
	require.NoError(t, err)
	require.Nil(t, episodes)
}

func TestScanner_ScanEpisodesByItem_ParallelDetector(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "tvshows")
	showDir := filepath.Join(sourcePath, "The Show")
	require.NoError(t, os.MkdirAll(showDir, 0o755))

	for i := 0; i < 10; i++ {
		name := filepath.Join(showDir, "ep"+string(rune('a'+i))+".mkv")
		require.NoError(t, os.WriteFile(name, []byte("m"), 0o644))
	}

	var detectorCalls atomic.Int32
	scanner := NewScanner(
		[]SourceConfig{{ID: "tv", Name: "TV", Path: sourcePath}},
		language.Chinese,
		WithEmbeddedDetector(func(string) (bool, bool, []string) {
			detectorCalls.Add(1)
			time.Sleep(10 * time.Millisecond)
			return true, false, []string{"eng"}
		}),
		WithMaxConcurrency(4),
	)

	itemID := "tv|" + showDir
	episodes, err := scanner.ScanEpisodesByItem(context.Background(), itemID)
	require.NoError(t, err)
	require.Len(t, episodes, 10)
	assert.Equal(t, int32(10), detectorCalls.Load())

	// All episodes should have embedded detected
	for _, ep := range episodes {
		assert.True(t, ep.Subtitles.HasEmbeddedSubtitle)
		assert.True(t, ep.Subtitles.HasSourceSubtitle)
	}
}

func TestScanner_TieredCaches(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "tvshows")
	showDir := filepath.Join(sourcePath, "Show")
	require.NoError(t, os.MkdirAll(showDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(showDir, "ep01.mkv"), []byte("m"), 0o644))

	var detectorCalls atomic.Int32
	scanner := NewScanner(
		[]SourceConfig{{ID: "tv", Name: "TV", Path: sourcePath}},
		language.Chinese,
		WithEmbeddedDetector(func(string) (bool, bool, []string) {
			detectorCalls.Add(1)
			return false, false, nil
		}),
		WithSourcesCacheTTL(10*time.Second),
		WithItemsCacheTTL(10*time.Second),
		WithEpisodesCacheTTL(10*time.Second),
	)

	// First calls populate caches
	_, err := scanner.ScanSources(context.Background())
	require.NoError(t, err)
	_, err = scanner.ScanItems(context.Background(), "tv")
	require.NoError(t, err)
	_, err = scanner.ScanEpisodesByItem(context.Background(), "tv|"+showDir)
	require.NoError(t, err)
	assert.Equal(t, int32(1), detectorCalls.Load())

	// Second calls should hit cache
	_, err = scanner.ScanSources(context.Background())
	require.NoError(t, err)
	_, err = scanner.ScanItems(context.Background(), "tv")
	require.NoError(t, err)
	_, err = scanner.ScanEpisodesByItem(context.Background(), "tv|"+showDir)
	require.NoError(t, err)
	assert.Equal(t, int32(1), detectorCalls.Load()) // no new detector calls
}

func TestScanner_Invalidate_ClearsAllCaches(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "tvshows")
	showDir := filepath.Join(sourcePath, "Show")
	require.NoError(t, os.MkdirAll(showDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(showDir, "ep01.mkv"), []byte("m"), 0o644))

	var detectorCalls atomic.Int32
	scanner := NewScanner(
		[]SourceConfig{{ID: "tv", Name: "TV", Path: sourcePath}},
		language.Chinese,
		WithEmbeddedDetector(func(string) (bool, bool, []string) {
			detectorCalls.Add(1)
			return false, false, nil
		}),
		WithSourcesCacheTTL(10*time.Second),
		WithItemsCacheTTL(10*time.Second),
		WithEpisodesCacheTTL(10*time.Second),
	)

	// Populate all caches
	_, err := scanner.ScanSources(context.Background())
	require.NoError(t, err)
	_, err = scanner.ScanItems(context.Background(), "tv")
	require.NoError(t, err)
	_, err = scanner.ScanEpisodesByItem(context.Background(), "tv|"+showDir)
	require.NoError(t, err)
	assert.Equal(t, int32(1), detectorCalls.Load())

	// Invalidate clears everything
	scanner.Invalidate()

	// Next calls should re-scan
	_, err = scanner.ScanSources(context.Background())
	require.NoError(t, err)
	_, err = scanner.ScanItems(context.Background(), "tv")
	require.NoError(t, err)
	_, err = scanner.ScanEpisodesByItem(context.Background(), "tv|"+showDir)
	require.NoError(t, err)
	assert.Equal(t, int32(2), detectorCalls.Load())
}

func TestScanner_UpdateTargetLanguage_TakesEffectImmediately(t *testing.T) {
	tmp := t.TempDir()
	showDir := filepath.Join(tmp, "shows", "Anime")
	require.NoError(t, os.MkdirAll(showDir, 0o755))

	mediaPath := filepath.Join(showDir, "ep01.mkv")
	require.NoError(t, os.WriteFile(mediaPath, []byte("m"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(showDir, "ep01.eng.srt"), []byte("s"), 0o644))

	scanner := NewScanner(
		[]SourceConfig{{ID: "shows", Name: "Shows", Path: filepath.Join(tmp, "shows")}},
		language.Chinese,
		WithCacheTTL(10*time.Second),
	)

	lib, err := scanner.Scan(context.Background())
	require.NoError(t, err)
	require.Len(t, lib.Episodes, 1)
	assert.True(t, lib.Episodes[0].Translatable)

	require.NoError(t, scanner.UpdateTargetLanguage("en"))

	lib, err = scanner.Scan(context.Background())
	require.NoError(t, err)
	require.Len(t, lib.Episodes, 1)
	assert.False(t, lib.Episodes[0].Translatable)
	assert.True(t, lib.Episodes[0].Subtitles.HasTargetSubtitle)
}
