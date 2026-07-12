package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/MimeLyc/contextual-sub-translator/internal/config"
	"github.com/MimeLyc/contextual-sub-translator/internal/jobs"
	"github.com/MimeLyc/contextual-sub-translator/internal/library"
	"golang.org/x/sync/errgroup"
)

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sources, err := s.scanner.ScanSources(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sources)
}

func (s *Server) handleListItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sourceID := r.URL.Query().Get("source")
	if sourceID != "" {
		items, err := s.scanner.ScanItems(r.Context(), sourceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, items)
		return
	}

	// No source filter: iterate all sources
	sources, err := s.scanner.ScanSources(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	allItems := make([]library.Item, 0)
	for _, src := range sources {
		items, err := s.scanner.ScanItems(r.Context(), src.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		allItems = append(allItems, items...)
	}
	writeJSON(w, http.StatusOK, allItems)
}

func (s *Server) handleListItemStatuses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sourceID := strings.TrimSpace(r.URL.Query().Get("source"))
	if sourceID == "" {
		writeError(w, http.StatusBadRequest, "source is required")
		return
	}
	items, err := s.scanner.ScanItems(r.Context(), sourceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jobsByMedia := latestJobsByMedia(s.queue.List())
	ret := make([]itemStatusSummary, len(items))
	g, ctx := errgroup.WithContext(r.Context())
	// A single item scan already probes episodes concurrently. Keeping the outer
	// limit low prevents collection summaries from flooding a NAS with ffprobe.
	g.SetLimit(2)
	for i := range items {
		index := i
		item := items[i]
		g.Go(func() error {
			episodes, scanErr := s.scanner.ScanEpisodesByItem(ctx, item.ID)
			if scanErr != nil {
				return scanErr
			}
			ret[index] = summarizeItemStatuses(item.ID, episodes, jobsByMedia)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ret)
}

func (s *Server) handleGetItemStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	itemID := strings.TrimSpace(r.URL.Query().Get("item"))
	if itemID == "" {
		writeError(w, http.StatusBadRequest, "item is required")
		return
	}
	episodes, err := s.scanner.ScanEpisodesByItem(r.Context(), itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summarizeItemStatuses(itemID, episodes, latestJobsByMedia(s.queue.List())))
}

func (s *Server) handleListEpisodesByItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/cover") {
		s.handleItemCover(w, r)
		return
	}

	// /api/library/items/{id}/episodes
	path := strings.TrimPrefix(r.URL.Path, "/api/library/items/")
	if !strings.HasSuffix(path, "/episodes") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	itemID := strings.TrimSuffix(path, "/episodes")
	itemID = strings.TrimSuffix(itemID, "/")
	if decoded, err := url.PathUnescape(itemID); err == nil {
		itemID = decoded
	}
	if itemID == "" {
		writeError(w, http.StatusBadRequest, "missing item id")
		return
	}

	episodes, err := s.scanner.ScanEpisodesByItem(r.Context(), itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jobsByMedia := latestJobsByMedia(s.queue.List())
	ret := make([]episodeResponse, 0, len(episodes))
	for _, episode := range episodes {
		item := episodeResponse{
			Episode: episode,
		}
		if job, ok := jobsByMedia[episode.MediaPath]; ok {
			item.JobStatus = job.Status
			item.JobSource = job.Source
			item.InProgress = job.Status == jobs.StatusPending || job.Status == jobs.StatusRunning
		}
		ret = append(ret, item)
	}
	writeJSON(w, http.StatusOK, episodesListResponse{
		TargetLanguage: s.scanner.TargetLanguage(),
		Episodes:       ret,
	})
}

func (s *Server) handleItemCover(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/library/items/")
	path = strings.TrimSuffix(strings.TrimSuffix(path, "/"), "/cover")
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}
	coverPath, ok := s.scanner.ItemCoverPath(path)
	if !ok {
		writeError(w, http.StatusNotFound, "cover not found")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, coverPath)
}

type episodesListResponse struct {
	TargetLanguage string            `json:"target_language"`
	Episodes       []episodeResponse `json:"episodes"`
}

type episodeResponse struct {
	library.Episode
	InProgress bool        `json:"in_progress"`
	JobStatus  jobs.Status `json:"job_status,omitempty"`
	JobSource  string      `json:"job_source,omitempty"`
}

type itemStatusSummary struct {
	ItemID      string `json:"item_id"`
	Total       int    `json:"total"`
	NoNeed      int    `json:"no_need"`
	Idle        int    `json:"idle"`
	Pending     int    `json:"pending"`
	Translating int    `json:"translating"`
	Failed      int    `json:"failed"`
	Skipped     int    `json:"skipped"`
	NoSource    int    `json:"no_source"`
}

func summarizeItemStatuses(itemID string, episodes []library.Episode, jobsByMedia map[string]*jobs.TranslationJob) itemStatusSummary {
	ret := itemStatusSummary{ItemID: itemID, Total: len(episodes)}
	for _, episode := range episodes {
		job := jobsByMedia[episode.MediaPath]
		switch {
		case episode.Subtitles.HasTargetSubtitle || (job != nil && job.Status == jobs.StatusSuccess):
			ret.NoNeed++
		case !episode.Subtitles.HasSourceSubtitle:
			ret.NoSource++
		case job != nil && job.Status == jobs.StatusPending:
			ret.Pending++
		case job != nil && job.Status == jobs.StatusRunning:
			ret.Translating++
		case job != nil && job.Status == jobs.StatusFailed:
			ret.Failed++
		case job != nil && job.Status == jobs.StatusSkipped:
			ret.Skipped++
		default:
			ret.Idle++
		}
	}
	return ret
}

func latestJobsByMedia(jobList []*jobs.TranslationJob) map[string]*jobs.TranslationJob {
	ret := make(map[string]*jobs.TranslationJob)
	for _, job := range jobList {
		if job == nil || job.Payload.MediaFile == "" {
			continue
		}
		existing, ok := ret[job.Payload.MediaFile]
		if !ok || preferLatestJob(job, existing) {
			ret[job.Payload.MediaFile] = job
		}
	}
	return ret
}

func preferLatestJob(next, current *jobs.TranslationJob) bool {
	if !next.UpdatedAt.Equal(current.UpdatedAt) {
		return next.UpdatedAt.After(current.UpdatedAt)
	}
	if !next.CreatedAt.Equal(current.CreatedAt) {
		return next.CreatedAt.After(current.CreatedAt)
	}
	return next.ID > current.ID
}

type enqueueJobRequest struct {
	Source       string `json:"source"`
	DedupeKey    string `json:"dedupe_key"`
	MediaPath    string `json:"media_path"`
	SubtitlePath string `json:"subtitle_path"`
	NFOPath      string `json:"nfo_path"`
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.queue.List())
	case http.MethodPost:
		var req enqueueJobRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if req.Source == "" {
			req.Source = "manual"
		}
		if req.MediaPath == "" {
			writeError(w, http.StatusBadRequest, "media_path is required")
			return
		}
		req.DedupeKey = req.MediaPath

		job, created := s.queue.Enqueue(jobs.EnqueueRequest{
			Source:    req.Source,
			DedupeKey: req.DedupeKey,
			Payload: jobs.JobPayload{
				MediaFile:    req.MediaPath,
				SubtitleFile: req.SubtitlePath,
				NFOFile:      req.NFOPath,
			},
		})
		code := http.StatusCreated
		if !created {
			code = http.StatusOK
		}
		writeJSON(w, code, map[string]any{
			"created": created,
			"job":     job,
		})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.scanner.Invalidate()
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok": true,
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeError(w, http.StatusNotImplemented, "settings store is not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		settings, err := s.settings.GetRuntimeSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		var req config.RuntimeSettings
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if err := req.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		saved, err := s.settings.UpdateRuntimeSettings(req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if s.apply != nil {
			if err := s.apply(saved); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		writeJSON(w, http.StatusOK, saved)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": msg,
	})
}
