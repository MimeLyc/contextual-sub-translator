import { createRouter, createWebHistory } from "vue-router";
import LibraryView from "./views/LibraryView.vue";
import SourceView from "./views/SourceView.vue";
import SeriesView from "./views/SeriesView.vue";
import JobsView from "./views/JobsView.vue";
import SettingsView from "./views/SettingsView.vue";

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", redirect: "/library" },
    { path: "/library", component: LibraryView },
    { path: "/library/:sourceId", component: SourceView, props: true },
    { path: "/series/:itemId", component: SeriesView, props: true },
    { path: "/jobs", component: JobsView },
    { path: "/settings", component: SettingsView }
  ]
});

export default router;
