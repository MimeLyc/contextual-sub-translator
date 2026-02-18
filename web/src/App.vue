<template>
  <div class="app-shell">
    <aside class="app-sidebar">
      <div class="brand">Contextual Subtitle Translator</div>
      <nav class="side-nav">
        <RouterLink to="/library">Library</RouterLink>
        <RouterLink to="/jobs">Jobs</RouterLink>
        <RouterLink to="/settings">Settings</RouterLink>
      </nav>
    </aside>
    <main class="content">
      <RouterView />
    </main>
  </div>
</template>

<script setup lang="ts"></script>
