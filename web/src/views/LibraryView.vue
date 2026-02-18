<template>
  <section class="panel">
    <div class="panel-head">
      <h1>Media Library</h1>
      <button class="btn" @click="refresh">Refresh</button>
    </div>
    <div class="cards">
      <RouterLink
        v-for="source in sources"
        :key="source.id"
        class="item-card"
        :to="`/library/${source.id}`"
      >
        <h3>{{ source.name }}</h3>
        <p>{{ source.item_count }} items</p>
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

onMounted(refresh);
</script>
