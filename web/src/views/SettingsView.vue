<template>
  <section class="panel">
    <div class="panel-head">
      <h1>Settings</h1>
      <div class="row-gap">
        <button class="btn" :disabled="loading || saving" @click="reload">Reload</button>
        <button class="btn btn-primary" :disabled="loading || saving" @click="save">Save</button>
      </div>
    </div>

    <div class="settings-form">
      <label class="field">
        <span>LLM API URL</span>
        <input v-model.trim="form.llm_api_url" type="text" placeholder="https://example.com/v1" />
      </label>

      <label class="field">
        <span>LLM API Key</span>
        <input v-model.trim="form.llm_api_key" type="password" placeholder="sk-..." />
      </label>

      <label class="field">
        <span>Model</span>
        <input v-model.trim="form.llm_model" type="text" placeholder="openai/gpt-4o-mini" />
      </label>

      <label class="field">
        <span>Cron Expression</span>
        <input v-model.trim="form.cron_expr" type="text" placeholder="0 0 * * *" />
      </label>

      <label class="field">
        <span>Target Language</span>
        <select v-model="form.target_language">
          <option value="" disabled>Select language</option>
          <option value="zh">Chinese (zh)</option>
          <option value="en">English (en)</option>
          <option value="ja">Japanese (ja)</option>
          <option value="ko">Korean (ko)</option>
          <option value="fr">French (fr)</option>
          <option value="de">German (de)</option>
          <option value="es">Spanish (es)</option>
          <option value="pt">Portuguese (pt)</option>
          <option value="ru">Russian (ru)</option>
          <option value="it">Italian (it)</option>
          <option value="ar">Arabic (ar)</option>
          <option value="th">Thai (th)</option>
          <option value="vi">Vietnamese (vi)</option>
        </select>
      </label>
    </div>

    <p v-if="message" class="settings-message">{{ message }}</p>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { getSettings, updateSettings, type RuntimeSettings } from "../api";

const loading = ref(false);
const saving = ref(false);
const message = ref("");

const form = reactive<RuntimeSettings>({
  llm_api_url: "",
  llm_api_key: "",
  llm_model: "",
  cron_expr: "",
  target_language: ""
});

async function loadSettings() {
  loading.value = true;
  message.value = "";
  try {
    const settings = await getSettings();
    form.llm_api_url = settings.llm_api_url || "";
    form.llm_api_key = settings.llm_api_key || "";
    form.llm_model = settings.llm_model || "";
    form.cron_expr = settings.cron_expr || "";
    form.target_language = settings.target_language || "";
  } catch (err) {
    message.value = err instanceof Error ? err.message : "Failed to load settings";
  } finally {
    loading.value = false;
  }
}

async function reload() {
  await loadSettings();
}

async function save() {
  saving.value = true;
  message.value = "";
  try {
    const saved = await updateSettings({
      llm_api_url: form.llm_api_url,
      llm_api_key: form.llm_api_key,
      llm_model: form.llm_model,
      cron_expr: form.cron_expr,
      target_language: form.target_language
    });
    form.llm_api_url = saved.llm_api_url || "";
    form.llm_api_key = saved.llm_api_key || "";
    form.llm_model = saved.llm_model || "";
    form.cron_expr = saved.cron_expr || "";
    form.target_language = saved.target_language || "";
    message.value = "Settings saved";
  } catch (err) {
    message.value = err instanceof Error ? err.message : "Failed to save settings";
  } finally {
    saving.value = false;
  }
}

onMounted(loadSettings);
</script>
