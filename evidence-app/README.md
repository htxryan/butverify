# @butverify/evidence-app

Static bundle for the butverify.dev evidence gallery.

This package builds the JS + CSS that every published evidence site loads
from `https://<site>.butverify.dev/assets/evidence/v{semver}/_astro/main.{js,css}`.
The Go `bv` CLI (in `cmd/bv/`) generates the per-publish `index.html`,
inlines the manifest as `<script type="application/json"
id="evidence-manifest">…</script>`, and references the bundle assets by
the versioned CDN path. The CDN path is served by the customer-site
Worker — see
[`docs/2026-05-05-evidence-cdn-pipeline.md`](https://github.com/htxryan/butverify-service/blob/main/docs/2026-05-05-evidence-cdn-pipeline.md)
in the service repo.

## Stack

- **Vite 6** + **Svelte 5** — single-page bundle, predictable filenames,
  no per-page hashing (the version segment in the CDN path is the cache
  key). The marketing site uses Astro 5 for content-heavy pages; the
  evidence gallery is a single Svelte gallery hydrated from an inlined
  JSON manifest, so a stripped-down Vite build is the right tool. The
  output directory layout (`_astro/`) matches the marketing site's
  convention so operators inspecting R2 buckets see the same structure.

## Local dev

```bash
pnpm install
pnpm dev          # http://localhost:4321 with sample manifest in index.html
pnpm test         # vitest manifest parser tests
pnpm typecheck    # svelte-check + tsc --noEmit
pnpm build        # → dist/_astro/main.js + dist/_astro/main.css
pnpm preview      # serve dist/ locally
```

The dev preview reads the inlined sample manifest from `index.html` (the
same `<script id="evidence-manifest">` element the production CLI would
write). Identical boot path locally and in production.

## Build outputs

- `dist/_astro/main.js` — bundle entry, mounts the Svelte gallery into
  `#bv-gallery-root` after reading the manifest.
- `dist/_astro/main.css` — gallery styles + design tokens.
- `dist/index.html` — dev preview shell only; **not published**.

Filenames inside `_astro/` are predictable (no content hashes). The
version segment in the CDN path (e.g. `v1.0.0`) is the cache key.

## Publishing

CI workflow `.github/workflows/publish-evidence-bundle.yml` (in this
repo) builds this package and uploads `dist/_astro/*` to R2 at
`evidence-bundles/v{ver}/_astro/*` for the chosen environment. The
version comes from this package's `package.json`. Bump the version,
merge to `main`, then run the workflow against `dev` / `qa` / `prod`.

## Coupling to the Go CLI

The CLI hardcodes:

- Bundle version (matches this package's `version` field at CLI release
  time; the CLI's `EvidenceBundleVersion` constant lives in
  `pkg/templates/evidence.go`).
- Asset paths (`/assets/evidence/v{ver}/_astro/main.js` and `main.css`).
- Manifest schema (matches `pkg/templates/EvidenceInput`).

Cross-repo coordination: when bumping this package's version, also bump
`pkg/templates.EvidenceBundleVersion` in the same PR, and ensure the new
bundle is published to R2 BEFORE the CLI release ships.

## Components

- `Gallery.svelte` — root: layout switcher (stacked / carousel), header,
  layout host.
- `GalleryHeader.svelte` — title, subtitle, summary, issue/published metadata.
- `LayoutSwitcher.svelte` — stacked vs. carousel control with
  roving-tabindex keyboard nav.
- `StackedLayout.svelte` — vertical list of items.
- `CarouselLayout.svelte` — horizontal scroll-snap rail with prev/next
  controls + dot pagination + arrow-key nav.
- `GalleryItem.svelte` — single item: image (with annotation overlay
  shell) or video (with screen-reader fallback).
- `AnnotationOverlay.svelte` — visual shell only; interaction logic
  lands in epic E3 (pebble-4nwj).
- `ItemDetails.svelte` — collapsed `<details>` with per-item issue and
  properties.
- `EmptyState.svelte` — fallback for the (CLI-rejected) zero-items case.

## Accessibility

- Keyboard nav: arrow keys cycle the carousel and the layout switcher.
  Skip-to-content link at the top reveals on focus.
- `prefers-reduced-motion`: a blanket CSS rule disables transitions /
  animations; carousel scroll uses `behavior: auto` for users that
  prefer reduced motion (`prefersReducedMotion()` helper in
  `src/lib/manifest.ts`).
- Items render in DOM order regardless of carousel scroll position so
  screen readers traverse the gallery sequentially.
- Color contrast: text/text-dim/text-muted tokens hit WCAG AA against
  the bg/surface tokens in both light and dark themes.
