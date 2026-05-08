<script lang="ts">
  /* pebble-60yj — Reviews page.
   *
   * Lists the viewer's reviews on this site, grouped by status:
   *   - Pending      (current local drafts, not yet submitted)
   *   - Submitted    (server-side rows, not yet acknowledged)
   *   - Acknowledged (server-side rows, acknowledged by the publisher)
   *
   * Each card shows a status badge, a timestamp (last-edited / submitted_at /
   * acknowledged_at), and a count of annotations. The (site_id,
   * reviewer_login) UNIQUE constraint means the server lists at most one
   * row, and pending is at most one (the in-flight draft set), so this
   * page renders 0..3 cards in steady state — but the structure is
   * forward-compatible if the spec ever admits multiple submitted
   * reviews per site.
   */
  import type { AnnotationDraft } from "../lib/annotations.js";
  import type { ViewerReview } from "../lib/review-list.js";

  interface PendingDraftSummary {
    count: number;
    // Latest add or remove timestamp the store could surface; we don't
    // currently track this at the draft level, so the page falls back
    // to "Just now" when the count is non-zero. Kept as an optional
    // ISO string so future versions of the draft store can plumb a
    // `last_modified_at` through without reshaping this component.
    last_edited_at?: string | null;
  }

  let {
    drafts,
    pastReviews,
    listError,
    listLoading,
    onBack,
  }: {
    drafts: AnnotationDraft[];
    pastReviews: ViewerReview[];
    listError: string | null;
    listLoading: boolean;
    onBack: () => void;
  } = $props();

  const pendingSummary: PendingDraftSummary = $derived({
    count: drafts.length,
    last_edited_at: null,
  });

  const submittedReviews = $derived(
    pastReviews.filter((r) => r.status === "submitted"),
  );
  const acknowledgedReviews = $derived(
    pastReviews.filter((r) => r.status === "acknowledged"),
  );

  function formatTimestamp(iso: string): string {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
  }

  function pluralize(n: number, one: string, many: string): string {
    return n === 1 ? `${n} ${one}` : `${n} ${many}`;
  }
</script>

<section class="bv-reviews-page" aria-labelledby="bv-reviews-title">
  <header class="bv-reviews-header">
    <button
      type="button"
      class="bv-reviews-back"
      onclick={onBack}
      aria-label="Back to evidence gallery"
    >
      <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" fill="none"
        stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
        <path d="M10 3 5 8l5 5" />
      </svg>
      <span>Back</span>
    </button>
    <h1 id="bv-reviews-title">Your reviews on this site</h1>
  </header>

  {#if listLoading}
    <p class="bv-reviews-status" role="status">Loading reviews…</p>
  {:else if listError}
    <p class="bv-reviews-status bv-reviews-status--error" role="alert">{listError}</p>
  {/if}

  <!-- Pending -->
  <section class="bv-reviews-section" aria-labelledby="bv-reviews-pending">
    <h2 id="bv-reviews-pending" class="bv-reviews-section-title">Pending</h2>
    {#if pendingSummary.count === 0}
      <p class="bv-reviews-empty">No pending annotations.</p>
    {:else}
      <article class="bv-review-card bv-review-card--pending" aria-label="Pending review">
        <div class="bv-review-card-row">
          <span class="bv-status-badge bv-status-badge--pending" aria-label="Status: pending">
            <span class="bv-pending-dot" aria-hidden="true"></span>
            Pending
          </span>
          <span class="bv-review-card-time">
            {pendingSummary.last_edited_at
              ? `Last edited ${formatTimestamp(pendingSummary.last_edited_at)}`
              : "Awaiting submission"}
          </span>
        </div>
        <div class="bv-review-card-meta">
          {pluralize(pendingSummary.count, "annotation", "annotations")}
        </div>
      </article>
    {/if}
  </section>

  <!-- Submitted -->
  <section class="bv-reviews-section" aria-labelledby="bv-reviews-submitted">
    <h2 id="bv-reviews-submitted" class="bv-reviews-section-title">Submitted</h2>
    {#if submittedReviews.length === 0}
      <p class="bv-reviews-empty">No submitted reviews awaiting acknowledgement.</p>
    {:else}
      {#each submittedReviews as r (r.review_id)}
        <article class="bv-review-card" aria-label="Submitted review">
          <div class="bv-review-card-row">
            <span class="bv-status-badge bv-status-badge--submitted" aria-label="Status: submitted">
              Submitted
            </span>
            <span class="bv-review-card-time">
              Submitted {formatTimestamp(r.submitted_at)}
            </span>
          </div>
          <div class="bv-review-card-meta">
            {pluralize(r.annotation_count, "annotation", "annotations")}
          </div>
        </article>
      {/each}
    {/if}
  </section>

  <!-- Acknowledged -->
  <section class="bv-reviews-section" aria-labelledby="bv-reviews-acknowledged">
    <h2 id="bv-reviews-acknowledged" class="bv-reviews-section-title">Acknowledged</h2>
    {#if acknowledgedReviews.length === 0}
      <p class="bv-reviews-empty">No acknowledged reviews yet.</p>
    {:else}
      {#each acknowledgedReviews as r (r.review_id)}
        <article class="bv-review-card" aria-label="Acknowledged review">
          <div class="bv-review-card-row">
            <span
              class="bv-status-badge bv-status-badge--acknowledged"
              aria-label="Status: acknowledged"
            >
              Acknowledged
            </span>
            <span class="bv-review-card-time">
              {r.acknowledged_at
                ? `Acknowledged ${formatTimestamp(r.acknowledged_at)}`
                : `Submitted ${formatTimestamp(r.submitted_at)}`}
            </span>
          </div>
          <div class="bv-review-card-meta">
            {pluralize(r.annotation_count, "annotation", "annotations")}
          </div>
        </article>
      {/each}
    {/if}
  </section>
</section>

<style>
  .bv-reviews-page {
    max-width: var(--bv-max-content);
    width: calc(100% - var(--bv-space-5) * 2);
    margin: 0 auto;
    padding: var(--bv-space-5) 0 var(--bv-space-7);
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-5);
  }

  .bv-reviews-header {
    display: flex;
    align-items: center;
    gap: var(--bv-space-3);
    flex-wrap: wrap;
  }
  .bv-reviews-header h1 {
    margin: 0;
    font-size: var(--bv-text-xl);
    line-height: 1.3;
    color: var(--bv-text);
  }

  .bv-reviews-back {
    display: inline-flex;
    align-items: center;
    gap: var(--bv-space-2);
    background: var(--bv-bg);
    border: 1px solid var(--bv-border);
    color: var(--bv-text-dim);
    border-radius: var(--bv-radius-md);
    padding: var(--bv-space-2) var(--bv-space-3);
    font-size: var(--bv-text-sm);
    cursor: pointer;
    min-height: 36px;
    transition:
      background-color var(--bv-duration-quick) var(--bv-ease-out),
      border-color var(--bv-duration-quick) var(--bv-ease-out),
      color var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-reviews-back:hover,
  .bv-reviews-back:focus-visible {
    background: var(--bv-surface-2);
    border-color: var(--bv-border-strong);
    color: var(--bv-text);
  }

  .bv-reviews-status {
    margin: 0;
    font-size: var(--bv-text-sm);
    color: var(--bv-text-dim);
  }
  .bv-reviews-status--error {
    color: var(--bv-danger, #b91c1c);
  }

  .bv-reviews-section {
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-3);
  }
  .bv-reviews-section-title {
    margin: 0;
    font-size: var(--bv-text-base);
    color: var(--bv-text);
    font-weight: 600;
  }
  .bv-reviews-empty {
    margin: 0;
    font-size: var(--bv-text-sm);
    color: var(--bv-text-dim);
  }

  .bv-review-card {
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-2);
    background: var(--bv-surface);
    border: 1px solid var(--bv-border);
    border-radius: var(--bv-radius-md);
    padding: var(--bv-space-3) var(--bv-space-4);
  }
  .bv-review-card--pending {
    border-color: var(--bv-accent);
    background: var(--bv-accent-soft);
  }
  .bv-review-card-row {
    display: flex;
    align-items: center;
    gap: var(--bv-space-3);
    flex-wrap: wrap;
  }
  .bv-review-card-time {
    font-size: var(--bv-text-sm);
    color: var(--bv-text-dim);
  }
  .bv-review-card-meta {
    font-size: var(--bv-text-xs);
    color: var(--bv-text-dim);
  }

  .bv-status-badge {
    display: inline-flex;
    align-items: center;
    gap: var(--bv-space-1);
    font-size: var(--bv-text-xs);
    padding: 4px var(--bv-space-2);
    border-radius: var(--bv-radius-pill);
    border: 1px solid var(--bv-border);
    background: var(--bv-bg);
    color: var(--bv-text-dim);
    line-height: 1.2;
    white-space: nowrap;
  }
  .bv-status-badge--pending {
    border-color: var(--bv-accent);
    background: var(--bv-accent-soft);
    color: var(--bv-accent-strong);
  }
  .bv-status-badge--submitted {
    border-color: var(--bv-info, #1d4ed8);
    background: color-mix(in srgb, var(--bv-info, #1d4ed8) 10%, transparent);
    color: var(--bv-info, #1d4ed8);
  }
  .bv-status-badge--acknowledged {
    border-color: var(--bv-success, #15803d);
    background: color-mix(in srgb, var(--bv-success, #15803d) 12%, transparent);
    color: var(--bv-success, #15803d);
  }
  .bv-pending-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--bv-accent);
    animation: bv-pending-pulse 2s ease-in-out infinite;
  }
  @keyframes bv-pending-pulse {
    0%,
    100% {
      box-shadow: 0 0 0 0 var(--bv-accent-soft);
    }
    50% {
      box-shadow: 0 0 0 4px transparent;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .bv-pending-dot {
      animation: none;
    }
  }
</style>
