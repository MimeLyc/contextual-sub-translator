<template>
  <section class="panel">
    <div class="panel-head">
      <div class="title-block">
        <span class="eyebrow">Workspace</span>
        <h1>Media library</h1>
        <p>Choose a source to browse its titles and translation coverage.</p>
      </div>
      <button class="btn" @click="refresh">Refresh</button>
    </div>
    <div class="source-grid">
      <RouterLink
        v-for="source in sources"
        :key="source.id"
        class="source-card"
        :to="`/library/${source.id}`"
      >
        <span class="source-icon" aria-hidden="true">{{ sourceInitials(source.name) }}</span>
        <span class="source-copy">
          <strong>{{ source.name }}</strong>
          <span>{{ source.item_count }} {{ source.item_count === 1 ? "title" : "titles" }}</span>
        </span>
        <span class="card-arrow" aria-hidden="true">→</span>
      </RouterLink>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { listSources, type Source } from "../api";

const sources = ref<Source[]>([]);

async function refresh() {
  sources.value = await listSources();
}

function sourceInitials(name: string) {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0])
    .join("")
    .toUpperCase() || "M";
}

onMounted(refresh);
</script>
