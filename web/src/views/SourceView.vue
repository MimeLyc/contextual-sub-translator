<template>
  <section class="panel">
    <div class="panel-head">
      <div class="title-block">
        <div class="breadcrumb">
          <RouterLink to="/library" class="breadcrumb-link">Library</RouterLink>
          <span class="breadcrumb-sep">/</span>
          <span>{{ sourceName }}</span>
        </div>
        <p>{{ items.length }} {{ items.length === 1 ? "title" : "titles" }} in this source</p>
      </div>
      <div class="row-gap collection-actions">
        <label class="sort-control">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="M8 7h10M8 12h7M8 17h4M4 7h.01M4 12h.01M4 17h.01" />
          </svg>
          <span class="sr-only">Sort titles</span>
          <select v-model="sortMode" aria-label="Sort titles">
            <option value="updated-desc">Recently updated</option>
            <option value="added-desc">Recently added</option>
            <option value="name-asc">Name A–Z</option>
            <option value="episodes-desc">Most episodes</option>
            <option value="actionable-desc">Most untranslated</option>
          </select>
        </label>
        <button class="btn" :disabled="loading" @click="refresh">
          {{ loading ? "Refreshing…" : "Refresh" }}
        </button>
      </div>
    </div>

    <p v-if="errorMessage" class="settings-message bad" role="alert">{{ errorMessage }}</p>
    <p v-else-if="statusLoading" class="collection-note">
      Loading status counts {{ loadedStatusCount }}/{{ items.length }}
    </p>
    <p v-else-if="statusError" class="collection-note" role="status">{{ statusError }}</p>

    <div class="poster-grid" :aria-busy="loading || statusLoading">
      <RouterLink
        v-for="item in sortedItems"
        :key="item.id"
        class="poster-card"
        :to="`/series/${encodeURIComponent(item.id)}`"
      >
        <div class="poster-media">
          <img
            v-if="!failedCovers.has(item.id)"
            :src="itemCoverURL(item.id)"
            :alt="`${item.name} cover`"
            loading="lazy"
            @error="markCoverFailed(item.id)"
          />
          <div v-else class="poster-fallback" aria-hidden="true">
            <span>{{ itemInitials(item.name) }}</span>
          </div>
          <div class="poster-status-counts" :aria-label="statusAriaLabel(item)">
            <span
              v-if="statusLoading && !itemStatuses[item.id] && !failedStatusItems.has(item.id)"
              class="poster-status-count is-loading"
            >
              Scanning…
            </span>
            <template v-else-if="itemStatuses[item.id]">
              <span
                v-for="status in statusEntries(item.id)"
                :key="status.key"
                class="poster-status-count"
                :class="`status-${status.key}`"
                :title="`${status.label}: ${status.count}`"
              >
                <span>{{ status.label }}</span>
                <strong>{{ status.count }}</strong>
              </span>
            </template>
            <span v-else class="poster-status-count status-total" title="Status counts unavailable">
              Total <strong>{{ item.episode_count }}</strong>
            </span>
          </div>
        </div>
        <div class="poster-copy">
          <h3>{{ item.name }}</h3>
          <p>
            {{ item.episode_count }} {{ item.episode_count === 1 ? "episode" : "episodes" }}
            <span aria-hidden="true">·</span>
            {{ itemDateLabel(item) }}
          </p>
        </div>
      </RouterLink>
      <p v-if="items.length === 0 && !loading" class="empty-msg">No items found.</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRoute } from "vue-router";
import {
  getItemStatus,
  listItems,
  listSources,
  type Item,
  type ItemStatusSummary,
} from "../api";

type SortMode = "updated-desc" | "added-desc" | "name-asc" | "episodes-desc" | "actionable-desc";

const STATUS_LOAD_CONCURRENCY = 2;

interface StatusEntry {
  key: string;
  label: string;
  count: number;
}

const route = useRoute();
const items = ref<Item[]>([]);
const sourceName = ref("");
const loading = ref(false);
const statusLoading = ref(false);
const errorMessage = ref("");
const statusError = ref("");
const sortMode = ref<SortMode>("updated-desc");
const itemStatuses = ref<Record<string, ItemStatusSummary>>({});
const failedStatusItems = ref(new Set<string>());
const failedCovers = ref(new Set<string>());
let refreshGeneration = 0;
let statusAbortController: AbortController | null = null;

const loadedStatusCount = computed(() => {
  return Object.keys(itemStatuses.value).length + failedStatusItems.value.size;
});

const sortedItems = computed(() => {
  const next = [...items.value];
  next.sort((a, b) => {
    let result = 0;
    switch (sortMode.value) {
      case "added-desc":
        result = timestamp(b.added_at) - timestamp(a.added_at);
        break;
      case "name-asc":
        result = a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });
        break;
      case "episodes-desc":
        result = b.episode_count - a.episode_count;
        break;
      case "actionable-desc":
        result = actionableCount(b.id) - actionableCount(a.id);
        break;
      default:
        result = timestamp(b.updated_at) - timestamp(a.updated_at);
    }
    return result || a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });
  });
  return next;
});

async function refresh() {
  statusAbortController?.abort();
  statusAbortController = null;
  const generation = ++refreshGeneration;
  loading.value = true;
  statusLoading.value = false;
  errorMessage.value = "";
  statusError.value = "";
  const sourceId = route.params.sourceId as string;
  let itemsLoaded = false;
  try {
    const [sources, nextItems] = await Promise.all([listSources(), listItems(sourceId)]);
    if (generation !== refreshGeneration) return;
    const source = sources.find((s) => s.id === sourceId);
    sourceName.value = source?.name || sourceId;
    items.value = nextItems;
    failedCovers.value = new Set();
    itemsLoaded = true;
  } catch (err) {
    if (generation === refreshGeneration) {
      errorMessage.value = `Failed to load titles: ${toErrorMessage(err)}`;
    }
  } finally {
    if (generation === refreshGeneration) {
      loading.value = false;
    }
  }

  if (!itemsLoaded || generation !== refreshGeneration) return;
  await loadStatusesProgressively(generation);
}

async function loadStatusesProgressively(generation: number) {
  const queue = [...sortedItems.value];
  const controller = new AbortController();
  statusAbortController = controller;
  statusLoading.value = true;
  itemStatuses.value = {};
  failedStatusItems.value = new Set();
  let cursor = 0;
  let failureCount = 0;

  async function worker() {
    while (!controller.signal.aborted) {
      const index = cursor;
      cursor += 1;
      if (index >= queue.length) return;

      const item = queue[index];
      try {
        const summary = await getItemStatus(item.id, controller.signal);
        if (generation !== refreshGeneration || controller.signal.aborted) return;
        itemStatuses.value = {
          ...itemStatuses.value,
          [summary.item_id]: summary,
        };
      } catch (err) {
        if (controller.signal.aborted || isAbortError(err) || generation !== refreshGeneration) return;
        const next = new Set(failedStatusItems.value);
        next.add(item.id);
        failedStatusItems.value = next;
        failureCount += 1;
      }
    }
  }

  const workers = Array.from(
    { length: Math.min(STATUS_LOAD_CONCURRENCY, queue.length) },
    () => worker(),
  );
  await Promise.all(workers);
  if (generation !== refreshGeneration || controller.signal.aborted) return;

  statusLoading.value = false;
  statusAbortController = null;
  if (failureCount > 0) {
    statusError.value = `Status counts are unavailable for ${failureCount} ${failureCount === 1 ? "title" : "titles"}.`;
  }
}

function timestamp(value: string) {
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? 0 : parsed;
}

function actionableCount(itemId: string) {
  const status = itemStatuses.value[itemId];
  if (!status) return 0;
  return status.idle + status.pending + status.translating + status.failed + status.skipped;
}

function statusEntries(itemId: string): StatusEntry[] {
  const status = itemStatuses.value[itemId];
  if (!status) return [];
  return [
    { key: "no-need", label: "No Need", count: status.no_need },
    { key: "idle", label: "Idle", count: status.idle },
    { key: "pending", label: "Pending", count: status.pending },
    { key: "translating", label: "Translating", count: status.translating },
    { key: "failed", label: "Failed", count: status.failed },
    { key: "skipped", label: "Skipped", count: status.skipped },
    { key: "no-source", label: "No Source", count: status.no_source },
  ].filter((entry) => entry.count > 0);
}

function statusAriaLabel(item: Item) {
  const entries = statusEntries(item.id);
  if (entries.length === 0) {
    return statusLoading.value ? `Scanning ${item.name} status counts` : `${item.episode_count} episodes`;
  }
  return entries.map((entry) => `${entry.label}: ${entry.count}`).join(", ");
}

function itemDateLabel(item: Item) {
  const showAdded = sortMode.value === "added-desc";
  const value = showAdded ? item.added_at : item.updated_at;
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime()) || parsed.getUTCFullYear() <= 1) {
    return "Date unavailable";
  }
  const label = showAdded ? "Added" : "Updated";
  return `${label} ${new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", year: "numeric" }).format(parsed)}`;
}

function itemCoverURL(itemId: string) {
  return `/api/library/items/${encodeURIComponent(itemId)}/cover`;
}

function markCoverFailed(itemId: string) {
  const next = new Set(failedCovers.value);
  next.add(itemId);
  failedCovers.value = next;
}

function itemInitials(name: string) {
  return name
    .split(/[^\p{L}\p{N}]+/u)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0])
    .join("")
    .toUpperCase() || "M";
}

function toErrorMessage(err: unknown): string {
  if (err instanceof Error && err.message.trim()) return err.message;
  return "unknown error";
}

function isAbortError(err: unknown) {
  return err instanceof DOMException && err.name === "AbortError";
}

onMounted(refresh);
onUnmounted(() => statusAbortController?.abort());
</script>
