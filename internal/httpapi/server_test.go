package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MimeLyc/contextual-sub-translator/internal/config"
	"github.com/MimeLyc/contextual-sub-translator/internal/jobs"
	"github.com/MimeLyc/contextual-sub-translator/internal/library"
	"github.com/MimeLyc/contextual-sub-translator/internal/persistence"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

type fakeSettingsStore struct {
	current   config.RuntimeSettings
	updateErr error
}

func (f *fakeSettingsStore) GetRuntimeSettings() (config.RuntimeSettings, error) {
	return f.current, nil
}

func (f *fakeSettingsStore) UpdateRuntimeSettings(next config.RuntimeSettings) (config.RuntimeSettings, error) {
	if f.updateErr != nil {
		return config.RuntimeSettings{}, f.updateErr
	}
	f.current = next
	return f.current, nil
}

func TestServer_ListSources(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "tvshows")
	require.NoError(t, os.MkdirAll(sourcePath, 0o755))

	scanner := library.NewScanner(
		[]library.SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: sourcePath},
		},
		language.Chinese,
	)

	queue := jobs.NewQueue(1, nil)
	srv := NewServer(scanner, queue)

	req := httptest.NewRequest(http.MethodGet, "/api/library/sources", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var sources []library.Source
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &sources))
	require.Len(t, sources, 1)
	require.Equal(t, "tvshows", sources[0].ID)
}

func TestServer_CreateJob_WithPayload(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "tvshows")
	require.NoError(t, os.MkdirAll(sourcePath, 0o755))

	scanner := library.NewScanner(
		[]library.SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: sourcePath},
		},
		language.Chinese,
	)

	queue := jobs.NewQueue(1, nil)
	srv := NewServer(scanner, queue)

	body := []byte(`{"source":"manual","dedupe_key":"m|s|zh","media_path":"/tmp/a.mkv","subtitle_path":"/tmp/a.srt","nfo_path":"/tmp/tvshow.nfo"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var ret struct {
		Created bool                 `json:"created"`
		Job     *jobs.TranslationJob `json:"job"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ret))
	require.True(t, ret.Created)
	require.NotNil(t, ret.Job)
	require.Equal(t, "m|s|zh", ret.Job.DedupeKey)
	require.Equal(t, "/tmp/a.mkv", ret.Job.Payload.MediaFile)
	require.Equal(t, "/tmp/a.srt", ret.Job.Payload.SubtitleFile)
	require.Equal(t, "/tmp/tvshow.nfo", ret.Job.Payload.NFOFile)
}

func TestServer_CreateJob_RequiresMediaPath(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "tvshows")
	require.NoError(t, os.MkdirAll(sourcePath, 0o755))

	scanner := library.NewScanner(
		[]library.SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: sourcePath},
		},
		language.Chinese,
	)

	queue := jobs.NewQueue(1, nil)
	srv := NewServer(scanner, queue)

	req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader([]byte(`{"source":"manual"}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestServer_ListEpisodes_IncludesCronInProgress(t *testing.T) {
	tmp := t.TempDir()
	showDir := filepath.Join(tmp, "tvshows", "The Show")
	require.NoError(t, os.MkdirAll(showDir, 0o755))

	mediaPath := filepath.Join(showDir, "episode01.mkv")
	subtitlePath := filepath.Join(showDir, "episode01.srt")
	require.NoError(t, os.WriteFile(mediaPath, []byte("media"), 0o644))
	require.NoError(t, os.WriteFile(subtitlePath, []byte("subtitle"), 0o644))

	scanner := library.NewScanner(
		[]library.SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: filepath.Join(tmp, "tvshows")},
		},
		language.Chinese,
	)

	items, err := scanner.ScanItems(context.Background(), "tvshows")
	require.NoError(t, err)
	require.Len(t, items, 1)

	queue := jobs.NewQueue(1, nil)
	job, created := queue.Enqueue(jobs.EnqueueRequest{
		Source:    "cron",
		DedupeKey: mediaPath + "|" + subtitlePath + "|zh",
		Payload: jobs.JobPayload{
			MediaFile:    mediaPath,
			SubtitleFile: subtitlePath,
		},
	})
	require.True(t, created)
	require.NotNil(t, job)
	require.Equal(t, jobs.StatusPending, job.Status)

	srv := NewServer(scanner, queue)
	itemID := url.PathEscape(items[0].ID)
	req := httptest.NewRequest(http.MethodGet, "/api/library/items/"+itemID+"/episodes", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		TargetLanguage string `json:"target_language"`
		Episodes       []struct {
			ID         string      `json:"id"`
			InProgress bool        `json:"in_progress"`
			JobStatus  jobs.Status `json:"job_status"`
			JobSource  string      `json:"job_source"`
		} `json:"episodes"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "zh", resp.TargetLanguage)
	require.Len(t, resp.Episodes, 1)
	require.Equal(t, mediaPath, resp.Episodes[0].ID)
	require.True(t, resp.Episodes[0].InProgress)
	require.Equal(t, jobs.StatusPending, resp.Episodes[0].JobStatus)
	require.Equal(t, "cron", resp.Episodes[0].JobSource)
}

func TestServer_GetSettings(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "tvshows")
	require.NoError(t, os.MkdirAll(sourcePath, 0o755))

	scanner := library.NewScanner(
		[]library.SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: sourcePath},
		},
		language.Chinese,
	)

	store := &fakeSettingsStore{
		current: config.RuntimeSettings{
			LLMAPIURL:      "https://example.test/v1",
			LLMAPIKey:      "ak-test",
			LLMModel:       "model-test",
			CronExpr:       "*/5 * * * *",
			TargetLanguage: "zh",
		},
	}
	srv := NewServer(scanner, jobs.NewQueue(1, nil), WithRuntimeSettingsStore(store))

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got config.RuntimeSettings
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, store.current, got)
}

func TestServer_UpdateSettings(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "tvshows")
	require.NoError(t, os.MkdirAll(sourcePath, 0o755))

	scanner := library.NewScanner(
		[]library.SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: sourcePath},
		},
		language.Chinese,
	)

	store := &fakeSettingsStore{
		current: config.RuntimeSettings{
			LLMAPIURL:      "https://old.example/v1",
			LLMAPIKey:      "old-ak",
			LLMModel:       "old-model",
			CronExpr:       "0 0 * * *",
			TargetLanguage: "zh",
		},
	}
	srv := NewServer(scanner, jobs.NewQueue(1, nil), WithRuntimeSettingsStore(store))

	body := []byte(`{"llm_api_url":"https://new.example/v1","llm_api_key":"new-ak","llm_model":"new-model","cron_expr":"*/10 * * * *","target_language":"en"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got config.RuntimeSettings
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "https://new.example/v1", got.LLMAPIURL)
	require.Equal(t, "new-ak", got.LLMAPIKey)
	require.Equal(t, "new-model", got.LLMModel)
	require.Equal(t, "*/10 * * * *", got.CronExpr)
	require.Equal(t, "en", got.TargetLanguage)
	require.Equal(t, got, store.current)
}

func TestServer_UpdateSettings_StoreFailure(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "tvshows")
	require.NoError(t, os.MkdirAll(sourcePath, 0o755))

	scanner := library.NewScanner(
		[]library.SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: sourcePath},
		},
		language.Chinese,
	)

	store := &fakeSettingsStore{
		current: config.RuntimeSettings{
			LLMAPIURL:      "https://old.example/v1",
			LLMAPIKey:      "old-ak",
			LLMModel:       "old-model",
			CronExpr:       "0 0 * * *",
			TargetLanguage: "zh",
		},
		updateErr: errors.New("save failed"),
	}
	srv := NewServer(scanner, jobs.NewQueue(1, nil), WithRuntimeSettingsStore(store))

	body := []byte(`{"llm_api_url":"https://new.example/v1","llm_api_key":"new-ak","llm_model":"new-model","cron_expr":"*/10 * * * *","target_language":"en"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestServer_UpdateSettings_AppliesRuntimeSettingsImmediately(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "tvshows")
	require.NoError(t, os.MkdirAll(sourcePath, 0o755))

	scanner := library.NewScanner(
		[]library.SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: sourcePath},
		},
		language.Chinese,
	)

	store := &fakeSettingsStore{
		current: config.RuntimeSettings{
			LLMAPIURL:      "https://old.example/v1",
			LLMAPIKey:      "old-ak",
			LLMModel:       "old-model",
			CronExpr:       "0 0 * * *",
			TargetLanguage: "zh",
		},
	}

	var applied config.RuntimeSettings
	var applyCalls int
	srv := NewServer(
		scanner,
		jobs.NewQueue(1, nil),
		WithRuntimeSettingsStore(store),
		WithRuntimeSettingsApplier(func(next config.RuntimeSettings) error {
			applied = next
			applyCalls++
			return nil
		}),
	)

	body := []byte(`{"llm_api_url":"https://new.example/v1","llm_api_key":"new-ak","llm_model":"new-model","cron_expr":"*/10 * * * *","target_language":"en"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, applyCalls)
	require.Equal(t, "en", applied.TargetLanguage)
	require.Equal(t, "*/10 * * * *", applied.CronExpr)
}

func TestServer_GetJobDetail_ReportsCheckpointProgress(t *testing.T) {
	tmp := t.TempDir()
	showDir := filepath.Join(tmp, "tvshows", "The Show", "Season 1")
	require.NoError(t, os.MkdirAll(showDir, 0o755))

	mediaPath := filepath.Join(showDir, "episode01.mkv")
	subtitlePath := filepath.Join(showDir, "episode01.srt")
	require.NoError(t, os.WriteFile(mediaPath, []byte("media"), 0o644))
	require.NoError(t, os.WriteFile(subtitlePath, []byte(sampleSRTThreeLines), 0o644))

	scanner := library.NewScanner(
		[]library.SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: filepath.Join(tmp, "tvshows")},
		},
		language.Chinese,
	)

	store, err := persistence.NewSQLiteStore(filepath.Join(tmp, "state.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	queue := jobs.NewQueue(1, store)
	job, created := queue.Enqueue(jobs.EnqueueRequest{
		Source:    "manual",
		DedupeKey: mediaPath + "|" + subtitlePath + "|zh",
		Payload: jobs.JobPayload{
			MediaFile:    mediaPath,
			SubtitleFile: subtitlePath,
		},
	})
	require.True(t, created)
	require.NotNil(t, job)
	require.NoError(t, store.SaveBatchCheckpoint(context.Background(), job.ID, 0, 2, []string{"第一行", "第二行"}))

	srv := NewServer(scanner, queue, WithJobDataStore(store))
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/"+job.ID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
		TargetLanguage string `json:"target_language"`
		Progress       struct {
			TranslatedLines int     `json:"translated_lines"`
			TotalLines      int     `json:"total_lines"`
			Percent         float64 `json:"percent"`
		} `json:"progress"`
		Episode struct {
			SeriesName  string `json:"series_name"`
			EpisodeName string `json:"episode_name"`
			Season      string `json:"season"`
			MediaPath   string `json:"media_path"`
		} `json:"episode"`
		Preview []struct {
			Index          int    `json:"index"`
			OriginalText   string `json:"original_text"`
			TranslatedText string `json:"translated_text"`
		} `json:"preview"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, job.ID, resp.Job.ID)
	require.Equal(t, "zh", resp.TargetLanguage)
	require.Equal(t, 2, resp.Progress.TranslatedLines)
	require.Equal(t, 3, resp.Progress.TotalLines)
	require.InDelta(t, 66.6667, resp.Progress.Percent, 0.1)
	require.Equal(t, "The Show", resp.Episode.SeriesName)
	require.Equal(t, "Season 1", resp.Episode.Season)
	require.Equal(t, mediaPath, resp.Episode.MediaPath)
	require.Len(t, resp.Preview, 3)
	require.Equal(t, "第一行", resp.Preview[0].TranslatedText)
	require.Equal(t, "", resp.Preview[2].TranslatedText)
}

func TestServer_GetJobDetail_FallsBackTargetLanguageWhenDedupeSuffixIsNotLanguage(t *testing.T) {
	tmp := t.TempDir()
	showDir := filepath.Join(tmp, "tvshows", "The Show")
	require.NoError(t, os.MkdirAll(showDir, 0o755))

	mediaPath := filepath.Join(showDir, "episode01.mkv")
	require.NoError(t, os.WriteFile(mediaPath, []byte("media"), 0o644))

	scanner := library.NewScanner(
		[]library.SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: filepath.Join(tmp, "tvshows")},
		},
		language.Chinese,
	)

	queue := jobs.NewQueue(1, nil)
	job, created := queue.Enqueue(jobs.EnqueueRequest{
		Source:    "manual",
		DedupeKey: mediaPath + "|[embedded]",
		Payload: jobs.JobPayload{
			MediaFile: mediaPath,
		},
	})
	require.True(t, created)
	require.NotNil(t, job)

	srv := NewServer(scanner, queue)
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/"+job.ID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		TargetLanguage string `json:"target_language"`
		Episode        struct {
			OutputSubtitlePath string `json:"output_subtitle_path"`
		} `json:"episode"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "zh", resp.TargetLanguage)
	require.Equal(t, filepath.Join(showDir, "episode01_ctxtrans.zh.srt"), resp.Episode.OutputSubtitlePath)
}

func TestServer_UpdateJobLine_RejectsRunningJob(t *testing.T) {
	tmp := t.TempDir()
	showDir := filepath.Join(tmp, "tvshows", "The Show")
	require.NoError(t, os.MkdirAll(showDir, 0o755))

	mediaPath := filepath.Join(showDir, "episode01.mkv")
	subtitlePath := filepath.Join(showDir, "episode01.srt")
	require.NoError(t, os.WriteFile(mediaPath, []byte("media"), 0o644))
	require.NoError(t, os.WriteFile(subtitlePath, []byte(sampleSRTThreeLines), 0o644))

	scanner := library.NewScanner(
		[]library.SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: filepath.Join(tmp, "tvshows")},
		},
		language.Chinese,
	)

	queue := jobs.NewQueue(1, nil)
	job, created := queue.Enqueue(jobs.EnqueueRequest{
		Source:    "manual",
		DedupeKey: mediaPath + "|" + subtitlePath + "|zh",
		Payload: jobs.JobPayload{
			MediaFile:    mediaPath,
			SubtitleFile: subtitlePath,
		},
	})
	require.True(t, created)

	srv := NewServer(scanner, queue)
	body := []byte(`{"lines":[{"index":1,"translated_text":"已修改"}]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/jobs/"+job.ID+"/lines", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestServer_UpdateJobLine_UpdatesOutputFileForCompletedJob(t *testing.T) {
	tmp := t.TempDir()
	showDir := filepath.Join(tmp, "tvshows", "The Show")
	require.NoError(t, os.MkdirAll(showDir, 0o755))

	mediaPath := filepath.Join(showDir, "episode01.mkv")
	subtitlePath := filepath.Join(showDir, "episode01.srt")
	require.NoError(t, os.WriteFile(mediaPath, []byte("media"), 0o644))
	require.NoError(t, os.WriteFile(subtitlePath, []byte(sampleSRTThreeLines), 0o644))

	scanner := library.NewScanner(
		[]library.SourceConfig{
			{ID: "tvshows", Name: "TV Shows", Path: filepath.Join(tmp, "tvshows")},
		},
		language.Chinese,
	)

	queue := jobs.NewQueue(1, nil)
	queue.Start(func(_ context.Context, _ *jobs.TranslationJob) error { return nil })
	t.Cleanup(func() {
		queue.Stop()
	})

	job, created := queue.Enqueue(jobs.EnqueueRequest{
		Source:    "manual",
		DedupeKey: mediaPath + "|" + subtitlePath + "|zh",
		Payload: jobs.JobPayload{
			MediaFile:    mediaPath,
			SubtitleFile: subtitlePath,
		},
	})
	require.True(t, created)
	require.Eventually(t, func() bool {
		got, ok := queue.Get(job.ID)
		return ok && got.Status == jobs.StatusSuccess
	}, time.Second, 20*time.Millisecond)

	srv := NewServer(scanner, queue)
	body := []byte(`{"lines":[{"index":1,"translated_text":"第一行已改"},{"index":2,"translated_text":"第二行已改"}]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/jobs/"+job.ID+"/lines", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	outputPath := filepath.Join(showDir, "episode01_ctxtrans.zh.srt")
	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "第一行已改")
	require.Contains(t, string(data), "第二行已改")
}

const sampleSRTThreeLines = `1
00:00:01,000 --> 00:00:02,000
line one

2
00:00:03,000 --> 00:00:04,000
line two

3
00:00:05,000 --> 00:00:06,000
line three

`
