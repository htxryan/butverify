<script lang="ts">
  import type { EvidenceManifest } from "../lib/manifest.js";

  let { manifest }: { manifest: EvidenceManifest } = $props();

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
</script>

<main class="bv-meta-page" id="bv-meta-content" tabindex="-1">
  <div class="bv-meta-inner">
    <hgroup class="bv-meta-hgroup">
      <h1 class="bv-meta-title">{manifest.title}</h1>
      {#if manifest.subtitle}
        <p class="bv-meta-subtitle">{manifest.subtitle}</p>
      {/if}
    </hgroup>

    {#if manifest.summary}
      <p class="bv-meta-summary">{manifest.summary}</p>
    {/if}

    <dl class="bv-meta-grid">
      {#if issueLabel}
        <div class="bv-meta-row">
          <dt>Issue</dt>
          <dd>
            {#if issueUrl}
              <a href={issueUrl} rel="noreferrer noopener nofollow" target="_blank">{issueLabel}</a>
            {:else}
              {issueLabel}
            {/if}
          </dd>
        </div>
      {/if}
      {#if publishedAt}
        <div class="bv-meta-row">
          <dt>Published</dt>
          <dd>{publishedAt}</dd>
        </div>
      {/if}
      {#if manifest.generator_version}
        <div class="bv-meta-row">
          <dt>Generator</dt>
          <dd>bv {manifest.generator_version}</dd>
        </div>
      {/if}
      {#if manifest.bundle_version}
        <div class="bv-meta-row">
          <dt>Bundle</dt>
          <dd>v{manifest.bundle_version}</dd>
        </div>
      {/if}
      <div class="bv-meta-row">
        <dt>Items</dt>
        <dd>{manifest.items.length}</dd>
      </div>
    </dl>

    {#if manifest.items.length > 0}
      <section class="bv-meta-items">
        <h2 class="bv-meta-items-heading">Evidence items</h2>
        <ol class="bv-meta-item-list">
          {#each manifest.items as item, i}
            <li class="bv-meta-item">
              <span class="bv-meta-item-num">{(i + 1).toString().padStart(2, "0")}</span>
              <div class="bv-meta-item-body">
                <strong class="bv-meta-item-title">{item.title ?? `Item ${i + 1}`}</strong>
                {#if item.description}
                  <p class="bv-meta-item-desc">{item.description}</p>
                {/if}
              </div>
            </li>
          {/each}
        </ol>
      </section>
    {/if}
  </div>
</main>

<style>
  .bv-meta-page {
    flex: 1;
    width: 100%;
  }
  .bv-meta-inner {
    max-width: var(--bv-max-narrow);
    margin: 0 auto;
    padding: var(--bv-space-8) var(--bv-space-5) var(--bv-space-10);
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-6);
  }

  .bv-meta-hgroup {
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-2);
  }
  .bv-meta-title {
    font-size: var(--bv-text-3xl);
    line-height: var(--bv-line-tight);
    color: var(--bv-text);
  }
  .bv-meta-subtitle {
    font-size: var(--bv-text-lg);
    color: var(--bv-text-dim);
    line-height: var(--bv-line-snug);
  }
  .bv-meta-summary {
    font-size: var(--bv-text-base);
    color: var(--bv-text-dim);
    line-height: var(--bv-line-normal);
    white-space: pre-wrap;
    margin: 0;
  }

  .bv-meta-grid {
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-3);
    padding: var(--bv-space-5);
    background: var(--bv-surface);
    border: 1px solid var(--bv-border);
    border-radius: var(--bv-radius-lg);
  }
  .bv-meta-row {
    display: flex;
    align-items: baseline;
    gap: var(--bv-space-3);
  }
  .bv-meta-row dt {
    font-size: var(--bv-text-xs);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--bv-text-muted);
    min-width: 6rem;
    flex-shrink: 0;
  }
  .bv-meta-row dd {
    margin: 0;
    font-family: var(--bv-font-mono);
    font-size: var(--bv-text-sm);
    color: var(--bv-text-dim);
  }
  .bv-meta-row dd a {
    color: var(--bv-accent);
    text-decoration: none;
  }
  .bv-meta-row dd a:hover {
    text-decoration: underline;
  }

  .bv-meta-items {
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-4);
  }
  .bv-meta-items-heading {
    font-size: var(--bv-text-sm);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--bv-text-muted);
  }
  .bv-meta-item-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0;
    border: 1px solid var(--bv-border);
    border-radius: var(--bv-radius-lg);
    overflow: hidden;
  }
  .bv-meta-item {
    display: flex;
    align-items: flex-start;
    gap: var(--bv-space-4);
    padding: var(--bv-space-4) var(--bv-space-5);
    border-bottom: 1px solid var(--bv-border);
  }
  .bv-meta-item:last-child {
    border-bottom: 0;
  }
  .bv-meta-item-num {
    font-family: var(--bv-font-mono);
    font-size: var(--bv-text-xs);
    color: var(--bv-text-muted);
    letter-spacing: 0.04em;
    margin-top: 3px;
    flex-shrink: 0;
  }
  .bv-meta-item-body {
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-1);
    min-width: 0;
  }
  .bv-meta-item-title {
    font-size: var(--bv-text-base);
    color: var(--bv-text);
    font-weight: 500;
  }
  .bv-meta-item-desc {
    margin: 0;
    font-size: var(--bv-text-sm);
    color: var(--bv-text-dim);
    line-height: var(--bv-line-normal);
  }

  @media (min-width: 720px) {
    .bv-meta-title {
      font-size: var(--bv-text-4xl);
    }
  }
</style>
