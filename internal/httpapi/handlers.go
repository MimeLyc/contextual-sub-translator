package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/MimeLyc/contextual-sub-translator/internal/config"
	"github.com/MimeLyc/contextual-sub-translator/internal/jobs"
	"github.com/MimeLyc/contextual-sub-translator/internal/library"
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

func (s *Server) handleListEpisodesByItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
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

	activeJobsByMedia := inProgressJobsByMedia(s.queue.List())
	ret := make([]episodeResponse, 0, len(episodes))
	for _, episode := range episodes {
		item := episodeResponse{
			Episode: episode,
		}
		if job, ok := activeJobsByMedia[episode.MediaPath]; ok {
			item.InProgress = true
			item.JobStatus = job.Status
			item.JobSource = job.Source
		}
		ret = append(ret, item)
	}
	writeJSON(w, http.StatusOK, episodesListResponse{
		TargetLanguage: s.scanner.TargetLanguage(),
		Episodes:       ret,
	})
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

func inProgressJobsByMedia(jobList []*jobs.TranslationJob) map[string]*jobs.TranslationJob {
	ret := make(map[string]*jobs.TranslationJob)
	for _, job := range jobList {
		if job == nil || job.Payload.MediaFile == "" {
			continue
		}
		if job.Status != jobs.StatusPending && job.Status != jobs.StatusRunning {
			continue
		}
		existing, ok := ret[job.Payload.MediaFile]
		if !ok || preferInProgressJob(job, existing) {
			ret[job.Payload.MediaFile] = job
		}
	}
	return ret
}

func preferInProgressJob(next, current *jobs.TranslationJob) bool {
	nextRank := inProgressRank(next.Status)
	currentRank := inProgressRank(current.Status)
	if nextRank != currentRank {
		return nextRank > currentRank
	}
	return next.UpdatedAt.After(current.UpdatedAt)
}

func inProgressRank(status jobs.Status) int {
	switch status {
	case jobs.StatusRunning:
		return 2
	case jobs.StatusPending:
		return 1
	default:
		return 0
	}
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
		if req.DedupeKey == "" {
			keySuffix := req.SubtitlePath
			if keySuffix == "" {
				keySuffix = "[embedded]"
			}
			req.DedupeKey = req.MediaPath + "|" + keySuffix
		}

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
