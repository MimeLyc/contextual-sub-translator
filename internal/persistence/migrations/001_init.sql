-- schema_migrations is bootstrapped in code; no need to create it here.

CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    dedupe_key TEXT NOT NULL,
    media_file TEXT NOT NULL,
    subtitle_file TEXT NOT NULL DEFAULT '',
    nfo_file TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_jobs_updated_at ON jobs(updated_at);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_dedupe_active ON jobs(dedupe_key)
    WHERE status IN ('pending', 'running');

CREATE TABLE IF NOT EXISTS job_batch_checkpoints (
    job_id TEXT NOT NULL,
    batch_start INTEGER NOT NULL,
    batch_end INTEGER NOT NULL,
    translated_json TEXT NOT NULL,
    updated_at DATETIME NOT NULL,
    PRIMARY KEY (job_id, batch_start, batch_end)
);

CREATE INDEX IF NOT EXISTS idx_job_batch_checkpoints_updated_at ON job_batch_checkpoints(updated_at);

CREATE TABLE IF NOT EXISTS subtitle_cache (
    cache_key TEXT PRIMARY KEY,
    media_path TEXT NOT NULL,
    job_id TEXT NOT NULL DEFAULT '',
    format TEXT NOT NULL,
    language TEXT NOT NULL,
    path_hint TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL,
    is_temp INTEGER NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_subtitle_cache_job_id ON subtitle_cache(job_id);
CREATE INDEX IF NOT EXISTS idx_subtitle_cache_updated_at ON subtitle_cache(updated_at);

CREATE TABLE IF NOT EXISTS media_meta_cache (
    media_path TEXT NOT NULL,
    target_lang TEXT NOT NULL,
    external_langs_json TEXT NOT NULL,
    embedded_langs_json TEXT NOT NULL,
    has_target_external INTEGER NOT NULL,
    has_target_embedded INTEGER NOT NULL,
    expires_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    PRIMARY KEY (media_path, target_lang)
);

CREATE INDEX IF NOT EXISTS idx_media_meta_cache_expires_at ON media_meta_cache(expires_at);
