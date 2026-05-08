import { svelte } from "@sveltejs/vite-plugin-svelte";
import { defineConfig } from "vite";

// Vite + Svelte build for the evidence gallery bundle.
//
// Output:
//   dist/index.html         — dev/preview shell. NOT published; the Go
//                             CLI generates per-publish index.html with
//                             the manifest JSON inlined.
//   dist/_astro/main.js     — bundle entry (Svelte gallery), predictable
//                             filename so the Go CLI's index.html can
//                             hardcode `<script src="..._astro/main.js">`.
//   dist/_astro/main.css    — gallery styles + design tokens.
//
// Why "_astro/" and not "assets/"? Two reasons:
//   1. The customer-site Worker reserves the per-site path
//      `/<path>/` for the tenant's content. Putting bundle assets at
//      `_astro/` keeps the bundle namespace distinct from anything a
//      tenant might publish (collisions are highly unlikely either way).
//   2. The marketing-site (also Astro 5) emits to `_astro/`; matching
//      the convention keeps both surfaces familiar to operators
//      inspecting R2 buckets in the dashboard.
//
// The CDN-versioned path (e.g. `/assets/evidence/v1.0.0/_astro/main.js`)
// is the cache key. Files inside a version directory are immutable by
// construction, so we drop content hashes.

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2022",
    cssCodeSplit: false,
    rollupOptions: {
      // Vite picks up `index.html` automatically as the HTML entry; the
      // <script type="module" src="/src/main.ts"> inside it becomes the
      // JS entry, which we name `main` via the chunk-naming hooks below.
      output: {
        entryFileNames: "_astro/main.js",
        chunkFileNames: "_astro/[name].js",
        // Single CSS bundle gets the predictable `main.css` name; other
        // assets keep their original names so they can be referenced
        // from JS by import path.
        assetFileNames: (info) => {
          const name = info.name ?? "asset";
          if (name.endsWith(".css")) return "_astro/main.css";
          return "_astro/[name][extname]";
        },
      },
    },
  },
  server: {
    port: 4321,
    open: false,
  },
});
