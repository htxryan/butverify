<script lang="ts">
  /* Gallery shell: orchestrates layout switching, header, and item rendering.
   * Stateless re: review/annotation submission — those interactions land in
   * a later epic (pebble-4nwj). This component renders the visual shells
   * for annotation overlays so the design tokens and layout don't change
   * shape when interaction logic plugs in.
   */
  import StackedLayout from "./StackedLayout.svelte";
  import CarouselLayout from "./CarouselLayout.svelte";
  import GalleryHeader from "./GalleryHeader.svelte";
  import LayoutSwitcher from "./LayoutSwitcher.svelte";
  import EmptyState from "./EmptyState.svelte";
  import type { EvidenceManifest } from "../lib/manifest.js";

  type Layout = "stacked" | "carousel";

  let { manifest }: { manifest: EvidenceManifest } = $props();

  // Layout: read from URL hash (?layout=carousel) on first render; persist
  // to localStorage so a viewer's preference rides between visits. Default:
  // stacked (better for narrative flow on mobile + screen readers).
  function readInitialLayout(): Layout {
    if (typeof window === "undefined") return "stacked";
    const hash = new URLSearchParams(window.location.search).get("layout");
    if (hash === "carousel" || hash === "stacked") return hash;
    try {
      const stored = window.localStorage.getItem("bv-evidence-layout");
      if (stored === "carousel" || stored === "stacked") return stored;
    } catch {
      // localStorage unavailable (private browsing edge cases) — fall
      // through to default.
    }
    return "stacked";
  }

  let layout = $state<Layout>(readInitialLayout());

  $effect(() => {
    if (typeof window === "undefined") return;
    try {
      window.localStorage.setItem("bv-evidence-layout", layout);
    } catch {
      // no-op
    }
  });
</script>

<a href="#bv-gallery-content" class="bv-skip-link">Skip to gallery</a>

<div class="bv-gallery-shell">
  <GalleryHeader {manifest} />

  {#if manifest.items.length === 0}
    <EmptyState />
  {:else}
    <div class="bv-gallery-toolbar">
      <LayoutSwitcher bind:value={layout} />
    </div>

    <main id="bv-gallery-content" class="bv-gallery-content" tabindex="-1">
      {#if layout === "stacked"}
        <StackedLayout items={manifest.items} />
      {:else}
        <CarouselLayout items={manifest.items} />
      {/if}
    </main>
  {/if}
</div>

<style>
  .bv-gallery-shell {
    display: flex;
    flex-direction: column;
    min-height: 100dvh;
    background: var(--bv-bg);
    color: var(--bv-text);
  }

  .bv-gallery-toolbar {
    max-width: var(--bv-max-content);
    width: 100%;
    margin: 0 auto;
    padding: 0 var(--bv-space-5) var(--bv-space-3);
    display: flex;
    justify-content: flex-end;
  }

  .bv-gallery-content {
    flex: 1;
    width: 100%;
  }
  .bv-gallery-content:focus-visible {
    outline: 2px solid var(--bv-accent);
    outline-offset: -2px;
  }
</style>
