<template>
  <section class="panel">
    <div class="panel-head">
      <div class="title-block">
        <div class="breadcrumb">
          <RouterLink class="breadcrumb-link" to="/library">Library</RouterLink>
          <span class="breadcrumb-sep">/</span>
          <RouterLink class="breadcrumb-link" :to="sourceRoute">{{ sourceName }}</RouterLink>
          <span class="breadcrumb-sep">/</span>
          <span>{{ seriesName }}</span>
        </div>
        <p>Select idle episodes to add them to the translation queue.</p>
      </div>
      <div class="row-gap">
        <button class="btn" @click="refresh">Reload</button>
        <button class="btn btn-primary" :disabled="selectedCount === 0 || submitting" @click="submitSelected">
          {{ submitButtonLabel }}
        </button>
      </div>
    </div>

    <p v-if="message" class="settings-message" role="status" aria-live="polite">{{ message }}</p>

    <div class="table-shell">
      <table class="episode-table">
        <thead>
          <tr>
            <th class="select-cell">
              <input
                type="checkbox"
                aria-label="Select all translatable episodes"
                :checked="allSelectableSelected"
                :indeterminate="someSelectableSelected"
                :disabled="selectableEpisodes.length === 0"
                @change="toggleSelectAll"
              />
            </th>
            <th>Episode</th>
            <th>Status</th>
            <th>Subtitles</th>
          </tr>
        </thead>
        <tbody v-for="group in seasonGroups" :key="group.season">
          <tr v-if="showSeasonHeadings" class="season-row">
            <th colspan="4">{{ group.label }}</th>
          </tr>
          <tr v-for="ep in group.episodes" :key="ep.id" :class="{ selected: selectedEpisodeIds.includes(ep.id) }">
            <td class="select-cell">
              <input
                v-model="selectedEpisodeIds"
                type="checkbox"
                :value="ep.id"
                :aria-label="`Select ${ep.name}`"
                :disabled="!canTranslate(ep)"
              />
            </td>
            <td>
              <div class="episode-title">{{ ep.name }}</div>
              <div class="episode-path">{{ mediaFileName(ep.media_path) }}</div>
            </td>
            <td>
              <span class="chip" :class="statusClass(ep)">{{ statusLabel(ep) }}</span>
            </td>
            <td>
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
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRoute } from "vue-router";
import { createJob, listEpisodes, listSources, type Episode } from "../api";

const EPISODE_REFRESH_INTERVAL_MS = 30_000;

const route = useRoute();
const episodes = ref<Episode[]>([]);
const targetLanguage = ref("");
const selectedEpisodeIds = ref<string[]>([]);
const selectedCount = computed(() => selectedEpisodeIds.value.length);
const selectableEpisodes = computed(() => episodes.value.filter(canTranslate));
const allSelectableSelected = computed(() => {
  return selectableEpisodes.value.length > 0 && selectableEpisodes.value.every((ep) => selectedEpisodeIds.value.includes(ep.id));
});
const someSelectableSelected = computed(() => {
  return !allSelectableSelected.value && selectableEpisodes.value.some((ep) => selectedEpisodeIds.value.includes(ep.id));
});
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

const decodedItemId = computed(() => decodeURIComponent(route.params.itemId as string));
const itemIdParts = computed(() => decodedItemId.value.split("|"));
const sourceId = computed(() => itemIdParts.value.length > 1 ? itemIdParts.value[0] : "");
const sourceRoute = computed(() => sourceId.value ? `/library/${encodeURIComponent(sourceId.value)}` : "/library");
const sourceName = ref(humanizeSourceId(sourceId.value));

const seriesName = computed(() => {
  const path = itemIdParts.value.length > 1 ? itemIdParts.value.slice(1).join("|") : decodedItemId.value;
  const segments = path.split("/").filter(Boolean);
  return segments.length > 0 ? segments[segments.length - 1] : decodedItemId.value;
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
  if (ep.subtitles.has_target_subtitle || ep.job_status === "success") return "No Need";
  if (!ep.subtitles.has_source_subtitle) return "No Source";
  if (ep.job_status === "pending") return "Pending";
  if (ep.job_status === "running") return "Translating";
  if (ep.job_status === "failed") return "Failed";
  if (ep.job_status === "skipped") return "Skipped";
  return "Idle";
}

function statusClass(ep: Episode): string {
  if (ep.subtitles.has_target_subtitle || ep.job_status === "success") return "ok";
  if (!ep.subtitles.has_source_subtitle || ep.job_status === "failed") return "bad";
  if (ep.job_status === "pending") return "warn";
  if (ep.job_status === "running") return "active";
  return "idle";
}

function canTranslate(ep: Episode) {
  return ep.translatable && !ep.in_progress && ep.job_status !== "success";
}

function toggleSelectAll() {
  if (allSelectableSelected.value) {
    const selectableIDs = new Set(selectableEpisodes.value.map((ep) => ep.id));
    selectedEpisodeIds.value = selectedEpisodeIds.value.filter((id) => !selectableIDs.has(id));
    return;
  }
  selectedEpisodeIds.value = selectableEpisodes.value.map((ep) => ep.id);
}

function mediaFileName(path: string) {
  return path.replaceAll("\\", "/").split("/").filter(Boolean).pop() || path;
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
  return ep.media_path;
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
      episodes.value.some((ep) => ep.id === id && canTranslate(ep))
    );
  } catch (err) {
    message.value = `Failed to refresh episodes: ${toErrorMessage(err)}`;
  } finally {
    refreshing = false;
  }
}

async function loadSourceName() {
  const fallback = humanizeSourceId(sourceId.value);
  sourceName.value = fallback || "Library";
  if (!sourceId.value) return;
  try {
    const sources = await listSources();
    sourceName.value = sources.find((source) => source.id === sourceId.value)?.name || fallback;
  } catch {
    // Keep the readable route-derived fallback; episode loading remains independent.
  }
}

function humanizeSourceId(value: string) {
  return value
    .replaceAll(/[-_]+/g, " ")
    .replace(/\b\p{L}/gu, (letter) => letter.toUpperCase());
}

async function submitSelected() {
  if (submitting.value) return;
  const selected = episodes.value.filter(
    (ep) => selectedEpisodeIds.value.includes(ep.id) && canTranslate(ep)
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

  for (const [index, result] of results.entries()) {
    if (result.status === "fulfilled") {
      const episode = selected[index];
      episode.job_status = result.value.job.status;
      episode.job_source = result.value.job.source;
      episode.in_progress = result.value.job.status === "pending" || result.value.job.status === "running";
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
  await Promise.all([refresh(), loadSourceName()]);
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
