// Bundle entry point. The Go CLI's generated index.html loads this script
// (from the CDN-versioned path) AFTER inlining a manifest <script
// type="application/json" id="evidence-manifest">…</script>. We:
//   1. Read the manifest from the inlined element.
//   2. Mount Gallery.svelte into #bv-gallery-root.
//   3. On any failure, render a minimal text fallback so the page is
//      never blank on a broken manifest (the CLI validated, but a
//      hand-edit during a hotfix attempt could break things).
//
// This is the only TS file Vite treats as an entry point — see
// astro.config.mjs.

import { mount } from "svelte";
import Gallery from "./components/Gallery.svelte";
import { ManifestParseError, readManifest } from "./lib/manifest.js";
import "./styles/tokens.css";
import "./styles/base.css";

function renderFallback(root: HTMLElement, message: string) {
  root.innerHTML = "";
  const wrap = document.createElement("div");
  wrap.style.maxWidth = "44rem";
  wrap.style.margin = "4rem auto";
  wrap.style.padding = "1rem 1.5rem";
  wrap.style.color = "var(--bv-text-dim, #4a5364)";
  wrap.style.fontFamily =
    "ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif";
  const h = document.createElement("h1");
  h.textContent = "Evidence gallery — manifest error";
  h.style.fontSize = "1.5rem";
  h.style.marginBottom = "0.75rem";
  h.style.color = "var(--bv-text, #11141b)";
  const p = document.createElement("p");
  p.textContent = message;
  wrap.appendChild(h);
  wrap.appendChild(p);
  root.appendChild(wrap);
}

function boot() {
  const root = document.getElementById("bv-gallery-root");
  if (!root) {
    // No mount point → CLI didn't render a shell, or someone is loading
    // this script outside the gallery context. Nothing to do.
    return;
  }
  try {
    const manifest = readManifest(document);
    mount(Gallery, { target: root, props: { manifest } });
  } catch (err) {
    const msg =
      err instanceof ManifestParseError
        ? err.message
        : err instanceof Error
          ? err.message
          : "unknown error";
    // eslint-disable-next-line no-console
    console.error("[butverify-evidence] manifest boot failed:", err);
    renderFallback(root, msg);
  }
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", boot, { once: true });
} else {
  boot();
}
