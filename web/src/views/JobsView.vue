<template>
  <section class="panel">
    <div class="panel-head">
      <h1>Jobs</h1>
      <div class="row-gap">
        <button class="btn" @click="refresh">Refresh</button>
      </div>
    </div>

    <div class="job-list">
      <article v-for="job in jobs" :key="job.id" class="job-row">
        <div>
          <div class="job-id">{{ job.id }}</div>
          <div class="job-key">{{ job.dedupe_key }}</div>
        </div>
        <div class="chips">
          <span class="chip" :class="statusClass(job.status)">{{ job.status }}</span>
          <span v-if="job.error" class="chip bad">{{ job.error }}</span>
        </div>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { listJobs, type Job } from "../api";

const jobs = ref<Job[]>([]);
let evt: EventSource | null = null;

function statusClass(status: string) {
  if (status === "success") return "ok";
  if (status === "running" || status === "pending") return "warn";
  return "bad";
}

async function refresh() {
  jobs.value = await listJobs();
}

onMounted(async () => {
  await refresh();
  evt = new EventSource("/api/jobs/stream");
  evt.onmessage = (event) => {
    try {
      jobs.value = JSON.parse(event.data) as Job[];
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
