<script lang="ts">
  import type { EvidenceItem } from "../lib/manifest.js";
  let { item }: { item: EvidenceItem } = $props();

  let issueLabel = $derived(
    item.metadata?.issue_id ?? item.metadata?.issue_title ?? item.metadata?.issue_url ?? null,
  );
  let issueUrl = $derived(item.metadata?.issue_url ?? null);

  // Properties: render scalar values inline, object values in a folded
  // <details>. Sort alphabetically for stable display.
  let propertyEntries = $derived(
    item.properties
      ? Object.entries(item.properties).sort(([a], [b]) => a.localeCompare(b))
      : [],
  );

  function isScalar(v: unknown): v is string | number {
    return typeof v === "string" || typeof v === "number";
  }
</script>

{#if issueLabel || propertyEntries.length > 0}
  <details class="bv-item-details">
    <summary>More details</summary>
    {#if issueLabel}
      <dl class="bv-item-meta">
        <div class="bv-item-meta-row">
          <dt>Issue</dt>
          <dd>
            {#if issueUrl}
              <a href={issueUrl} rel="noreferrer noopener nofollow" target="_blank">{issueLabel}</a>
            {:else}
              {issueLabel}
            {/if}
          </dd>
        </div>
      </dl>
    {/if}
    {#if propertyEntries.length > 0}
      <dl class="bv-item-meta">
        {#each propertyEntries as [key, value]}
          <div class="bv-item-meta-row">
            <dt>{key}</dt>
            <dd>
              {#if isScalar(value)}
                {value}
              {:else}
                <pre class="bv-item-meta-json">{JSON.stringify(value, null, 2)}</pre>
              {/if}
            </dd>
          </div>
        {/each}
      </dl>
    {/if}
  </details>
{/if}

<style>
  .bv-item-details {
    border-top: 1px solid var(--bv-border);
    padding-top: var(--bv-space-3);
  }
  .bv-item-details summary {
    font-size: var(--bv-text-sm);
    color: var(--bv-text-muted);
    cursor: pointer;
    list-style: none;
    user-select: none;
    padding: var(--bv-space-1) 0;
  }
  .bv-item-details summary::marker,
  .bv-item-details summary::-webkit-details-marker {
    display: none;
  }
  .bv-item-details summary::before {
    content: "▸ ";
    color: var(--bv-text-muted);
    transition: transform var(--bv-duration-quick) var(--bv-ease-out);
    display: inline-block;
  }
  .bv-item-details[open] summary::before {
    content: "▾ ";
  }
  .bv-item-meta {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: var(--bv-space-2) var(--bv-space-4);
    margin: var(--bv-space-3) 0;
  }
  .bv-item-meta-row {
    display: contents;
  }
  .bv-item-meta-row dt {
    color: var(--bv-text-muted);
    font-size: var(--bv-text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    align-self: start;
    padding-top: 2px;
  }
  .bv-item-meta-row dd {
    margin: 0;
    color: var(--bv-text-dim);
    font-size: var(--bv-text-sm);
  }
  .bv-item-meta-json {
    font-family: var(--bv-font-mono);
    font-size: var(--bv-text-xs);
    background: var(--bv-surface-2);
    border: 1px solid var(--bv-border);
    border-radius: var(--bv-radius-sm);
    padding: var(--bv-space-2);
    margin: 0;
    overflow-x: auto;
  }
</style>
