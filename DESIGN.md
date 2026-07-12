# Design

## Source of truth
- Status: Active
- Last refreshed: 2026-07-12
- Primary product surfaces: Library sources, media collections, episode table, translation jobs, runtime settings.
- Evidence reviewed: `README.md`, `web/src/App.vue`, `web/src/styles.css`, `web/src/views/*.vue`, `web/src/api.ts`, `internal/library/*`, `internal/httpapi/*`, the existing Jellyfin-style implementation plan, and the deployed local NAS UI/API.
- External reference: Capacities' official product interface, used as directional inspiration for content hierarchy, quiet navigation, media-rich objects, and restrained chrome. This is not a pixel-match target.

## Brand
- Personality: Calm, precise, editorial, quietly capable.
- Trust signals: Explicit queue state, immediate selection feedback, visible progress, readable media metadata, and reversible editing.
- Avoid: Green-tinted surfaces, large saturated navigation blocks, ornamental gradients, ambiguous status labels, dense card-within-card layouts, and hidden loading.

## Product goals
- Goals: Make it obvious what media exists, what needs translation, what is queued or running, and what completed; keep common manual-translation actions fast and predictable.
- Non-goals: Reproduce Jellyfin or Capacities exactly; become a general media player; introduce a separate design-system dependency.
- Success signals: Users can identify a title from its cover, distinguish Idle from Pending at a glance, see a selected job immediately, and never receive translated subtitle text containing HTML markup.

## Personas and jobs
- Primary personas: A self-hosting media-library owner operating the service on a NAS.
- User jobs: Browse media folders, find untranslated episodes, enqueue selected episodes, monitor progress, inspect or correct output, and adjust runtime settings.
- Key contexts of use: Desktop browser on a local network, intermittent NAS latency, long-running background translation, occasional tablet/mobile access.

## Information architecture
- Primary navigation: Library, Jobs, Settings in a persistent left rail; compact bottom/row navigation on narrow screens.
- Core routes/screens: `/library`, `/library/:sourceId`, `/series/:itemId`, `/jobs`, `/settings`.
- Content hierarchy: Breadcrumb (`Library / source / title` where applicable) -> primary action -> core collection/table -> contextual status/details -> secondary metadata.

## Design principles
- Principle 1: State is a product contract. Use mutually exclusive, deterministic labels and never use Pending as a generic empty state.
- Principle 2: Selection responds immediately. Show the selected entity synchronously, then progressively load expensive details without replacing it with an empty state.
- Principle 3: Media is recognizable before it is readable. Prefer folder poster artwork, then a stable typographic fallback.
- Principle 4: Quiet shell, expressive content. Navigation and surfaces stay neutral; accent color is reserved for focus, primary actions, and current selection.
- Tradeoffs: Favor clear tables and compact metadata over cinematic poster scale; allow semantic red/amber for failure/warning while avoiding pervasive status color.

## Visual language
- Color: Warm off-white canvas, paper-white surfaces, graphite text, cool gray borders, muted violet-blue accent. No green cast. Success uses restrained blue/violet rather than green.
- Typography: Native sans-serif stack with strong editorial hierarchy; tabular numerals for job IDs, counts, and progress.
- Spacing/layout rhythm: 4/8px base rhythm; 16-24px page spacing; max readable content width while jobs may use the full viewport.
- Shape/radius/elevation: 8-14px radii, hairline borders, minimal elevation only on hover/active overlays.
- Motion: 120-180ms color/transform transitions; progress transitions may be linear; no decorative motion.
- Imagery/iconography: Folder poster/cover images use a 2:3 crop. Monochrome line icons and typographic fallbacks remain legible without images.

## Components
- Existing components to reuse: Vue route views, `.panel`, `.panel-head`, `.btn`, `.chip`, message, progress, and job-detail patterns.
- New/changed components: Branded app rail, source tiles, poster cards with image fallback and vertically stacked per-status counters, collection sort control, three-level breadcrumbs, semantic episode table, select-all control, job detail loading shell, skeleton rows.
- Variants and states: Buttons (secondary/primary/disabled), chips and poster counters (idle/pending/running/no-need/failed/skipped/no-source), poster (loading/image/fallback/status-unavailable), detail (selected-loading/loaded/error).
- Token/component ownership: Global CSS custom properties in `web/src/styles.css`; route-specific markup stays in the owning Vue view. Do not add a component library.

## Accessibility
- Target standard: WCAG 2.1 AA for contrast, focus visibility, keyboard operation, and semantic structure.
- Keyboard/focus behavior: Every navigation/action is reachable; table checkboxes have labels; clickable job rows are buttons or keyboard-operable; `:focus-visible` is always visible.
- Contrast/readability: Body text and status text meet AA; status never relies on color alone.
- Screen-reader semantics: Real tables for episode data, table captions or accessible headings, `aria-live` for async messages, `aria-busy` for loading detail.
- Reduced motion and sensory considerations: Disable non-essential transitions under `prefers-reduced-motion`.

## Responsive behavior
- Supported breakpoints/devices: Desktop >= 1024px, compact/tablet 700-1023px, narrow/mobile < 700px.
- Layout adaptations: Jobs split pane collapses to one column; poster grids reduce columns; episode table becomes horizontally scrollable without dropping status meaning; app rail becomes a compact top bar.
- Touch/hover differences: Minimum 40px action targets; hover is enhancement only; visible active state persists after click.

## Interaction states
- Loading: Preserve page structure; use inline skeletons/spinners and `aria-busy`, never a misleading empty state. Collection posters render before expensive status scans; cards update independently in current sort order as each item finishes instead of waiting for the entire source.
- Empty: Explain what is absent and, when applicable, offer Refresh.
- Error: Keep the last usable content, show a concise inline error, and permit retry.
- Success: Confirm enqueue/save with counts and immediately reconcile local state with the server.
- Disabled: Explain through nearby state/copy; translated, missing-source, pending, and running rows cannot be selected.
- Offline/slow network, if applicable: Selected job summary appears immediately; expensive detail arrives progressively; SSE disconnect does not erase current content.

## Content voice
- Tone: Short, factual, calm.
- Terminology: `No Need` = target subtitle exists or job completed successfully; `Idle` = translatable and absent from Jobs; `Pending` = present in Jobs but not started; `Translating` = running; `Failed`/`Skipped` preserve terminal job outcome; `No Source` = no usable source subtitle.
- Microcopy rules: Sentence case; use `Translate N selected`; avoid internal terms such as dedupe keys in primary UI; show exact job IDs only as secondary metadata.

## Implementation constraints
- Framework/styling system: Vue 3, Vue Router, Vite, TypeScript, and plain CSS; Go `net/http` backend.
- Design-token constraints: Extend the existing root CSS variables; replace green palette in place; no parallel token layer.
- Performance constraints: Poster discovery is bounded to the item root; cover responses validate paths under configured media roots; job detail must avoid scanning the entire media library; collection status summaries use per-item requests with a two-item client concurrency limit, prioritize the current sort order, and cancel obsolete scans on refresh/navigation.
- Library time semantics: `Added` is the earliest media-file modification time within a title; `Updated` is the latest relevant media, subtitle, artwork, or NFO modification time. These are portable filesystem-derived proxies because the service does not persist library-ingestion history.
- Compatibility constraints: Preserve existing JSON fields and routes where practical; NAS deployment remains a single container; cover absence must not break the collection page.
- Test/screenshot expectations: Add Go regression tests for cover selection/path safety, episode job-state mapping, fast media-path description, and HTML sanitization; run Go tests plus the Vite production build.

## Open questions
- [ ] Whether users prefer localized UI copy / owner / impacts future i18n only; current UI remains English for consistency.
- [ ] Whether movie backdrops should be supported in addition to poster/folder images / owner / impacts future gallery variants, not this release.
- [ ] Whether completed/skipped jobs should be archivable from the Jobs page / owner / impacts future queue management, not current status semantics.
