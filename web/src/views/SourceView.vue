<template>
  <section class="panel">
    <div class="panel-head">
      <div class="breadcrumb">
        <RouterLink to="/library" class="breadcrumb-link">Library</RouterLink>
        <span class="breadcrumb-sep">/</span>
        <span>{{ sourceName }}</span>
      </div>
      <button class="btn" @click="refresh">Refresh</button>
    </div>
    <div class="cards">
      <RouterLink
        v-for="item in items"
        :key="item.id"
        class="item-card"
        :to="`/series/${encodeURIComponent(item.id)}`"
      >
        <h3>{{ item.name }}</h3>
        <p>{{ item.episode_count }} episodes</p>
      </RouterLink>
      <p v-if="items.length === 0 && !loading" class="empty-msg">No items found.</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import { listItems, listSources, type Item } from "../api";

const route = useRoute();
const items = ref<Item[]>([]);
const sourceName = ref("");
const loading = ref(false);

async function refresh() {
  loading.value = true;
  const sourceId = route.params.sourceId as string;
  try {
    const sources = await listSources();
    const source = sources.find((s) => s.id === sourceId);
    sourceName.value = source?.name || sourceId;
    items.value = await listItems(sourceId);
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);
</script>
