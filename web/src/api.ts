export interface Source {
  id: string;
  name: string;
  path: string;
  item_count: number;
}

export interface Item {
  id: string;
  source_id: string;
  name: string;
  path: string;
  episode_count: number;
}

export interface SubtitleStatus {
  has_source_subtitle: boolean;
  has_target_subtitle: boolean;
  has_embedded_subtitle: boolean;
  has_embedded_target_subtitle: boolean;
  source_subtitle_files: string[];
  target_subtitle_files: string[];
  languages: string[];
}

export interface Episode {
  id: string;
  source_id: string;
  item_id: string;
  name: string;
  season: string;
  media_path: string;
  subtitles: SubtitleStatus;
  translatable: boolean;
  in_progress?: boolean;
  job_status?: Job["status"];
  job_source?: string;
}

export interface Job {
  id: string;
  source: string;
  dedupe_key: string;
  payload: JobPayload;
  status: "pending" | "running" | "success" | "failed" | "skipped";
  error?: string;
  created_at: string;
  updated_at: string;
}

export interface JobPayload {
  media_file: string;
  subtitle_file: string;
  nfo_file: string;
}

export interface RuntimeSettings {
  llm_api_url: string;
  llm_api_key: string;
  llm_model: string;
  cron_expr: string;
  target_language: string;
}

export interface CreateJobRequest {
  source: string;
  dedupeKey: string;
  mediaPath: string;
  subtitlePath?: string;
  nfoPath?: string;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers || {})
    }
  });
  if (!res.ok) {
    const msg = await res.text();
    throw new Error(msg || `request failed: ${res.status}`);
  }
  return (await res.json()) as T;
}

export function listSources(): Promise<Source[]> {
  return request<Source[]>("/api/library/sources");
}

export function listItems(sourceId: string): Promise<Item[]> {
  const q = new URLSearchParams({ source: sourceId });
  return request<Item[]>(`/api/library/items?${q.toString()}`);
}

export interface EpisodesResponse {
  target_language: string;
  episodes: Episode[];
}

export function listEpisodes(itemId: string): Promise<EpisodesResponse> {
  return request<EpisodesResponse>(`/api/library/items/${encodeURIComponent(itemId)}/episodes`);
}

export function listJobs(): Promise<Job[]> {
  return request<Job[]>("/api/jobs");
}

export async function createJob(req: CreateJobRequest): Promise<Job> {
  const ret = await request<{ created: boolean; job: Job }>("/api/jobs", {
    method: "POST",
    body: JSON.stringify({
      source: req.source,
      dedupe_key: req.dedupeKey,
      media_path: req.mediaPath,
      subtitle_path: req.subtitlePath || "",
      nfo_path: req.nfoPath || ""
    })
  });
  return ret.job;
}

export async function triggerScan(): Promise<void> {
  await request<{ ok: boolean }>("/api/scan", { method: "POST" });
}

export function getSettings(): Promise<RuntimeSettings> {
  return request<RuntimeSettings>("/api/settings");
}

export function updateSettings(settings: RuntimeSettings): Promise<RuntimeSettings> {
  return request<RuntimeSettings>("/api/settings", {
    method: "PUT",
    body: JSON.stringify(settings)
  });
}
