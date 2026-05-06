<script lang="ts">
  import type { EvidenceManifest } from "../lib/manifest.js";
  import LayoutSwitcher from "./LayoutSwitcher.svelte";

  type Layout = "stacked" | "carousel";

  let {
    manifest,
    layout = $bindable("stacked"),
  }: { manifest: EvidenceManifest; layout: Layout } = $props();

  function formatDate(iso: string | undefined): string | null {
    if (!iso) return null;
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return null;
    const pad = (n: number) => n.toString().padStart(2, "0");
    return (
      `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())}` +
      ` ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())} UTC`
    );
  }

  let issueLabel = $derived(
    manifest.metadata?.issue_id ??
      manifest.metadata?.issue_title ??
      manifest.metadata?.issue_url ??
      null,
  );
  let issueUrl = $derived(manifest.metadata?.issue_url ?? null);
  let publishedAt = $derived(formatDate(manifest.generated_at));

  // Collapse state — persisted so it survives navigation.
  let collapsed = $state(false);
  $effect(() => {
    try {
      collapsed = localStorage.getItem("bv-header-collapsed") === "true";
    } catch {
      // localStorage unavailable — use default
    }
  });

  function toggleCollapsed() {
    collapsed = !collapsed;
    try {
      localStorage.setItem("bv-header-collapsed", String(collapsed));
    } catch {}
  }
</script>

<header
  class="bv-gallery-header"
  class:bv-gallery-header--collapsed={collapsed}
  aria-label="Evidence gallery title"
>
  <div class="bv-gallery-header-inner">
    <!-- Title row: always visible -->
    <div class="bv-gallery-header-titlebar">
      <h1 class="bv-gallery-title">{manifest.title}</h1>
      <div class="bv-gallery-header-controls">
        {#if collapsed}
          <LayoutSwitcher bind:value={layout} />
        {/if}
        <button
          class="bv-header-toggle"
          type="button"
          onclick={toggleCollapsed}
          aria-label={collapsed ? "Expand header" : "Collapse header"}
          aria-expanded={!collapsed}
        >
          <svg
            viewBox="0 0 16 16"
            width="14"
            height="14"
            aria-hidden="true"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            class:bv-chevron-collapsed={collapsed}
          >
            <polyline points="3,10 8,5 13,10" />
          </svg>
        </button>
      </div>
    </div>

    <!-- Expandable body -->
    {#if !collapsed}
      {#if manifest.subtitle}
        <p class="bv-gallery-subtitle">{manifest.subtitle}</p>
      {/if}
      {#if manifest.summary}
        <p class="bv-gallery-summary">{manifest.summary}</p>
      {/if}
      {#if issueLabel || publishedAt || manifest.generator_version}
        <dl class="bv-gallery-meta">
          {#if issueLabel}
            <div class="bv-gallery-meta-row">
              <dt>Issue</dt>
              <dd>
                {#if issueUrl}
                  <a href={issueUrl} rel="noreferrer noopener nofollow" target="_blank"
                    >{issueLabel}</a
                  >
                {:else}
                  {issueLabel}
                {/if}
              </dd>
            </div>
          {/if}
          {#if publishedAt}
            <div class="bv-gallery-meta-row">
              <dt>Published</dt>
              <dd>{publishedAt}</dd>
            </div>
          {/if}
          {#if manifest.generator_version}
            <div class="bv-gallery-meta-row">
              <dt>Generator</dt>
              <dd>bv {manifest.generator_version}</dd>
            </div>
          {/if}
        </dl>
      {/if}
      <!-- Toolbar lives inside the expanded header -->
      <div class="bv-gallery-header-toolbar">
        <LayoutSwitcher bind:value={layout} />
      </div>
    {/if}
  </div>
</header>

<style>
  .bv-gallery-header {
    width: 100%;
    border-bottom: 1px solid var(--bv-border);
    background: var(--bv-surface);
  }
  .bv-gallery-header--collapsed {
    position: sticky;
    top: 0;
    z-index: 20;
  }
  .bv-gallery-header-inner {
    max-width: var(--bv-max-content);
    margin: 0 auto;
    padding: var(--bv-space-8) var(--bv-space-5) var(--bv-space-6);
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-3);
  }
  .bv-gallery-header--collapsed .bv-gallery-header-inner {
    padding: var(--bv-space-3) var(--bv-space-5);
  }

  .bv-gallery-header-titlebar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--bv-space-3);
    min-width: 0;
  }
  .bv-gallery-header-controls {
    display: flex;
    align-items: center;
    gap: var(--bv-space-3);
    flex-shrink: 0;
  }

  .bv-gallery-title {
    font-size: var(--bv-text-3xl);
    line-height: var(--bv-line-tight);
    color: var(--bv-text);
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .bv-gallery-header--collapsed .bv-gallery-title {
    font-size: var(--bv-text-lg);
  }
  .bv-gallery-subtitle {
    font-size: var(--bv-text-lg);
    color: var(--bv-text-dim);
    line-height: var(--bv-line-snug);
  }
  .bv-gallery-summary {
    font-size: var(--bv-text-base);
    color: var(--bv-text-dim);
    line-height: var(--bv-line-normal);
    white-space: pre-wrap;
    max-width: var(--bv-max-narrow);
  }
  .bv-gallery-meta {
    margin: var(--bv-space-2) 0 0;
    display: flex;
    flex-wrap: wrap;
    gap: var(--bv-space-2) var(--bv-space-5);
  }
  .bv-gallery-meta-row {
    display: flex;
    align-items: center;
    gap: var(--bv-space-2);
    font-size: var(--bv-text-sm);
  }
  .bv-gallery-meta-row dt {
    color: var(--bv-text-muted);
    text-transform: uppercase;
    font-size: var(--bv-text-xs);
    letter-spacing: 0.04em;
  }
  .bv-gallery-meta-row dd {
    margin: 0;
    color: var(--bv-text-dim);
    font-family: var(--bv-font-mono);
    font-size: var(--bv-text-xs);
  }

  .bv-gallery-header-toolbar {
    margin-top: var(--bv-space-2);
    display: flex;
    justify-content: flex-end;
  }

  .bv-header-toggle {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2rem;
    height: 2rem;
    border-radius: var(--bv-radius-md);
    background: transparent;
    border: 1px solid var(--bv-border);
    color: var(--bv-text-muted);
    cursor: pointer;
    flex-shrink: 0;
    transition:
      background var(--bv-duration-quick) var(--bv-ease-out),
      color var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-header-toggle:hover {
    background: var(--bv-surface-2);
    color: var(--bv-text);
  }
  .bv-header-toggle:focus-visible {
    outline: 2px solid var(--bv-accent);
    outline-offset: 2px;
  }

  .bv-chevron-collapsed {
    transform: rotate(180deg);
  }

  @media (min-width: 720px) {
    .bv-gallery-title {
      font-size: var(--bv-text-4xl);
    }
    .bv-gallery-header--collapsed .bv-gallery-title {
      font-size: var(--bv-text-xl);
    }
  }
</style>
