<template>
  <div class="app-shell">
    <aside class="app-sidebar">
      <RouterLink class="brand" to="/library" aria-label="Contextual Subtitle Translator home">
        <span class="brand-mark" aria-hidden="true">CT</span>
        <span class="brand-copy">
          <strong>Contextual</strong>
          <span>Subtitle workspace</span>
        </span>
      </RouterLink>
      <nav class="side-nav">
        <RouterLink to="/library">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 5.5h6l1.6 2H20v11H4z" /></svg>
          <span>Library</span>
        </RouterLink>
        <RouterLink to="/jobs">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5 4h14v16H5zM8 8h8M8 12h8M8 16h5" /></svg>
          <span>Jobs</span>
        </RouterLink>
        <RouterLink to="/settings">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7ZM19 12l2-1-2-3-2 .5-1.5-1L15 5h-4l-.5 2.5-1.5 1L7 8l-2 3 2 1v2l-2 1 2 3 2-.5 1.5 1L11 21h4l.5-2.5 1.5-1 2 .5 2-3-2-1z" /></svg>
          <span>Settings</span>
        </RouterLink>
      </nav>
      <div class="sidebar-foot">
        <span class="connection-dot" aria-hidden="true"></span>
        Local workspace
      </div>
    </aside>
    <main class="content">
      <RouterView />
    </main>
  </div>
</template>

<script setup lang="ts"></script>
