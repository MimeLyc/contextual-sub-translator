package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/MimeLyc/contextual-sub-translator/internal/jobs"
	"github.com/MimeLyc/contextual-sub-translator/internal/library"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"golang.org/x/text/language"
)

const (
	defaultJobPreviewLimit = 80
	maxJobPreviewLimit     = 500
)

var (
	errJobNotFound     = errors.New("job not found")
	errJobInProgress   = errors.New("job is running")
	errJobNotCompleted = errors.New("job is not completed")
	errInvalidLine     = errors.New("line index out of range")

	jobTargetLanguagePattern = regexp.MustCompile(`^[A-Za-z]{2,3}(?:[-_][A-Za-z0-9]{2,8})*$`)
)

type jobDetailResponse struct {
	Job            *jobs.TranslationJob `json:"job"`
	TargetLanguage string               `json:"target_language"`
	Progress       jobProgressResponse  `json:"progress"`
	Episode        jobEpisodeInfo       `json:"episode"`
	Preview        []jobPreviewLine     `json:"preview"`
	PreviewOffset  int                  `json:"preview_offset"`
	PreviewLimit   int                  `json:"preview_limit"`
	Editable       bool                 `json:"editable"`
}

type jobProgressResponse struct {
	TranslatedLines int     `json:"translated_lines"`
	TotalLines      int     `json:"total_lines"`
	Percent         float64 `json:"percent"`
}

type jobEpisodeInfo struct {
	SourceID           string `json:"source_id"`
	ItemID             string `json:"item_id"`
	SeriesName         string `json:"series_name"`
	Season             string `json:"season"`
	EpisodeName        string `json:"episode_name"`
	MediaPath          string `json:"media_path"`
	SubtitlePath       string `json:"subtitle_path"`
	OutputSubtitlePath string `json:"output_subtitle_path"`
}

type jobPreviewLine struct {
	Index          int    `json:"index"`
	OriginalText   string `json:"original_text"`
	TranslatedText string `json:"translated_text"`
}

type updateJobLinesRequest struct {
	Lines []updateJobLineRequest `json:"lines"`
}

type updateJobLineRequest struct {
	Index          int    `json:"index"`
	TranslatedText string `json:"translated_text"`
}

type jobSnapshot struct {
	Job             *jobs.TranslationJob
	TargetLanguage  string
	OutputPath      string
	SourceLines     []subtitle.Line
	OutputLines     []subtitle.Line
	TranslatedByIdx map[int]string
	TotalLines      int
}

func (s *Server) handleJobDetailRoutes(w http.ResponseWriter, r *http.Request) {
	jobID, action, ok := parseJobRoute(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	switch action {
	case "":
		s.handleJobDetail(w, r, jobID)
	case "lines":
		s.handleUpdateJobLines(w, r, jobID)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func parseJobRoute(path string) (jobID string, action string, ok bool) {
	trimmed := strings.TrimPrefix(path, "/api/jobs/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) > 2 {
		return "", "", false
	}
	rawID, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(rawID) == "" {
		return "", "", false
	}
	if len(parts) == 1 {
		return rawID, "", true
	}
	return rawID, parts[1], true
}

func (s *Server) handleJobDetail(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	offset := parsePositiveIntWithDefault(r.URL.Query().Get("offset"), 0)
	limit := parsePositiveIntWithDefault(r.URL.Query().Get("limit"), defaultJobPreviewLimit)
	if limit <= 0 {
		limit = defaultJobPreviewLimit
	}
	if limit > maxJobPreviewLimit {
		limit = maxJobPreviewLimit
	}

	detail, err := s.buildJobDetail(r.Context(), jobID, offset, limit)
	if err != nil {
		switch {
		case errors.Is(err, errJobNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleUpdateJobLines(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req updateJobLinesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if len(req.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "lines is required")
		return
	}

	detail, err := s.updateJobLines(r.Context(), jobID, req.Lines)
	if err != nil {
		switch {
		case errors.Is(err, errJobNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, errJobInProgress):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, errJobNotCompleted), errors.Is(err, errInvalidLine):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func parsePositiveIntWithDefault(raw string, def int) int {
	if strings.TrimSpace(raw) == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return def
	}
	return v
}

func (s *Server) buildJobDetail(ctx context.Context, jobID string, offset int, limit int) (jobDetailResponse, error) {
	job, ok := s.queue.Get(jobID)
	if !ok {
		return jobDetailResponse{}, errJobNotFound
	}

	snapshot, err := s.buildSnapshot(ctx, job)
	if err != nil {
		return jobDetailResponse{}, err
	}

	progress := computeJobProgress(snapshot.TotalLines, snapshot.TranslatedByIdx)
	detail := jobDetailResponse{
		Job:            snapshot.Job,
		TargetLanguage: snapshot.TargetLanguage,
		Progress:       progress,
		Episode:        s.resolveJobEpisodeInfo(ctx, snapshot.Job, snapshot.OutputPath),
		Preview:        buildPreviewLines(snapshot.SourceLines, snapshot.TranslatedByIdx, offset, limit, snapshot.TotalLines),
		PreviewOffset:  offset,
		PreviewLimit:   limit,
		Editable:       snapshot.Job.Status == jobs.StatusSuccess,
	}
	return detail, nil
}

func (s *Server) updateJobLines(ctx context.Context, jobID string, patches []updateJobLineRequest) (jobDetailResponse, error) {
	job, ok := s.queue.Get(jobID)
	if !ok {
		return jobDetailResponse{}, errJobNotFound
	}
	if job.Status == jobs.StatusPending || job.Status == jobs.StatusRunning {
		return jobDetailResponse{}, errJobInProgress
	}
	if job.Status != jobs.StatusSuccess {
		return jobDetailResponse{}, errJobNotCompleted
	}

	snapshot, err := s.buildSnapshot(ctx, job)
	if err != nil {
		return jobDetailResponse{}, err
	}
	if snapshot.TotalLines <= 0 {
		return jobDetailResponse{}, fmt.Errorf("no subtitle lines found for job %s", jobID)
	}

	writable := makeWritableLines(snapshot.SourceLines, snapshot.OutputLines, snapshot.TranslatedByIdx, snapshot.TotalLines)
	for _, patch := range patches {
		if patch.Index <= 0 || patch.Index > len(writable) {
			return jobDetailResponse{}, errInvalidLine
		}
		writable[patch.Index-1].TranslatedText = patch.TranslatedText
	}

	langTag, err := language.Parse(snapshot.TargetLanguage)
	if err != nil {
		langTag = language.Und
	}
	if err := subtitle.NewWriter().Write(snapshot.OutputPath, &subtitle.File{
		Lines:    writable,
		Language: langTag,
		Format:   "SRT",
		Path:     snapshot.OutputPath,
	}); err != nil {
		return jobDetailResponse{}, err
	}

	return s.buildJobDetail(ctx, jobID, 0, defaultJobPreviewLimit)
}

func (s *Server) buildSnapshot(ctx context.Context, job *jobs.TranslationJob) (jobSnapshot, error) {
	if job == nil {
		return jobSnapshot{}, errJobNotFound
	}

	targetLanguage := detectJobTargetLanguage(job, s.scanner.TargetLanguage())
	outputPath := buildOutputSubtitlePath(job, targetLanguage)

	sourceLines, err := s.readSourceLinesForJob(ctx, job)
	if err != nil {
		return jobSnapshot{}, err
	}
	outputLines, err := readSubtitleLinesIfFileExists(outputPath)
	if err != nil {
		return jobSnapshot{}, err
	}
	translations, err := s.loadCheckpointTranslations(ctx, job.ID)
	if err != nil {
		return jobSnapshot{}, err
	}

	for i, line := range outputLines {
		text := strings.TrimSpace(line.Text)
		if text == "" {
			continue
		}
		translations[i+1] = line.Text
	}

	totalLines := max(len(sourceLines), len(outputLines))
	for idx := range translations {
		totalLines = max(totalLines, idx)
	}

	return jobSnapshot{
		Job:             job,
		TargetLanguage:  targetLanguage,
		OutputPath:      outputPath,
		SourceLines:     sourceLines,
		OutputLines:     outputLines,
		TranslatedByIdx: translations,
		TotalLines:      totalLines,
	}, nil
}

func computeJobProgress(total int, translated map[int]string) jobProgressResponse {
	if total <= 0 {
		return jobProgressResponse{
			TranslatedLines: 0,
			TotalLines:      0,
			Percent:         0,
		}
	}
	done := 0
	for i := 1; i <= total; i++ {
		if strings.TrimSpace(translated[i]) != "" {
			done++
		}
	}
	return jobProgressResponse{
		TranslatedLines: done,
		TotalLines:      total,
		Percent:         (float64(done) / float64(total)) * 100,
	}
}

func buildPreviewLines(source []subtitle.Line, translated map[int]string, offset int, limit int, total int) []jobPreviewLine {
	if total <= 0 || offset >= total {
		return []jobPreviewLine{}
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = defaultJobPreviewLimit
	}

	end := min(total, offset+limit)
	ret := make([]jobPreviewLine, 0, end-offset)
	for i := offset; i < end; i++ {
		idx := i + 1
		original := ""
		if i < len(source) {
			original = source[i].Text
		}
		ret = append(ret, jobPreviewLine{
			Index:          idx,
			OriginalText:   original,
			TranslatedText: translated[idx],
		})
	}
	return ret
}

func makeWritableLines(source []subtitle.Line, output []subtitle.Line, translated map[int]string, total int) []subtitle.Line {
	ret := make([]subtitle.Line, total)
	for i := 0; i < total; i++ {
		idx := i + 1
		var base subtitle.Line
		switch {
		case i < len(source):
			base = source[i]
		case i < len(output):
			base = output[i]
		default:
			base = subtitle.Line{
				Index: idx,
			}
		}
		if base.Index == 0 {
			base.Index = idx
		}
		if text, ok := translated[idx]; ok {
			base.TranslatedText = text
		}
		ret[i] = base
	}
	return ret
}

func (s *Server) readSourceLinesForJob(ctx context.Context, job *jobs.TranslationJob) ([]subtitle.Line, error) {
	if job == nil {
		return nil, errJobNotFound
	}
	if lines, ok, err := readSubtitleLinesByPath(job.Payload.SubtitleFile); err != nil {
		return nil, err
	} else if ok {
		return lines, nil
	}
	if s.jobData == nil || strings.TrimSpace(job.Payload.MediaFile) == "" {
		return nil, nil
	}
	cacheKey := subtitleCacheKey(job.Payload.MediaFile)
	cached, ok, err := s.jobData.GetSubtitleCache(ctx, cacheKey)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return cached.Lines, nil
}

func readSubtitleLinesIfFileExists(path string) ([]subtitle.Line, error) {
	lines, ok, err := readSubtitleLinesByPath(path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return lines, nil
}

func readSubtitleLinesByPath(path string) ([]subtitle.Line, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false, nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	file, err := subtitle.NewReader(path).Read()
	if err != nil {
		return nil, false, err
	}
	return file.Lines, true, nil
}

func (s *Server) loadCheckpointTranslations(ctx context.Context, jobID string) (map[int]string, error) {
	ret := make(map[int]string)
	if s.jobData == nil {
		return ret, nil
	}
	checkpoints, err := s.jobData.LoadBatchCheckpoints(ctx, jobID)
	if err != nil {
		return nil, err
	}
	for _, cp := range checkpoints {
		for i, text := range cp.TranslatedLines {
			idx := cp.BatchStart + i + 1
			if idx <= 0 {
				continue
			}
			if text == "" {
				continue
			}
			ret[idx] = text
		}
	}
	return ret, nil
}

func detectJobTargetLanguage(job *jobs.TranslationJob, fallback string) string {
	if job != nil {
		parts := strings.Split(job.DedupeKey, "|")
		if len(parts) > 0 {
			candidate := strings.TrimSpace(parts[len(parts)-1])
			if normalized, ok := normalizeTargetLanguage(candidate); ok {
				return normalized
			}
		}
	}
	if normalized, ok := normalizeTargetLanguage(fallback); ok {
		return normalized
	}
	return "zh"
}

func normalizeTargetLanguage(raw string) (string, bool) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return "", false
	}
	if strings.ContainsAny(candidate, `/\.[\]`) {
		return "", false
	}
	candidate = strings.ReplaceAll(candidate, "_", "-")
	if !jobTargetLanguagePattern.MatchString(candidate) {
		return "", false
	}
	tag, err := language.Parse(candidate)
	if err != nil || tag == language.Und {
		return "", false
	}
	return tag.String(), true
}

func buildOutputSubtitlePath(job *jobs.TranslationJob, targetLanguage string) string {
	if job == nil {
		return ""
	}
	mediaPath := strings.TrimSpace(job.Payload.MediaFile)
	subPath := strings.TrimSpace(job.Payload.SubtitleFile)
	if subPath == "" {
		subPath = syntheticEmbeddedSubtitlePath(mediaPath)
	}
	base := filepath.Base(subPath)
	ext := filepath.Ext(base)
	if ext == "" {
		ext = ".srt"
	}
	stem := strings.TrimSuffix(base, ext)
	if idx := strings.Index(stem, "_ctxtrans"); idx >= 0 {
		stem = stem[:idx]
	}
	if stem == "" {
		mediaBase := filepath.Base(mediaPath)
		mediaExt := filepath.Ext(mediaBase)
		stem = strings.TrimSuffix(mediaBase, mediaExt)
	}
	if targetLanguage == "" {
		targetLanguage = "zh"
	}
	outputName := stem + "_ctxtrans." + targetLanguage + ext
	return filepath.Join(filepath.Dir(mediaPath), outputName)
}

func syntheticEmbeddedSubtitlePath(mediaPath string) string {
	ext := filepath.Ext(mediaPath)
	stem := strings.TrimSuffix(filepath.Base(mediaPath), ext)
	return filepath.Join(filepath.Dir(mediaPath), stem+"_ctxtrans_embedded.srt")
}

func subtitleCacheKey(mediaPath string) string {
	return mediaPath + "|s:0"
}

func (s *Server) resolveJobEpisodeInfo(ctx context.Context, job *jobs.TranslationJob, outputPath string) jobEpisodeInfo {
	info := jobEpisodeInfo{
		MediaPath:          job.Payload.MediaFile,
		SubtitlePath:       job.Payload.SubtitleFile,
		OutputSubtitlePath: outputPath,
	}
	episode, itemName, ok := s.findEpisodeByMediaPath(ctx, job.Payload.MediaFile)
	if ok {
		info.SourceID = episode.SourceID
		info.ItemID = episode.ItemID
		info.SeriesName = itemName
		info.Season = episode.Season
		info.EpisodeName = episode.Name
		return info
	}
	info.SeriesName = filepath.Base(filepath.Dir(job.Payload.MediaFile))
	base := filepath.Base(job.Payload.MediaFile)
	info.EpisodeName = strings.TrimSuffix(base, filepath.Ext(base))
	return info
}

func (s *Server) findEpisodeByMediaPath(ctx context.Context, mediaPath string) (library.Episode, string, bool) {
	if s.scanner == nil || strings.TrimSpace(mediaPath) == "" {
		return library.Episode{}, "", false
	}
	sources, err := s.scanner.ScanSources(ctx)
	if err != nil {
		return library.Episode{}, "", false
	}
	for _, source := range sources {
		items, err := s.scanner.ScanItems(ctx, source.ID)
		if err != nil {
			continue
		}
		for _, item := range items {
			episodes, err := s.scanner.ScanEpisodesByItem(ctx, item.ID)
			if err != nil {
				continue
			}
			for _, episode := range episodes {
				if episode.MediaPath == mediaPath {
					return episode, item.Name, true
				}
			}
		}
	}
	return library.Episode{}, "", false
}
