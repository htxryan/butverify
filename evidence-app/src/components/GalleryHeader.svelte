<script lang="ts">
  import type { EvidenceManifest } from "../lib/manifest.js";
  import LayoutSwitcher from "./LayoutSwitcher.svelte";

  type Layout = "stacked" | "carousel";
  type Page = "evidence" | "details";

  let {
    manifest,
    layout = $bindable("stacked"),
    page,
    onNavigate,
  }: {
    manifest: EvidenceManifest;
    layout: Layout;
    page: Page;
    onNavigate: (p: Page) => void;
  } = $props();
</script>

<header class="bv-topbar" aria-label="Site navigation">
  <div class="bv-topbar-inner">
    {#if page === "details"}
      <button
        class="bv-topbar-back"
        type="button"
        onclick={() => onNavigate("evidence")}
        aria-label="Back to evidence"
      >
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="10,3 5,8 10,13" />
        </svg>
        Evidence
      </button>
    {/if}

    <h1 class="bv-topbar-title" class:bv-topbar-title--details={page === "details"}>
      {manifest.title}
    </h1>

    <div class="bv-topbar-controls">
      {#if page === "evidence"}
        <LayoutSwitcher bind:value={layout} />
        <button
          class="bv-topbar-nav"
          type="button"
          onclick={() => onNavigate("details")}
          aria-label="Details"
        >
          <!-- Info icon: always visible -->
          <svg class="bv-topbar-nav-icon" viewBox="0 0 16 16" width="15" height="15" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="8" cy="8" r="6" />
            <line x1="8" y1="7.5" x2="8" y2="11" />
            <circle cx="8" cy="5.25" r="0.5" fill="currentColor" stroke="none" />
          </svg>
          <!-- Text label: hidden on mobile -->
          <span class="bv-topbar-nav-label">Details</span>
        </button>
      {/if}
    </div>
  </div>
</header>

<style>
  .bv-topbar {
    position: sticky;
    top: 0;
    z-index: 20;
    width: 100%;
    background: var(--bv-surface);
    border-bottom: 1px solid var(--bv-border);
  }
  .bv-topbar-inner {
    max-width: var(--bv-max-content);
    margin: 0 auto;
    padding: 0 var(--bv-space-5);
    height: 3.25rem;
    display: flex;
    align-items: center;
    gap: var(--bv-space-3);
  }

  .bv-topbar-back {
    display: inline-flex;
    align-items: center;
    gap: var(--bv-space-1);
    font-size: var(--bv-text-sm);
    color: var(--bv-text-dim);
    background: transparent;
    border: none;
    cursor: pointer;
    padding: var(--bv-space-1) var(--bv-space-2);
    border-radius: var(--bv-radius-sm);
    flex-shrink: 0;
    transition: color var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-topbar-back:hover {
    color: var(--bv-text);
  }
  .bv-topbar-back:focus-visible {
    outline: 2px solid var(--bv-accent);
    outline-offset: 2px;
  }

  .bv-topbar-title {
    flex: 1;
    font-size: var(--bv-text-sm);
    font-weight: 600;
    color: var(--bv-text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }
  .bv-topbar-title--details {
    /* On the details page the title is in the page body — keep it in
     * the topbar as a breadcrumb label only, slightly muted. */
    color: var(--bv-text-muted);
    font-weight: 400;
  }

  .bv-topbar-controls {
    display: flex;
    align-items: center;
    gap: var(--bv-space-3);
    flex-shrink: 0;
  }

  .bv-topbar-nav {
    display: inline-flex;
    align-items: center;
    gap: var(--bv-space-1);
    font-size: var(--bv-text-sm);
    color: var(--bv-text-dim);
    background: transparent;
    border: 1px solid var(--bv-border);
    border-radius: var(--bv-radius-md);
    padding: var(--bv-space-1) var(--bv-space-3);
    cursor: pointer;
    transition:
      color var(--bv-duration-quick) var(--bv-ease-out),
      background var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-topbar-nav:hover {
    color: var(--bv-text);
    background: var(--bv-surface-2);
  }
  .bv-topbar-nav:focus-visible {
    outline: 2px solid var(--bv-accent);
    outline-offset: 2px;
  }

  @media (max-width: 639px) {
    .bv-topbar-nav {
      padding: var(--bv-space-1) var(--bv-space-2);
    }
    .bv-topbar-nav-label {
      display: none;
    }
  }
</style>
