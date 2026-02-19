package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
	"golang.org/x/text/language"

	"github.com/MimeLyc/contextual-sub-translator/internal/agent"
	"github.com/MimeLyc/contextual-sub-translator/internal/config"
	"github.com/MimeLyc/contextual-sub-translator/internal/jobs"
	"github.com/MimeLyc/contextual-sub-translator/internal/media"
	"github.com/MimeLyc/contextual-sub-translator/internal/persistence"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"github.com/MimeLyc/contextual-sub-translator/internal/termmap"
	"github.com/MimeLyc/contextual-sub-translator/internal/tools"
	"github.com/MimeLyc/contextual-sub-translator/internal/translator"
	"github.com/MimeLyc/contextual-sub-translator/pkg/icron"
	"github.com/MimeLyc/contextual-sub-translator/pkg/log"
	"github.com/robfig/cron/v3"
)

type transService struct {
	mu             sync.RWMutex
	cfg            config.Config
	lastTrigerTime time.Time
	cronExpr       string
	cron           *cron.Cron
	jobQueue       *jobs.Queue
	store          *persistence.SQLiteStore
	runFunc        func()
	cronEntryID    cron.EntryID
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

func NewRunnableTransServiceWithQueue(
	cfg config.Config,
	cron *cron.Cron,
	queue *jobs.Queue,
) transService {
	return NewRunnableTransServiceWithQueueAndStore(cfg, cron, queue, nil)
}

func NewRunnableTransServiceWithQueueAndStore(
	cfg config.Config,
	cron *cron.Cron,
	queue *jobs.Queue,
	store *persistence.SQLiteStore,
) transService {
	svc := NewRunnableTransService(cfg, cron)
	svc.jobQueue = queue
	svc.store = store
	return svc
}

func (s *transService) enqueueCronBundle(bundle MediaPathBundle) (*jobs.TranslationJob, bool, error) {
	return s.enqueueBundle("cron", bundle)
}

func (s *transService) enqueueManualBundle(bundle MediaPathBundle) (*jobs.TranslationJob, bool, error) {
	return s.enqueueBundle("manual", bundle)
}

func (s *transService) enqueueBundle(source string, bundle MediaPathBundle) (*jobs.TranslationJob, bool, error) {
	if s.jobQueue == nil {
		return nil, false, fmt.Errorf("job queue is not configured")
	}
	dedupeKey := s.bundleDedupeKey(bundle)
	payload := jobs.JobPayload{
		MediaFile: bundle.MediaFile,
	}
	if len(bundle.SubtitleFiles) > 0 {
		payload.SubtitleFile = bundle.SubtitleFiles[0]
	}
	if len(bundle.NFOFiles) > 0 {
		payload.NFOFile = bundle.NFOFiles[0]
	}
	job, created := s.jobQueue.Enqueue(jobs.EnqueueRequest{
		Source:    source,
		DedupeKey: dedupeKey,
		Payload:   payload,
	})
	return job, created, nil
}

func (s *transService) bundleDedupeKey(bundle MediaPathBundle) string {
	cfg := s.configSnapshot()
	subPath := ""
	if len(bundle.SubtitleFiles) > 0 {
		subPath = bundle.SubtitleFiles[0]
	}
	return fmt.Sprintf(
		"%s|%s|%s",
		bundle.MediaFile,
		subPath,
		cfg.Translate.TargetLanguage.String(),
	)
}

var singleflightGroup singleflight.Group
var termMapFileLocks sync.Map

func (s *transService) configSnapshot() config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *transService) scheduleSnapshot() (cron.EntryID, string, func()) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cronEntryID, s.cronExpr, s.runFunc
}

func (s *transService) Schedule(
	ctx context.Context,
) error {
	log.Info("Run TransService")
	if s.jobQueue != nil {
		s.jobQueue.Start(func(execCtx context.Context, job *jobs.TranslationJob) error {
			llmAgent, searchEnabled, err := s.buildAgent()
			if err != nil {
				return err
			}
			return s.processJob(execCtx, job, llmAgent, searchEnabled)
		})
		go func() {
			<-ctx.Done()
			s.jobQueue.Stop()
		}()
	}

	runFunc := func() {
		_, _, _ = singleflightGroup.Do("run", func() (any, error) {
			cfg := s.configSnapshot()
			for _, dir := range cfg.Media.MediaPaths() {
				log.Info("Run in dir %s", dir)
				err := s.run(ctx, dir)
				if err != nil {
					log.Error("Failed to run in dir %s: %v", dir, err)
				}
			}
			return nil, nil
		})
	}
	// Run once immediately on startup
	go runFunc()

	_, cronExpr, _ := s.scheduleSnapshot()
	entryID, err := s.cron.AddFunc(cronExpr, runFunc)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.runFunc = runFunc
	s.cronEntryID = entryID
	s.mu.Unlock()
	return nil
}

func (s *transService) ApplyRuntimeSettings(next config.RuntimeSettings) error {
	if err := next.Validate(); err != nil {
		return err
	}
	targetTag, err := language.Parse(next.TargetLanguage)
	if err != nil {
		return fmt.Errorf("invalid target_language: %w", err)
	}

	oldEntryID, oldCronExpr, runFunc := s.scheduleSnapshot()
	newEntryID := oldEntryID
	if runFunc != nil && next.CronExpr != oldCronExpr {
		entryID, err := s.cron.AddFunc(next.CronExpr, runFunc)
		if err != nil {
			return fmt.Errorf("failed to apply cron expression %q: %w", next.CronExpr, err)
		}
		if oldEntryID != 0 {
			s.cron.Remove(oldEntryID)
		}
		newEntryID = entryID
	}

	s.mu.Lock()
	s.cfg.LLM.APIURL = next.LLMAPIURL
	s.cfg.LLM.APIKey = next.LLMAPIKey
	s.cfg.LLM.Model = next.LLMModel
	s.cfg.Translate.CronExpr = next.CronExpr
	s.cfg.Translate.TargetLanguage = targetTag
	s.cronExpr = next.CronExpr
	s.cronEntryID = newEntryID
	s.mu.Unlock()
	return nil
}

func (s *transService) run(
	ctx context.Context,
	dir string,
) error {
	s.cleanupExpiredCaches(ctx)

	toTrans, err := s.findTargetMediaTuplesInDir(ctx, dir)
	if err != nil {
		log.Error("Failed to find target media tuples in dir %s: %v", dir, err)
		return err
	}
	log.Info("Found %d target media tuples in dir %s", len(toTrans), dir)

	if s.jobQueue != nil {
		for _, bundle := range toTrans {
			if err := s.enqueueCronMediaBundle(bundle); err != nil {
				log.Error("Failed to enqueue cron bundle for media %s: %v", bundle.MediaFile, err)
			}
		}
		return nil
	}

	llmAgent, searchEnabled, err := s.buildAgent()
	if err != nil {
		return err
	}

	cfg := s.configSnapshot()
	bundleConcurrency := max(1, cfg.Agent.BundleConcurrency)
	if bundleConcurrency == 1 {
		for _, bundle := range toTrans {
			if err := s.processBundle(ctx, bundle, "", llmAgent, searchEnabled); err != nil {
				return err
			}
		}
		return nil
	}

	log.Info("Processing bundles with concurrency=%d", bundleConcurrency)
	group, groupCtx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, bundleConcurrency)

	for _, bundle := range toTrans {
		bundle := bundle
		group.Go(func() error {
			select {
			case sem <- struct{}{}:
			case <-groupCtx.Done():
				return groupCtx.Err()
			}
			defer func() { <-sem }()
			return s.processBundle(groupCtx, bundle, "", llmAgent, searchEnabled)
		})
	}

	return group.Wait()
}

func (s *transService) buildAgent() (*agent.LLMAgent, bool, error) {
	cfg := s.configSnapshot()
	llmConfig := agent.LLMConfig{
		APIKey:      cfg.LLM.APIKey,
		APIURL:      cfg.LLM.APIURL,
		Model:       cfg.LLM.Model,
		MaxTokens:   cfg.LLM.MaxTokens,
		Temperature: cfg.LLM.Temperature,
		Timeout:     cfg.LLM.Timeout,
	}

	registry := tools.NewRegistry()
	searchEnabled := false

	if cfg.Search.APIKey != "" {
		webSearchTool := tools.NewWebSearchTool(cfg.Search.APIKey, cfg.Search.APIURL)
		if err := registry.Register(webSearchTool); err != nil {
			log.Error("Failed to register web_search tool: %v", err)
		} else {
			searchEnabled = true
			log.Info("Web search tool enabled")
		}
	}

	llmAgent, err := agent.NewLLMAgent(llmConfig, registry, cfg.Agent.MaxIterations)
	if err != nil {
		log.Error("Failed to create agent-core-go agent: %v", err)
		return nil, false, err
	}
	return llmAgent, searchEnabled, nil
}

func (s *transService) enqueueCronMediaBundle(bundle MediaBundle) error {
	if len(bundle.SubtitleFiles) == 0 {
		log.Info("Skipping media %s: no subtitle files available", bundle.MediaFile)
		return nil
	}

	pathBundle := MediaPathBundle{
		MediaFile:     bundle.MediaFile,
		SubtitleFiles: []string{bundle.SubtitleFiles[0].Path},
	}
	if len(bundle.NFOFiles) > 0 {
		pathBundle.NFOFiles = []string{bundle.NFOFiles[0].Path}
	}

	job, created, err := s.enqueueCronBundle(pathBundle)
	if err != nil {
		return err
	}
	if created {
		log.Info("Queued cron job %s for media %s", job.ID, bundle.MediaFile)
	} else {
		log.Info("Skipped duplicated cron job %s for media %s", job.ID, bundle.MediaFile)
	}
	return nil
}

func (s *transService) processJob(
	ctx context.Context,
	job *jobs.TranslationJob,
	llmAgent *agent.LLMAgent,
	searchEnabled bool,
) error {
	if job == nil {
		return fmt.Errorf("job is nil")
	}
	if job.Payload.MediaFile == "" {
		return fmt.Errorf("media_file is required")
	}

	subFile, err := s.loadSubtitleForJob(ctx, job)
	if err != nil {
		return err
	}

	bundle := MediaBundle{
		MediaFile:     job.Payload.MediaFile,
		SubtitleFiles: []subtitle.File{*subFile},
	}
	if job.Payload.NFOFile != "" {
		nfoInfo, err := NewNFOReader().ReadTVShowInfo(job.Payload.NFOFile)
		if err != nil {
			log.Error("Failed to read NFO file %s: %v", job.Payload.NFOFile, err)
		} else {
			bundle.NFOFiles = []media.TVShowInfo{*nfoInfo}
		}
	}

	return s.processBundle(ctx, bundle, job.ID, llmAgent, searchEnabled)
}

func (s *transService) processBundle(
	ctx context.Context,
	bundle MediaBundle,
	jobID string,
	llmAgent *agent.LLMAgent,
	searchEnabled bool,
) error {
	if len(bundle.SubtitleFiles) == 0 {
		log.Info("Skipping media %s: no subtitle files available", bundle.MediaFile)
		return nil
	}
	translateCtx := ctx
	if s.store != nil && jobID != "" {
		checkpointStore, err := newPersistentBatchCheckpointStore(translateCtx, s.store, jobID)
		if err != nil {
			log.Error("Failed to load checkpoints for job %s: %v", jobID, err)
		} else {
			translateCtx = withBatchCheckpointStore(translateCtx, checkpointStore)
		}
	}
	targetSub := bundle.SubtitleFiles[0]
	agentTranslator := translator.NewAgentTranslator(llmAgent, searchEnabled)
	cfg := s.configSnapshot()

	var termMapData map[string]string
	srcLang := targetSub.Language.String()
	tgtLang := cfg.Translate.TargetLanguage.String()
	mediaDir := filepath.Dir(bundle.MediaFile)

	tmPath := termmap.FindInAncestors(mediaDir, srcLang, tgtLang)
	if tmPath != "" {
		tm, err := termmap.Load(tmPath)
		if err != nil {
			log.Error("Failed to load term map from %s: %v", tmPath, err)
		} else {
			termMapData = map[string]string(tm)
			log.Info("Loaded term map from %s (%d terms)", tmPath, len(tm))
		}
	} else if searchEnabled && len(bundle.NFOFiles) > 0 {
		gen := termmap.NewGenerator(llmAgent)
		tm, err := gen.Generate(ctx, bundle.NFOFiles[0], srcLang, tgtLang)
		if err != nil {
			log.Error("Failed to generate term map: %v", err)
		} else {
			saveDir := findTermMapSaveDir(bundle.NFOFiles, mediaDir)
			savePath := termmap.FilePath(saveDir, srcLang, tgtLang)
			merged, err := saveMergedTermMap(savePath, termMapData, tm)
			if err != nil {
				log.Error("Failed to save term map to %s: %v", savePath, err)
			} else {
				termMapData = merged
				log.Info("Generated and saved term map to %s (%d terms)", savePath, len(tm))
			}
		}
	}

	log.Info("Translating subtitle media %s from %s to %s", bundle.MediaFile, targetSub.Language, cfg.Translate.TargetLanguage)
	transLator, err := NewTranslator(
		TranslatorConfig{
			TargetLanguage: cfg.Translate.TargetLanguage,
			ContextEnabled: true,
			SubtitleFile:   &targetSub,
			OutputDir:      filepath.Dir(bundle.MediaFile),
			InputPath:      targetSub.Path,
			TermMap:        termMapData,
		},
		agentTranslator,
	)
	if err != nil {
		log.Error("Failed to create translator: %v", err)
		return err
	}

	nfoPath := ""
	if len(bundle.NFOFiles) > 0 {
		nfoPath = bundle.NFOFiles[0].Path
	}
	if _, err := transLator.Translate(translateCtx, nfoPath); err != nil {
		log.Error("Failed to translate subtitle media %s: %v", bundle.MediaFile, err)
		return err
	}
	log.Info("Translated subtitle media %s", bundle.MediaFile)
	if s.store != nil && jobID != "" {
		if err := s.store.ClearJobTemp(ctx, jobID); err != nil {
			log.Warn("Failed to clear temporary data for job %s: %v", jobID, err)
		}
	}

	if discoverer, ok := agentTranslator.(translator.TermDiscoverer); ok {
		toolCalls := discoverer.CollectedToolCalls()
		discoverer.ResetCollectedToolCalls()

		if len(toolCalls) > 0 && searchEnabled && len(bundle.NFOFiles) > 0 {
			gen := termmap.NewGenerator(llmAgent)
			newTerms, err := gen.ExtractNewTerms(ctx, toolCalls, termmap.TermMap(termMapData), bundle.NFOFiles[0].Title, srcLang, tgtLang)
			if err != nil {
				log.Error("Failed to extract new terms from tool calls: %v", err)
			} else if len(newTerms) > 0 {
				saveDir := findTermMapSaveDir(bundle.NFOFiles, mediaDir)
				savePath := termmap.FilePath(saveDir, srcLang, tgtLang)

				merged, err := saveMergedTermMap(savePath, termMapData, newTerms)
				if err != nil {
					log.Error("Failed to save updated term map to %s: %v", savePath, err)
				} else {
					termMapData = merged
					log.Info("Updated term map with %d new terms at %s", len(newTerms), savePath)
				}
			}
		}
	}

	return nil
}

func (s *transService) loadSubtitleForJob(ctx context.Context, job *jobs.TranslationJob) (*subtitle.File, error) {
	if job == nil {
		return nil, fmt.Errorf("job is nil")
	}

	subtitlePath := job.Payload.SubtitleFile
	if subtitlePath != "" {
		subFile, err := subtitle.NewReader(subtitlePath).Read()
		if err != nil {
			return nil, fmt.Errorf("failed to read subtitle file %s: %w", subtitlePath, err)
		}
		return subFile, nil
	}

	cacheKey := subtitleCacheKey(job.Payload.MediaFile)
	if s.store != nil {
		cached, ok, err := s.store.GetSubtitleCache(ctx, cacheKey)
		if err != nil {
			log.Error("Failed to load subtitle cache %s: %v", cacheKey, err)
		} else if ok {
			return &cached, nil
		}
	}

	operator := media.NewOperator(job.Payload.MediaFile)
	payload, err := operator.ExtractSubtitleToBytes()
	if err != nil {
		// Fallback to file extraction for environments where stdout extraction is unavailable.
		extracted, fileErr := operator.DefExtractSubtitle()
		if fileErr != nil {
			return nil, fmt.Errorf("failed to extract subtitle from media file %s: %w", job.Payload.MediaFile, err)
		}
		subFile, readErr := subtitle.NewReader(extracted).Read()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read subtitle file %s: %w", extracted, readErr)
		}
		if s.store != nil {
			if cacheErr := s.store.PutSubtitleCache(ctx, persistence.SubtitleCacheEntry{
				CacheKey:  cacheKey,
				MediaPath: job.Payload.MediaFile,
				JobID:     job.ID,
				File:      *subFile,
				IsTemp:    true,
			}); cacheErr != nil {
				log.Error("Failed to save subtitle cache %s: %v", cacheKey, cacheErr)
			}
		}
		return subFile, nil
	}
	subFile, err := subtitle.ReadSRTBytes(payload, syntheticSubtitlePath(job.Payload.MediaFile))
	if err != nil {
		return nil, fmt.Errorf("failed to parse extracted subtitle of media %s: %w", job.Payload.MediaFile, err)
	}
	if s.store != nil {
		if err := s.store.PutSubtitleCache(ctx, persistence.SubtitleCacheEntry{
			CacheKey:  cacheKey,
			MediaPath: job.Payload.MediaFile,
			JobID:     job.ID,
			File:      *subFile,
			IsTemp:    true,
		}); err != nil {
			log.Error("Failed to save subtitle cache %s: %v", cacheKey, err)
		}
	}
	return subFile, nil
}

func subtitleCacheKey(mediaPath string) string {
	return mediaPath + "|s:0"
}

func syntheticSubtitlePath(mediaPath string) string {
	ext := filepath.Ext(mediaPath)
	stem := strings.TrimSuffix(filepath.Base(mediaPath), ext)
	return filepath.Join(filepath.Dir(mediaPath), stem+"_ctxtrans_embedded.srt")
}

func (s *transService) findTargetMediaTuplesInDir(
	ctx context.Context,
	dir string,
) (ret []MediaBundle, err error) {
	cfg := s.configSnapshot()
	all, err := s.findSourceBundlesInDir(ctx, dir)
	if err != nil {
		return
	}

	ret = make([]MediaBundle, 0, len(all))
	for _, bundle := range all {
		mediaPath := bundle.MediaFile
		now := time.Now().UTC()
		var cachedMeta persistence.MediaMetaCache
		cachedHit := false
		if s.store != nil && mediaPath != "" {
			meta, ok, err := s.store.GetMediaMetaCache(ctx, mediaPath, cfg.Translate.TargetLanguage.String(), now)
			if err != nil {
				log.Error("Failed to load media metadata cache for %s: %v", mediaPath, err)
			} else if ok {
				cachedMeta = meta
				cachedHit = true
			}
		}
		if cachedHit && (cachedMeta.HasTargetExternal || cachedMeta.HasTargetEmbedded) {
			continue
		}

		subtitles, err := s.readSubtitleFiles(ctx, bundle.SubtitleFiles)
		if err != nil {
			log.Error("Failed to read subtitle files of media file %s: %v", bundle.MediaFile, err)
			continue
		}

		// If target subtitle exists, skip
		if containTargetSubtitle(subtitles, cfg.Translate.TargetLanguage) {
			continue
		}

		mediaReader := media.NewOperator(bundle.MediaFile)
		var subDescs subtitle.Descriptions
		if cachedHit {
			subDescs = descriptionsFromLanguageCodes(cachedMeta.EmbeddedLanguages)
		} else {
			subDescs, err = mediaReader.ReadSubtitleDescription()
			if err != nil {
				log.Error("Failed to read subtitle description of media file %s: %v", bundle.MediaFile, err)
				// Keep processing with external subtitle signals even if ffprobe is unavailable.
				subDescs = nil
			}
			if s.store != nil && mediaPath != "" {
				if err := s.store.PutMediaMetaCache(ctx, persistence.MediaMetaCache{
					MediaPath:         mediaPath,
					TargetLanguage:    cfg.Translate.TargetLanguage.String(),
					ExternalLanguages: subtitleLanguages(subtitles),
					EmbeddedLanguages: descriptionLanguages(subDescs),
					HasTargetExternal: containTargetSubtitle(subtitles, cfg.Translate.TargetLanguage),
					HasTargetEmbedded: subDescs.HasLanguage(cfg.Translate.TargetLanguage),
					ExpiresAt:         now.Add(10 * time.Minute),
					UpdatedAt:         now,
				}); err != nil {
					log.Error("Failed to save media metadata cache for %s: %v", mediaPath, err)
				}
			}
		}
		if subDescs.HasLanguage(cfg.Translate.TargetLanguage) {
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
		if languageMatches(sub.Language, targetLanguage) {
			return true
		}
		if subtitlePathMatchesLanguage(sub.Path, targetLanguage) {
			return true
		}
	}
	return false
}

func languageMatches(a, b language.Tag) bool {
	if a == b {
		return true
	}
	ab, _ := a.Base()
	bb, _ := b.Base()
	return ab.String() != "und" && ab == bb
}

func subtitlePathMatchesLanguage(path string, target language.Tag) bool {
	if path == "" {
		return false
	}

	fileName := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(fileName))
	if !isSubtitleFile(ext) {
		return false
	}

	base := strings.TrimSuffix(fileName, ext)
	idx := strings.LastIndex(base, ".")
	if idx < 0 || idx == len(base)-1 {
		return false
	}
	suffix := strings.ReplaceAll(base[idx+1:], "_", "-")
	targetStr := strings.ToLower(strings.ReplaceAll(target.String(), "_", "-"))
	targetBase, _ := target.Base()
	targetBaseStr := strings.ToLower(targetBase.String())

	return suffix == targetStr || suffix == targetBaseStr || strings.HasPrefix(suffix, targetBaseStr+"-")
}

func (s *transService) readSubtitleFiles(
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

func (s *transService) findSourceBundlesInDir(
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

	// Step 1: Find all target files (subtitles or media files)
	var targetFiles []string

	// Walk all files and keep media/subtitle candidates.
	err = filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if isSubtitleFile(ext) || isMediaFile(ext) {
			targetFiles = append(targetFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan files: %w", err)
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

		// Find NFO files in current or parent directories and prefer episode-level NFO.
		nfoFiles := findNFOFiles(baseDir)
		if episodeNFO := findEpisodeNFOFile(baseDir, baseName); episodeNFO != "" {
			nfoFiles = append([]string{episodeNFO}, nfoFiles...)
		}
		bundle.NFOFiles = dedupePaths(nfoFiles)

		// Add bundle if it has at least a subtitle or media file
		if len(bundle.SubtitleFiles) > 0 || bundle.MediaFile != "" {
			releaseDate, hasReleaseDate := releaseDateFromNFOFiles(bundle.NFOFiles)
			if !hasReleaseDate {
				log.Info("Skip bundle %s: release date is unavailable", baseName)
				continue
			}
			if releaseDate.Before(startTime) {
				continue
			}
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
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext == "" {
		return fileName
	}
	base := strings.TrimSuffix(fileName, ext)

	// For subtitle files, strip a trailing language suffix (e.g. ".eng", ".zh-cn")
	// so "episode.eng.srt" and "episode.zh-cn.srt" map to the same media base.
	if isSubtitleFile(ext) {
		if idx := strings.LastIndex(base, "."); idx > 0 {
			langSuffix := strings.ReplaceAll(base[idx+1:], "_", "-")
			if looksLikeLanguageSuffix(langSuffix) {
				return base[:idx]
			}
		}
	}
	return base
}

func looksLikeLanguageSuffix(s string) bool {
	if s == "" {
		return false
	}

	parts := strings.Split(strings.ToLower(s), "-")
	if len(parts) == 0 || len(parts) > 3 {
		return false
	}

	for i, part := range parts {
		if part == "" {
			return false
		}
		// language: en, eng; region/script fragments: cn, tw, hans...
		if i == 0 && (len(part) < 2 || len(part) > 3) {
			return false
		}
		if i > 0 && (len(part) < 2 || len(part) > 4) {
			return false
		}
		for _, r := range part {
			if r < 'a' || r > 'z' {
				return false
			}
		}
	}
	return true
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

func findEpisodeNFOFile(dir, baseName string) string {
	candidate := filepath.Join(dir, baseName+".nfo")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

func dedupePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(paths))
	ret := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		ret = append(ret, p)
	}
	return ret
}

func releaseDateFromNFOFiles(nfoFiles []string) (time.Time, bool) {
	reader := NewNFOReader()
	var latest time.Time
	found := false

	for _, nfoPath := range nfoFiles {
		info, err := reader.ReadTVShowInfo(nfoPath)
		if err != nil {
			continue
		}

		candidates := []string{info.Aired, info.Premiered}
		if info.Year > 0 {
			candidates = append(candidates, fmt.Sprintf("%04d", info.Year))
		}

		for _, candidate := range candidates {
			parsed, ok := parseNFODate(candidate)
			if !ok {
				continue
			}
			if !found || parsed.After(latest) {
				latest = parsed
				found = true
			}
		}
	}

	return latest, found
}

func parseNFODate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}

	layouts := []string{
		"2006-01-02",
		"2006/01/02",
		"2006.01.02",
		"2006-01",
		"2006",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, raw)
		if err != nil {
			continue
		}
		return parsed.UTC(), true
	}
	return time.Time{}, false
}

func subtitleLanguages(subtitles []subtitle.File) []string {
	if len(subtitles) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(subtitles))
	ret := make([]string, 0, len(subtitles))
	for _, sub := range subtitles {
		lang := strings.TrimSpace(sub.Language.String())
		if lang == "" || lang == "und" || seen[lang] {
			continue
		}
		seen[lang] = true
		ret = append(ret, lang)
	}
	return ret
}

func descriptionLanguages(descs subtitle.Descriptions) []string {
	if len(descs) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(descs))
	ret := make([]string, 0, len(descs))
	for _, desc := range descs {
		code := strings.TrimSpace(desc.LangTag.String())
		if code == "" || code == "und" {
			code = strings.TrimSpace(desc.Language)
		}
		if code == "" || code == "und" || seen[code] {
			continue
		}
		seen[code] = true
		ret = append(ret, code)
	}
	return ret
}

func descriptionsFromLanguageCodes(langs []string) subtitle.Descriptions {
	if len(langs) == 0 {
		return nil
	}
	ret := make(subtitle.Descriptions, 0, len(langs))
	for _, langCode := range langs {
		langCode = strings.TrimSpace(langCode)
		if langCode == "" {
			continue
		}
		ret = append(ret, subtitle.Description{
			Language: langCode,
			LangTag:  language.All.Make(langCode),
		})
	}
	return ret
}

// isSubtitleFile checks if the file extension is a subtitle format
func isSubtitleFile(ext string) bool {
	return slices.Contains(subtitleExts, ext)
}

// isMediaFile checks if the file extension is a media format that supports embedded subtitles
func isMediaFile(ext string) bool {
	return slices.Contains(mediaExts, ext)
}

// findTermMapSaveDir finds the best directory to save a term map.
// Prefers the directory containing tvshow.nfo for show-level coverage,
// falling back to the first NFO's directory or the given fallback.
func findTermMapSaveDir(nfoFiles []media.TVShowInfo, fallbackDir string) string {
	for _, nfo := range nfoFiles {
		if filepath.Base(nfo.Path) == "tvshow.nfo" {
			return filepath.Dir(nfo.Path)
		}
	}
	if len(nfoFiles) > 0 && nfoFiles[0].Path != "" {
		return filepath.Dir(nfoFiles[0].Path)
	}
	return fallbackDir
}

func saveMergedTermMap(savePath string, existing map[string]string, newTerms termmap.TermMap) (map[string]string, error) {
	merged := make(map[string]string, len(existing)+len(newTerms))
	for key, value := range existing {
		merged[key] = value
	}
	for key, value := range newTerms {
		merged[key] = value
	}

	if err := withTermMapFileLock(savePath, func() error {
		return termmap.Save(savePath, termmap.TermMap(merged))
	}); err != nil {
		return nil, err
	}

	return merged, nil
}

func withTermMapFileLock(path string, fn func() error) error {
	muAny, _ := termMapFileLocks.LoadOrStore(path, &sync.Mutex{})
	mu := muAny.(*sync.Mutex)

	mu.Lock()
	defer mu.Unlock()

	return fn()
}

func (s *transService) startTime() (time.Time, error) {
	s.mu.RLock()
	lastTriggerTime := s.lastTrigerTime
	cronExpr := s.cronExpr
	s.mu.RUnlock()

	if lastTriggerTime.IsZero() {
		now := time.Now()
		cronSchedule, err := icron.GetTriggerInfo(cronExpr, now)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to get cron schedule: %w", err)
		}

		windowStart := now.Add(-24 * 14 * time.Hour)
		if windowStart.After(cronSchedule.Last) {
			return windowStart, nil
		}
		return cronSchedule.Last, nil
	}

	return lastTriggerTime, nil
}

func (s *transService) cleanupExpiredCaches(ctx context.Context) {
	if s.store == nil {
		return
	}
	n, err := s.store.DeleteExpiredMediaMetaCache(ctx, time.Now().UTC())
	if err != nil {
		log.Error("Failed to cleanup expired media meta cache: %v", err)
		return
	}
	if n > 0 {
		log.Info("Cleaned up %d expired media meta cache entries", n)
	}
}
