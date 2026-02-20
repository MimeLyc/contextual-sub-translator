<template>
  <section class="panel">
    <div class="panel-head">
      <div class="breadcrumb">
        <router-link class="breadcrumb-link" to="/">Library</router-link>
        <span class="breadcrumb-sep">/</span>
        <span>{{ seriesName }}</span>
      </div>
      <div class="row-gap">
        <button class="btn" @click="refresh">Reload</button>
        <button class="btn btn-primary" :disabled="selectedCount === 0 || submitting" @click="submitSelected">
          {{ submitButtonLabel }}
        </button>
      </div>
    </div>

    <p v-if="message" class="settings-message">{{ message }}</p>

    <div class="episode-list">
      <template v-for="group in seasonGroups" :key="group.season">
        <h3 v-if="showSeasonHeadings" class="season-heading">{{ group.label }}</h3>
        <label v-for="ep in group.episodes" :key="ep.id" class="episode-row">
          <input
            v-model="selectedEpisodeIds"
            type="checkbox"
            :value="ep.id"
            :disabled="!ep.translatable || !!ep.in_progress"
          />
          <div class="episode-main">
            <div class="episode-title">{{ ep.name }}</div>
          </div>
          <div class="status-col">
            <span class="chip" :class="statusClass(ep)">{{ statusLabel(ep) }}</span>
          </div>
          <div class="lang-col" :title="langTooltip(ep)">
            <template v-if="langList(ep).length > 0">
              <span
                v-for="lang in langList(ep)"
                :key="lang"
                class="lang-tag"
                :class="langClass(lang)"
              >{{ lang }}</span>
            </template>
            <span v-else class="no-subs">No subtitles</span>
          </div>
        </label>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRoute } from "vue-router";
import { createJob, listEpisodes, type Episode } from "../api";

const EPISODE_REFRESH_INTERVAL_MS = 30_000;

const route = useRoute();
const episodes = ref<Episode[]>([]);
const targetLanguage = ref("");
const selectedEpisodeIds = ref<string[]>([]);
const selectedCount = computed(() => selectedEpisodeIds.value.length);
const submitting = ref(false);
const message = ref("");
const submitButtonLabel = computed(() => {
  if (submitting.value) {
    return `Queuing ${selectedCount.value} Selected...`;
  }
  return `Translate ${selectedCount.value} Selected`;
});
let refreshTimer: number | null = null;
let refreshing = false;

const seriesName = computed(() => {
  const rawItemId = route.params.itemId as string;
  const decoded = decodeURIComponent(rawItemId);
  const parts = decoded.split("|");
  const path = parts.length > 1 ? parts[1] : decoded;
  const segments = path.split("/").filter(Boolean);
  return segments.length > 0 ? segments[segments.length - 1] : decoded;
});

interface SeasonGroup {
  season: string;
  label: string;
  episodes: Episode[];
}

const seasonGroups = computed<SeasonGroup[]>(() => {
  const groups = new Map<string, Episode[]>();
  for (const ep of episodes.value) {
    const key = ep.season || "";
    if (!groups.has(key)) {
      groups.set(key, []);
    }
    groups.get(key)!.push(ep);
  }

  const result: SeasonGroup[] = [];
  for (const [season, eps] of groups) {
    eps.sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }));
    result.push({
      season,
      label: season || "Episodes",
      episodes: eps,
    });
  }

  result.sort((a, b) => a.season.localeCompare(b.season, undefined, { numeric: true }));
  return result;
});

const showSeasonHeadings = computed(() => {
  const groups = seasonGroups.value;
  if (groups.length === 0) return false;
  if (groups.length === 1 && groups[0].season === "") return false;
  return true;
});

function statusLabel(ep: Episode): string {
  if (!ep.subtitles.has_source_subtitle) return "No Source";
  if (ep.in_progress) return ep.job_source === "cron" ? "Cron Translating" : "Translating";
  if (ep.subtitles.has_target_subtitle) return "Translated";
  return "Pending";
}

function statusClass(ep: Episode): string {
  if (!ep.subtitles.has_source_subtitle) return "bad";
  if (ep.in_progress) return "warn";
  if (ep.subtitles.has_target_subtitle) return "ok";
  return "";
}

function langList(ep: Episode): string[] {
  const langs = ep.subtitles.languages || [];
  if (langs.length === 0) return [];
  const tgt = targetLanguage.value;
  if (!tgt) return langs;
  const sorted = [...langs].sort((a, b) => {
    const aIsTarget = isTargetLang(a) ? 0 : 1;
    const bIsTarget = isTargetLang(b) ? 0 : 1;
    return aIsTarget - bIsTarget;
  });
  return sorted;
}

function isTargetLang(lang: string): boolean {
  const tgt = targetLanguage.value;
  if (!tgt) return false;
  return lang === tgt || lang.startsWith(tgt + "-");
}

function langClass(lang: string): string {
  if (isTargetLang(lang)) return "target";
  return "";
}

function langTooltip(ep: Episode): string {
  const langs = ep.subtitles.languages || [];
  if (langs.length === 0) return "No subtitles detected";
  return langs.join(", ");
}

function dedupeKey(ep: Episode): string {
  const sourceSub = ep.subtitles.source_subtitle_files[0] || "[embedded]";
  return `${ep.media_path}|${sourceSub}|${targetLanguage.value || "zh"}`;
}

async function refresh() {
  if (refreshing) return;
  refreshing = true;
  try {
    const rawItemId = route.params.itemId as string;
    const resp = await listEpisodes(decodeURIComponent(rawItemId));
    episodes.value = resp.episodes || [];
    targetLanguage.value = resp.target_language || "";
    selectedEpisodeIds.value = selectedEpisodeIds.value.filter((id) =>
      episodes.value.some((ep) => ep.id === id && ep.translatable && !ep.in_progress)
    );
  } catch (err) {
    message.value = `Failed to refresh episodes: ${toErrorMessage(err)}`;
  } finally {
    refreshing = false;
  }
}

async function submitSelected() {
  if (submitting.value) return;
  const selected = episodes.value.filter(
    (ep) => selectedEpisodeIds.value.includes(ep.id) && ep.translatable && !ep.in_progress
  );
  if (selected.length === 0) {
    message.value = "No translatable episodes selected.";
    return;
  }

  submitting.value = true;
  message.value = "";
  let createdCount = 0;
  let dedupedCount = 0;
  let failedCount = 0;

  const results = await Promise.allSettled(
    selected.map((ep) =>
      createJob({
        source: "manual",
        dedupeKey: dedupeKey(ep),
        mediaPath: ep.media_path,
        subtitlePath: ep.subtitles.source_subtitle_files[0] || ""
      })
    )
  );

  for (const result of results) {
    if (result.status === "fulfilled") {
      if (result.value.created) {
        createdCount += 1;
      } else {
        dedupedCount += 1;
      }
    } else {
      failedCount += 1;
    }
  }

  if (failedCount > 0) {
    message.value = `Queued ${createdCount} episode(s), skipped ${dedupedCount}, failed ${failedCount}.`;
  } else {
    message.value = `Queued ${createdCount} episode(s), skipped ${dedupedCount}.`;
  }

  selectedEpisodeIds.value = [];
  await refresh();
  submitting.value = false;
}

function toErrorMessage(err: unknown): string {
  if (err instanceof Error && err.message.trim()) {
    return err.message;
  }
  return "unknown error";
}

onMounted(async () => {
  await refresh();
  refreshTimer = window.setInterval(() => {
    void refresh();
  }, EPISODE_REFRESH_INTERVAL_MS);
});

onUnmounted(() => {
  if (refreshTimer !== null) {
    window.clearInterval(refreshTimer);
    refreshTimer = null;
  }
});
</script>
