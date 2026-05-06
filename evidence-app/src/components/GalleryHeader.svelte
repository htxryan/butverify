<script lang="ts">
  import type { EvidenceManifest } from "../lib/manifest.js";

  let { manifest }: { manifest: EvidenceManifest } = $props();

  function formatDate(iso: string | undefined): string | null {
    if (!iso) return null;
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return null;
    // YYYY-MM-DD HH:MM UTC. Stable across locales for proof artifacts.
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
</script>

<header class="bv-gallery-header" aria-label="Evidence gallery title">
  <div class="bv-gallery-header-inner">
    <h1 class="bv-gallery-title">{manifest.title}</h1>
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
  </div>
</header>

<style>
  .bv-gallery-header {
    width: 100%;
    border-bottom: 1px solid var(--bv-border);
    background: var(--bv-surface);
  }
  .bv-gallery-header-inner {
    max-width: var(--bv-max-content);
    margin: 0 auto;
    padding: var(--bv-space-8) var(--bv-space-5) var(--bv-space-6);
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-3);
  }
  .bv-gallery-title {
    font-size: var(--bv-text-3xl);
    line-height: var(--bv-line-tight);
    color: var(--bv-text);
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

  @media (min-width: 720px) {
    .bv-gallery-title {
      font-size: var(--bv-text-4xl);
    }
  }
</style>
