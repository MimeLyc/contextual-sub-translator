package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"
	"golang.org/x/text/language"

	"github.com/MimeLyc/contextual-sub-translator/internal/config"
	"github.com/MimeLyc/contextual-sub-translator/internal/llm"
	"github.com/MimeLyc/contextual-sub-translator/internal/media"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"github.com/MimeLyc/contextual-sub-translator/internal/translator"
	"github.com/MimeLyc/contextual-sub-translator/pkg/file"
	"github.com/MimeLyc/contextual-sub-translator/pkg/icron"
	"github.com/MimeLyc/contextual-sub-translator/pkg/log"
	"github.com/robfig/cron/v3"
)

type transService struct {
	cfg            config.Config
	lastTrigerTime time.Time
	cronExpr       string
	cron           *cron.Cron
}

func NewRunnableTransService(
	cfg config.Config,
	cron *cron.Cron,
) transService {
	return transService{
		cfg:      cfg,
		cronExpr: cfg.Translate.CronExpr,
		cron:     cron,
	}
}

var singleflightGroup singleflight.Group

func (s transService) Schedule(
	ctx context.Context,
) error {
	log.Info("Run TransService")

	runFunc := func() {
		_, _, _ = singleflightGroup.Do("run", func() (any, error) {
			for _, dir := range s.cfg.Media.MediaPaths() {
				log.Info("Run in dir %s", dir)
				err := s.run(ctx, dir)
				if err != nil {
					log.Error("Failed to run in dir %s: %v", dir, err)
				}
			}
			return nil, nil
		})
	}
	_, err := s.cron.AddFunc(s.cronExpr, runFunc)
	return err
}

func (s transService) run(
	ctx context.Context,
	dir string,
) error {
	toTrans, err := s.findTargetMediaTuplesInDir(ctx, dir)
	if err != nil {
		log.Error("Failed to find target media tuples in dir %s: %v", dir, err)
		return err
	}
	log.Info("Found %d target media tuples in dir %s", len(toTrans), dir)

	llmClient, err := llm.NewClient(&llm.Config{
		APIKey:      s.cfg.LLM.APIKey,
		APIURL:      s.cfg.LLM.APIURL,
		Model:       s.cfg.LLM.Model,
		MaxTokens:   s.cfg.LLM.MaxTokens,
		Temperature: s.cfg.LLM.Temperature,
		Timeout:     s.cfg.LLM.Timeout,
		SiteURL:     s.cfg.LLM.SiteURL,
		AppName:     s.cfg.LLM.AppName,
	})
	if err != nil {
		log.Error("Failed to create LLM client: %v", err)
		return err
	}

	aitranslator := translator.NewAiTranslator(*llmClient)

	for _, bundle := range toTrans {
		targetSub := bundle.SubtitleFiles[0]

		log.Info("Translating subtitle media %s from %s to %s", bundle.MediaFile, targetSub.Language, s.cfg.Translate.TargetLanguage)
		transLator, err := NewTranslator(
			TranslatorConfig{
				TargetLanguage: s.cfg.Translate.TargetLanguage,
				ContextEnabled: true,
				SubtitleFile:   &targetSub,
				OutputDir:      filepath.Dir(bundle.MediaFile),
				InputPath:      targetSub.Path,
			},
			aitranslator,
		)
		if err != nil {
			log.Error("Failed to create translator: %v", err)
			return err
		}

		// TODO: check if nfo file exists
		if res, err := transLator.Translate(ctx, bundle.NFOFiles[0].Path); err != nil {
			log.Error("Failed to translate subtitle media %s: %v", bundle.MediaFile, err)
			return err
		} else {
			log.Info("Translated subtitle media %s: %v", bundle.MediaFile, res)
		}
	}
	return nil
}

func (s transService) findTargetMediaTuplesInDir(
	ctx context.Context,
	dir string,
) (ret []MediaBundle, err error) {
	all, err := s.findSourceBundlesInDir(ctx, dir)
	if err != nil {
		return
	}

	ret = make([]MediaBundle, 0, len(all))
	for _, bundle := range all {
		subtitles, err := s.readSubtitleFiles(ctx, bundle.SubtitleFiles)
		if err != nil {
			log.Error("Failed to read subtitle files of media file %s: %v", bundle.MediaFile, err)
			continue
		}

		// If target subtitle exists, skip
		if containTargetSubtitle(subtitles, s.cfg.Translate.TargetLanguage) {
			continue
		}

		// If target subtitle is built into media file, skip
		mediaReader := media.NewOperator(bundle.MediaFile)
		subDescs, err := mediaReader.ReadSubtitleDescription()
		if err != nil {
			log.Error("Failed to read subtitle description of media file %s: %v", bundle.MediaFile, err)
			continue
		}
		if subDescs.HasLanguage(s.cfg.Translate.TargetLanguage) {
			log.Info("Target subtitle already exists in media file %s", bundle.MediaFile)
			continue
		}

		// Read NFO files
		nfos := make([]media.TVShowInfo, len(bundle.NFOFiles))
		for i, nfo := range bundle.NFOFiles {
			tmp, err := NewNFOReader().ReadTVShowInfo(nfo)
			if err != nil {
				log.Error("Failed to read NFO file %s: %v", nfo, err)
				continue
			}
			nfos[i] = *tmp
		}

		// There is no target subtitle, extract one from media file
		if len(subtitles) == 0 && len(subDescs) > 0 {
			output, err := mediaReader.DefExtractSubtitle()
			if err != nil {
				log.Error("Failed to extract subtitle from media file %s: %v", bundle.MediaFile, err)
				continue
			}
			sub, err := subtitle.NewReader(output).Read()
			if err != nil {
				log.Error("Failed to read subtitle file %s: %v", output, err)
				continue
			}

			ret = append(ret, MediaBundle{
				MediaFile:     bundle.MediaFile,
				SubtitleFiles: []subtitle.File{*sub},
				NFOFiles:      nfos,
			})
		} else {
			ret = append(ret, MediaBundle{
				MediaFile:     bundle.MediaFile,
				SubtitleFiles: subtitles,
				NFOFiles:      nfos,
			})
		}
	}

	return
}

// containTargetSubtitle checks if any subtitle file has the target language
func containTargetSubtitle(subtitles []subtitle.File, targetLanguage language.Tag) bool {
	for _, sub := range subtitles {
		if sub.Language.String() == targetLanguage.String() {
			return true
		}
	}
	return false
}

func (s transService) readSubtitleFiles(
	ctx context.Context,
	paths []string,
) ([]subtitle.File, error) {
	ret := make([]subtitle.File, 0, len(paths))

	for _, path := range paths {
		file, err := subtitle.NewReader(path).Read()
		if err != nil {
			return nil, fmt.Errorf("failed to read subtitle file %s: %w", path, err)
		}
		ret = append(ret, *file)
	}

	return ret, nil
}

func (s transService) findSourceBundlesInDir(
	_ context.Context,
	dir string,
) ([]MediaPathBundle, error) {
	// check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("directory %s does not exist", dir)
	}

	startTime, err := s.startTime()
	if err != nil {
		return nil, fmt.Errorf("failed to get start time: %w", err)
	}
	log.Info("Start searching target metdia files after time: %v", startTime)

	// Step 1: Find target files (subtitles or media files)
	var targetFiles []string

	// Find files modified after lastTrigerTime
	recentFiles, err := file.FindRecentAfter(dir, startTime)
	if err != nil {
		return nil, fmt.Errorf("failed to find recent files: %w", err)
	}

	// Filter for target files (subtitles or media files)
	for _, filePath := range recentFiles {
		ext := strings.ToLower(filepath.Ext(filePath))
		if isSubtitleFile(ext) || isMediaFile(ext) {
			targetFiles = append(targetFiles, filePath)
		}
	}

	// Step 2: For each target file, find matching files
	var bundles []MediaPathBundle
	processedBases := make(map[string]bool)

	for _, targetFile := range targetFiles {
		baseName := getBaseName(targetFile)
		baseDir := filepath.Dir(targetFile)

		// Skip if already processed this base name
		if processedBases[baseName] {
			continue
		}
		processedBases[baseName] = true

		bundle := MediaPathBundle{}

		// Find matching subtitle files
		bundle.SubtitleFiles = findMatchingSubtitleFiles(baseDir, baseName)

		// Find matching media file
		bundle.MediaFile = findMatchingMediaFile(baseDir, baseName)

		// Find NFO files in current or parent directories
		bundle.NFOFiles = findNFOFiles(baseDir)

		// Add bundle if it has at least a subtitle or media file
		if len(bundle.SubtitleFiles) > 0 || bundle.MediaFile != "" {
			bundles = append(bundles, bundle)
		}
	}

	return bundles, nil
}

// getBaseName extracts the base name of a file
// e.g. "movie.mkv" -> "movie"
// e.g. "movie.eng.srt" -> "movie"
func getBaseName(filePath string) string {
	fileName := filepath.Base(filePath)
	if !strings.Contains(fileName, ".") {
		return fileName
	}
	return strings.Split(fileName, ".")[0]
}

// findMatchingSubtitleFiles finds all subtitle files with the same base name
func findMatchingSubtitleFiles(dir, baseName string) []string {
	var subtitleFiles []string

	// Read all files in the directory
	files, err := os.ReadDir(dir)
	if err != nil {
		return subtitleFiles
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileName := file.Name()
		for _, ext := range subtitleExts {
			// Check if file starts with baseName and ends with the subtitle extension
			if strings.HasPrefix(fileName, baseName) && strings.HasSuffix(fileName, ext) {
				subtitleFiles = append(subtitleFiles, filepath.Join(dir, fileName))
			}
		}
	}

	return subtitleFiles
}

// findMatchingMediaFile finds a media file with the same base name
func findMatchingMediaFile(dir, baseName string) string {
	for _, ext := range mediaExts {
		targetPath := filepath.Join(dir, baseName+ext)
		if _, err := os.Stat(targetPath); err == nil {
			return targetPath
		}
	}

	return ""
}

// findNFOFiles searches for NFO files in current directory and parent directories
func findNFOFiles(startDir string) []string {
	var nfoFiles []string
	currentDir := startDir
	// TODO: I don't know whether all medias follow the same naming convention
	nfoNames := []string{"tvshow.nfo", "season.nfo", "show.nfo"}

	for {
		// Check for NFO files in current directory
		for _, nfoName := range nfoNames {
			nfoPath := filepath.Join(currentDir, nfoName)
			if _, err := os.Stat(nfoPath); err == nil {
				nfoFiles = append(nfoFiles, nfoPath)
			}
		}

		// Move to parent directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// Reached root directory
			break
		}
		currentDir = parentDir
	}

	return nfoFiles
}

// isSubtitleFile checks if the file extension is a subtitle format
func isSubtitleFile(ext string) bool {
	return slices.Contains(subtitleExts, ext)
}

// isMediaFile checks if the file extension is a media format that supports embedded subtitles
func isMediaFile(ext string) bool {
	return slices.Contains(mediaExts, ext)
}

func (s transService) startTime() (time.Time, error) {
	if s.lastTrigerTime.IsZero() {
		cronSchedule, err := icron.GetTriggerInfo(s.cronExpr, time.Now())
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to get cron schedule: %w", err)
		}

		if time.Now().Add(-24 * 1 * time.Hour).Before(cronSchedule.Last) {
			return time.Now().Add(-24 * 7 * time.Hour), nil
		}
		return cronSchedule.Last, nil
	}

	return s.lastTrigerTime, nil
}
