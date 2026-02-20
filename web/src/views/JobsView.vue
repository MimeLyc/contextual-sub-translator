<template>
  <section class="panel">
    <div class="panel-head">
      <h1>Jobs</h1>
      <div class="row-gap">
        <button class="btn" @click="refresh">Refresh</button>
      </div>
    </div>

    <div class="jobs-layout">
      <div class="job-list">
        <article
          v-for="job in jobs"
          :key="job.id"
          class="job-row job-card job-selectable"
          :class="{ active: selectedJobId === job.id }"
          @click="selectJob(job.id)"
        >
          <div class="job-main">
            <div class="job-title-line">
              <strong class="job-title">{{ titleName(job) }}</strong>
              <span class="chip" :class="statusClass(job.status)">{{ job.status }}</span>
            </div>
            <div class="job-subline">{{ episodeName(job) }}</div>
            <div class="job-meta-line">
              <span class="job-meta">ID: {{ job.id }}</span>
              <span class="job-meta">{{ formatCreatedAt(job.created_at) }}</span>
            </div>
          </div>
        </article>
      </div>

      <div v-if="detail" class="job-detail">
        <div class="job-detail-head">
          <h2>{{ detail.job.id }}</h2>
          <div class="chips">
            <span class="chip" :class="statusClass(detail.job.status)">{{ detail.job.status }}</span>
            <span v-if="!detail.editable" class="chip warn">Locked While Running</span>
          </div>
        </div>

        <div class="job-meta-grid">
          <div class="meta-item">
            <span>Target</span>
            <strong>{{ detail.target_language || "-" }}</strong>
          </div>
          <div class="meta-item">
            <span>Series</span>
            <strong>{{ detail.episode.series_name || "-" }}</strong>
          </div>
          <div class="meta-item">
            <span>Season</span>
            <strong>{{ detail.episode.season || "-" }}</strong>
          </div>
          <div class="meta-item">
            <span>Episode</span>
            <strong>{{ detail.episode.episode_name || "-" }}</strong>
          </div>
        </div>

        <div class="progress-card">
          <div class="progress-text">
            {{ detail.progress.translated_lines }} / {{ detail.progress.total_lines }} lines
            ({{ formatPercent(detail.progress.percent) }})
          </div>
          <div class="progress-track">
            <div class="progress-fill" :style="{ width: progressWidth(detail.progress.percent) }"></div>
          </div>
        </div>

        <p v-if="message" class="settings-message">{{ message }}</p>

        <div class="preview-list">
          <article v-for="line in detail.preview" :key="line.index" class="preview-row">
            <div class="preview-index">#{{ line.index }}</div>
            <div class="preview-original">{{ line.original_text || "-" }}</div>
            <textarea
              class="preview-edit"
              :disabled="!detail.editable || saving"
              :value="translatedText(line)"
              @input="onLineInput(line.index, ($event.target as HTMLTextAreaElement).value)"
            ></textarea>
          </article>
        </div>

        <div class="row-gap">
          <button class="btn" :disabled="loadingDetail || saving" @click="reloadDetail">Reload Detail</button>
          <button class="btn btn-primary" :disabled="!canSave" @click="saveChanges">
            {{ saving ? "Saving..." : `Save ${dirtyChanges.length} Changes` }}
          </button>
        </div>
      </div>

      <div v-else class="empty-msg">Select a job to view details.</div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import {
  getJobDetail,
  listJobs,
  updateJobLines,
  type Job,
  type JobDetail,
  type JobLinePatch,
  type JobPreviewLine
} from "../api";

const DETAIL_SYNC_INTERVAL_MS = 3_000;

const jobs = ref<Job[]>([]);
const detail = ref<JobDetail | null>(null);
const selectedJobId = ref<string>("");
const draftByLine = ref<Record<number, string>>({});
const loadingDetail = ref(false);
const saving = ref(false);
const message = ref("");
let evt: EventSource | null = null;
let syncingDetail = false;
let detailReqSeq = 0;
let lastDetailSyncAt = 0;

function sortJobs(input: Job[]): Job[] {
  return [...input].sort((a, b) => {
    const left = Date.parse(a.created_at);
    const right = Date.parse(b.created_at);
    if (Number.isFinite(left) && Number.isFinite(right) && left !== right) {
      return right - left;
    }
    return b.id.localeCompare(a.id);
  });
}

function mediaSegments(job: Job): string[] {
  const raw = job.payload?.media_file || "";
  return raw.replaceAll("\\", "/").split("/").filter(Boolean);
}

function mediaStem(job: Job): string {
  const segs = mediaSegments(job);
  const file = segs[segs.length - 1] || "";
  const idx = file.lastIndexOf(".");
  const stem = idx > 0 ? file.slice(0, idx) : file;
  return humanize(stem || "-");
}

function humanize(value: string): string {
  return value.replaceAll("_", " ").replaceAll(".", " ").trim() || "-";
}

function isSeasonName(value: string): boolean {
  return /^season\s*\d+/i.test(value) || /^s\d+$/i.test(value) || /第.+季/.test(value);
}

function titleName(job: Job): string {
  const segs = mediaSegments(job);
  if (segs.length <= 1) return mediaStem(job);
  const parent = segs[segs.length - 2];
  const grand = segs[segs.length - 3];
  if (parent && isSeasonName(parent) && grand) {
    return humanize(grand);
  }
  return humanize(parent || mediaStem(job));
}

function episodeName(job: Job): string {
  return mediaStem(job);
}

function formatCreatedAt(value: string): string {
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value || "-";
  return d.toLocaleString();
}

function statusClass(status: string) {
  if (status === "success") return "ok";
  if (status === "running" || status === "pending") return "warn";
  return "bad";
}

function selectedJob(): Job | undefined {
  return jobs.value.find((job) => job.id === selectedJobId.value);
}

function normalizeSelection() {
  if (jobs.value.length === 0) {
    selectedJobId.value = "";
    detail.value = null;
    draftByLine.value = {};
    return;
  }
  if (!selectedJobId.value || !jobs.value.some((job) => job.id === selectedJobId.value)) {
    selectedJobId.value = jobs.value[0].id;
    detail.value = null;
    draftByLine.value = {};
  }
}

function translatedText(line: JobPreviewLine) {
  const drafted = draftByLine.value[line.index];
  if (drafted !== undefined) return drafted;
  return line.translated_text || "";
}

function onLineInput(index: number, value: string) {
  draftByLine.value = {
    ...draftByLine.value,
    [index]: value
  };
}

const dirtyChanges = computed<JobLinePatch[]>(() => {
  if (!detail.value || !detail.value.editable) return [];
  const base = new Map<number, string>();
  for (const line of detail.value.preview) {
    base.set(line.index, line.translated_text || "");
  }
  const ret: JobLinePatch[] = [];
  for (const [idxRaw, drafted] of Object.entries(draftByLine.value)) {
    const idx = Number(idxRaw);
    const original = base.get(idx) ?? "";
    if (drafted !== original) {
      ret.push({
        index: idx,
        translated_text: drafted
      });
    }
  }
  ret.sort((a, b) => a.index - b.index);
  return ret;
});

const canSave = computed(() => {
  return !!detail.value?.editable && !saving.value && dirtyChanges.value.length > 0;
});

function formatPercent(value: number) {
  if (Number.isNaN(value)) return "0.0%";
  return `${Math.max(0, Math.min(100, value)).toFixed(1)}%`;
}

function progressWidth(value: number) {
  if (Number.isNaN(value)) return "0%";
  const clamped = Math.max(0, Math.min(100, value));
  return `${clamped}%`;
}

function errorText(err: unknown, fallback: string) {
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}

async function loadDetail(jobId: string, preserveDraft: boolean) {
  const reqSeq = ++detailReqSeq;
  loadingDetail.value = true;
  try {
    const next = await getJobDetail(jobId);
    if (reqSeq !== detailReqSeq || selectedJobId.value !== jobId) return;
    detail.value = next;
    message.value = "";
    if (!preserveDraft || !next.editable) {
      draftByLine.value = {};
    }
  } catch (err) {
    if (reqSeq !== detailReqSeq || selectedJobId.value !== jobId) return;
    detail.value = null;
    draftByLine.value = {};
    message.value = errorText(err, "Failed to load job detail");
  } finally {
    if (reqSeq === detailReqSeq) {
      loadingDetail.value = false;
    }
  }
}

async function syncSelectedDetail(preserveDraft: boolean) {
  if (!selectedJobId.value || syncingDetail) return;
  syncingDetail = true;
  try {
    await loadDetail(selectedJobId.value, preserveDraft);
  } finally {
    syncingDetail = false;
  }
}

async function refresh() {
  jobs.value = sortJobs(await listJobs());
  normalizeSelection();
  await syncSelectedDetail(true);
}

async function reloadDetail() {
  await syncSelectedDetail(true);
}

async function selectJob(jobId: string) {
  if (selectedJobId.value === jobId) return;
  selectedJobId.value = jobId;
  detail.value = null;
  draftByLine.value = {};
  message.value = "";
  await loadDetail(jobId, false);
}

async function saveChanges() {
  if (!selectedJobId.value || !detail.value) return;
  if (detail.value.job.id !== selectedJobId.value) return;
  if (dirtyChanges.value.length === 0 || !detail.value.editable) return;
  saving.value = true;
  try {
    detail.value = await updateJobLines(selectedJobId.value, dirtyChanges.value);
    draftByLine.value = {};
    message.value = "Changes saved";
  } catch (err) {
    message.value = errorText(err, "Failed to save changes");
    await syncSelectedDetail(true);
  } finally {
    saving.value = false;
  }
}

function shouldSyncDetailFromStream() {
  if (!selectedJobId.value) return false;
  const selected = selectedJob();
  if (!selected) return false;
  if (selected.status === "pending" || selected.status === "running") {
    const now = Date.now();
    if (now - lastDetailSyncAt < DETAIL_SYNC_INTERVAL_MS) return false;
    lastDetailSyncAt = now;
    return true;
  }
  if (!detail.value || detail.value.job.id !== selected.id) return true;
  return detail.value.job.updated_at !== selected.updated_at;
}

onMounted(async () => {
  await refresh();
  evt = new EventSource("/api/jobs/stream");
  evt.onmessage = (event) => {
    try {
      jobs.value = sortJobs(JSON.parse(event.data) as Job[]);
      normalizeSelection();
      if (shouldSyncDetailFromStream()) {
        void syncSelectedDetail(true);
      }
    } catch {
      // ignore malformed payload
    }
  };
});

onUnmounted(() => {
  if (evt) {
    evt.close();
    evt = null;
  }
});
</script>
