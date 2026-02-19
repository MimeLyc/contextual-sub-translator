package persistence

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MimeLyc/contextual-sub-translator/internal/jobs"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"golang.org/x/text/language"
	_ "modernc.org/sqlite"
)

const mediaMetaDefaultTTL = 10 * time.Minute

//go:embed migrations/*.sql
var migrationFiles embed.FS

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("db path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &SQLiteStore{db: db}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) init(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA journal_mode = WAL;"); err != nil {
		return fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA busy_timeout = 5000;"); err != nil {
		return fmt.Errorf("set busy timeout: %w", err)
	}
	// Bootstrap schema_migrations table so we can track applied versions.
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		version := migrationVersion(entry.Name())
		if version <= 0 {
			continue
		}
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", entry.Name(), err)
		}
		if exists > 0 {
			continue
		}
		content, err := migrationFiles.ReadFile(filepath.Join("migrations", entry.Name()))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if _, err := s.db.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			return fmt.Errorf("record migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// migrationVersion extracts the leading integer from a migration filename (e.g. "001_init.sql" → 1).
func migrationVersion(name string) int {
	for i, c := range name {
		if c < '0' || c > '9' {
			if i == 0 {
				return 0
			}
			n, _ := strconv.Atoi(name[:i])
			return n
		}
	}
	n, _ := strconv.Atoi(name)
	return n
}

func (s *SQLiteStore) LoadJobs(ctx context.Context) ([]*jobs.TranslationJob, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, source, dedupe_key, media_file, subtitle_file, nfo_file, status, error, created_at, updated_at
		 FROM jobs
		 ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ret := make([]*jobs.TranslationJob, 0)
	for rows.Next() {
		var item jobs.TranslationJob
		var status string
		if err := rows.Scan(
			&item.ID,
			&item.Source,
			&item.DedupeKey,
			&item.Payload.MediaFile,
			&item.Payload.SubtitleFile,
			&item.Payload.NFOFile,
			&status,
			&item.Error,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Status = jobs.Status(status)
		ret = append(ret, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}

func (s *SQLiteStore) DeleteJob(ctx context.Context, jobID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE id = ?`, jobID)
	return err
}

func (s *SQLiteStore) UpsertJob(ctx context.Context, job *jobs.TranslationJob) error {
	if job == nil {
		return fmt.Errorf("job is nil")
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO jobs (
			id, source, dedupe_key, media_file, subtitle_file, nfo_file, status, error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			source=excluded.source,
			dedupe_key=excluded.dedupe_key,
			media_file=excluded.media_file,
			subtitle_file=excluded.subtitle_file,
			nfo_file=excluded.nfo_file,
			status=excluded.status,
			error=excluded.error,
			updated_at=excluded.updated_at`,
		job.ID,
		job.Source,
		job.DedupeKey,
		job.Payload.MediaFile,
		job.Payload.SubtitleFile,
		job.Payload.NFOFile,
		string(job.Status),
		job.Error,
		job.CreatedAt,
		job.UpdatedAt,
	)
	return err
}

func (s *SQLiteStore) SaveBatchCheckpoint(ctx context.Context, jobID string, batchStart int, batchEnd int, translatedLines []string) error {
	payload, err := json.Marshal(translatedLines)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO job_batch_checkpoints (job_id, batch_start, batch_end, translated_json, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(job_id, batch_start, batch_end) DO UPDATE SET
			translated_json=excluded.translated_json,
			updated_at=excluded.updated_at`,
		jobID,
		batchStart,
		batchEnd,
		string(payload),
		time.Now().UTC(),
	)
	return err
}

func (s *SQLiteStore) LoadBatchCheckpoints(ctx context.Context, jobID string) ([]BatchCheckpoint, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT job_id, batch_start, batch_end, translated_json, updated_at
		 FROM job_batch_checkpoints
		 WHERE job_id = ?
		 ORDER BY batch_start ASC`,
		jobID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ret := make([]BatchCheckpoint, 0)
	for rows.Next() {
		var item BatchCheckpoint
		var translatedJSON string
		if err := rows.Scan(&item.JobID, &item.BatchStart, &item.BatchEnd, &translatedJSON, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(translatedJSON), &item.TranslatedLines); err != nil {
			return nil, err
		}
		ret = append(ret, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}

type subtitlePayload struct {
	Lines    []subtitle.Line `json:"lines"`
	Language string          `json:"language"`
	Format   string          `json:"format"`
	Path     string          `json:"path"`
}

func (s *SQLiteStore) PutSubtitleCache(ctx context.Context, entry SubtitleCacheEntry) error {
	payload := subtitlePayload{
		Lines:    entry.File.Lines,
		Language: entry.File.Language.String(),
		Format:   entry.File.Format,
		Path:     entry.File.Path,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	updatedAt := entry.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO subtitle_cache (
			cache_key, media_path, job_id, format, language, path_hint, payload_json, is_temp, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET
			media_path=excluded.media_path,
			job_id=excluded.job_id,
			format=excluded.format,
			language=excluded.language,
			path_hint=excluded.path_hint,
			payload_json=excluded.payload_json,
			is_temp=excluded.is_temp,
			updated_at=excluded.updated_at`,
		entry.CacheKey,
		entry.MediaPath,
		entry.JobID,
		entry.File.Format,
		entry.File.Language.String(),
		entry.File.Path,
		string(jsonPayload),
		boolToInt(entry.IsTemp),
		updatedAt,
	)
	return err
}

func (s *SQLiteStore) GetSubtitleCache(ctx context.Context, cacheKey string) (subtitle.File, bool, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT payload_json
		 FROM subtitle_cache
		 WHERE cache_key = ?`,
		cacheKey,
	)
	var payloadJSON string
	if err := row.Scan(&payloadJSON); err != nil {
		if err == sql.ErrNoRows {
			return subtitle.File{}, false, nil
		}
		return subtitle.File{}, false, err
	}
	var payload subtitlePayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return subtitle.File{}, false, err
	}
	langTag, err := language.Parse(payload.Language)
	if err != nil {
		langTag = language.Und
	}
	ret := subtitle.File{
		Lines:    payload.Lines,
		Language: langTag,
		Format:   payload.Format,
		Path:     payload.Path,
	}
	return ret, true, nil
}

func (s *SQLiteStore) PutMediaMetaCache(ctx context.Context, meta MediaMetaCache) error {
	externalJSON, err := json.Marshal(meta.ExternalLanguages)
	if err != nil {
		return err
	}
	embeddedJSON, err := json.Marshal(meta.EmbeddedLanguages)
	if err != nil {
		return err
	}
	updatedAt := meta.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	expiresAt := meta.ExpiresAt.UTC()
	if expiresAt.IsZero() {
		expiresAt = updatedAt.Add(mediaMetaDefaultTTL)
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO media_meta_cache (
			media_path, target_lang, external_langs_json, embedded_langs_json, has_target_external, has_target_embedded, expires_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(media_path, target_lang) DO UPDATE SET
			external_langs_json=excluded.external_langs_json,
			embedded_langs_json=excluded.embedded_langs_json,
			has_target_external=excluded.has_target_external,
			has_target_embedded=excluded.has_target_embedded,
			expires_at=excluded.expires_at,
			updated_at=excluded.updated_at`,
		meta.MediaPath,
		meta.TargetLanguage,
		string(externalJSON),
		string(embeddedJSON),
		boolToInt(meta.HasTargetExternal),
		boolToInt(meta.HasTargetEmbedded),
		expiresAt,
		updatedAt,
	)
	return err
}

func (s *SQLiteStore) GetMediaMetaCache(ctx context.Context, mediaPath string, targetLanguage string, now time.Time) (MediaMetaCache, bool, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT media_path, target_lang, external_langs_json, embedded_langs_json, has_target_external, has_target_embedded, expires_at, updated_at
		 FROM media_meta_cache
		 WHERE media_path = ? AND target_lang = ? AND expires_at > ?`,
		mediaPath,
		targetLanguage,
		now.UTC(),
	)

	var ret MediaMetaCache
	var externalJSON string
	var embeddedJSON string
	var hasTargetExternal int
	var hasTargetEmbedded int
	if err := row.Scan(
		&ret.MediaPath,
		&ret.TargetLanguage,
		&externalJSON,
		&embeddedJSON,
		&hasTargetExternal,
		&hasTargetEmbedded,
		&ret.ExpiresAt,
		&ret.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return MediaMetaCache{}, false, nil
		}
		return MediaMetaCache{}, false, err
	}
	if err := json.Unmarshal([]byte(externalJSON), &ret.ExternalLanguages); err != nil {
		return MediaMetaCache{}, false, err
	}
	if err := json.Unmarshal([]byte(embeddedJSON), &ret.EmbeddedLanguages); err != nil {
		return MediaMetaCache{}, false, err
	}
	ret.HasTargetExternal = hasTargetExternal == 1
	ret.HasTargetEmbedded = hasTargetEmbedded == 1
	return ret, true, nil
}

func (s *SQLiteStore) ClearJobTemp(ctx context.Context, jobID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `DELETE FROM job_batch_checkpoints WHERE job_id = ?`, jobID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM subtitle_cache WHERE job_id = ? AND is_temp = 1`, jobID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteExpiredMediaMetaCache removes media_meta_cache rows whose expires_at is before now.
func (s *SQLiteStore) DeleteExpiredMediaMetaCache(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM media_meta_cache WHERE expires_at <= ?`, now.UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteJobData removes all data associated with a job (checkpoints + temp subtitle cache).
func (s *SQLiteStore) DeleteJobData(ctx context.Context, jobID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `DELETE FROM job_batch_checkpoints WHERE job_id = ?`, jobID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM subtitle_cache WHERE job_id = ?`, jobID); err != nil {
		return err
	}
	return tx.Commit()
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
