<script lang="ts">
  /* pebble-6fux — Review Details page.
   *
   * Renders a single past review IN CONTEXT: the gallery items are
   * shown with the review's image_region annotations drawn on the
   * images via the same AnnotationOverlay used in the editor, and a
   * read-only "Annotations" rail lists every annotation (site_comment,
   * item_comment, image_region, text_highlight) with its anchor.
   *
   * Hash route: #review/<review_id>
   *
   * The page reuses Gallery's review-session context (plumbed as
   * read-only by the parent), so GalleryItem renders overlays without
   * registering editing handlers and without rendering ReviewTools or
   * comment popovers. See GalleryItem.svelte's `readOnly` branch.
   *
   * The top frame makes it visually obvious the user is NOT in the
   * editor — distinct background, status badge, "Back" affordance.
   */
  import StackedLayout from "./StackedLayout.svelte";
  import EmptyState from "./EmptyState.svelte";
  import type { EvidenceManifest } from "../lib/manifest.js";
  import type { ReviewDetail } from "../lib/review-detail.js";
  import type { AnnotationDraft } from "../lib/annotations.js";

  type LoadState =
    | { kind: "idle" }
    | { kind: "loading" }
    | { kind: "ready"; review: ReviewDetail }
    | { kind: "error"; message: string; cause: "not_found" | "unauthenticated" | "other" };

  let {
    manifest,
    state,
    onBack,
  }: {
    manifest: EvidenceManifest;
    state: LoadState;
    onBack: () => void;
  } = $props();

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

  // Stable label per annotation type, mirrors ReviewPanel.labelFor but
  // tailored for a read-only catalog: includes 1-based item numbers
  // and a short anchor description.
  function labelFor(d: AnnotationDraft): string {
    switch (d.type) {
      case "site_comment":
        return "Site comment";
      case "item_comment":
        return `Item ${(d.item_index ?? 0) + 1} comment`;
      case "image_region":
        return `Item ${(d.item_index ?? 0) + 1} region (${d.region_shape ?? "?"})`;
      case "text_highlight":
        return d.text_scope === "item"
          ? `Item ${(d.item_index ?? 0) + 1} highlight`
          : "Site highlight";
    }
  }

  // Surface the captured snippet for text_highlight; for other types
  // we show the comment itself. Truncated so the rail stays scannable.
  function snippetFor(d: AnnotationDraft): string | null {
    if (d.type !== "text_highlight") return null;
    return d.text_snippet ?? null;
  }

  function truncate(text: string, max = 140): string {
    if (text.length <= max) return text;
    return text.slice(0, max - 1) + "…";
  }
</script>

<div class="bv-review-detail">
  <!-- Top frame: makes the review-viewing context visually distinct. -->
  <header class="bv-review-frame">
    <div class="bv-review-frame__row">
      <button
        type="button"
        class="bv-review-frame__back"
        onclick={onBack}
        aria-label="Back to reviews"
      >
        <svg
          viewBox="0 0 16 16"
          width="14"
          height="14"
          aria-hidden="true"
          fill="none"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M10 3 5 8l5 5" />
        </svg>
        <span>Back to reviews</span>
      </button>
      <div class="bv-review-frame__title">
        <span class="bv-review-frame__eyebrow">Viewing past review</span>
        {#if state.kind === "ready"}
          <span class="bv-review-frame__id">{state.review.review_id}</span>
        {/if}
      </div>
    </div>
    {#if state.kind === "ready"}
      <div class="bv-review-frame__meta">
        <span
          class="bv-status-badge bv-status-badge--{state.review.status}"
          aria-label={`Status: ${state.review.status}`}
        >
          {state.review.status === "acknowledged" ? "Acknowledged" : "Submitted"}
        </span>
        <span class="bv-review-frame__timestamp">
          Submitted {formatTimestamp(state.review.submitted_at)}
        </span>
        {#if state.review.acknowledged_at}
          <span class="bv-review-frame__timestamp">
            · Acknowledged {formatTimestamp(state.review.acknowledged_at)}
          </span>
        {/if}
        <span class="bv-review-frame__count">
          · {pluralize(state.review.annotation_count, "annotation", "annotations")}
        </span>
      </div>
    {/if}
  </header>

  {#if state.kind === "loading" || state.kind === "idle"}
    <p class="bv-review-detail__status" role="status">Loading review…</p>
  {:else if state.kind === "error"}
    <div class="bv-review-detail__error" role="alert">
      <p>
        {#if state.cause === "not_found"}
          This review can't be loaded. It may have been removed, never existed,
          or belong to another reviewer.
        {:else if state.cause === "unauthenticated"}
          You're signed out. Reload the page to sign back in, then try again.
        {:else}
          {state.message}
        {/if}
      </p>
      <button type="button" class="bv-review-detail__back-cta" onclick={onBack}>
        Back to reviews
      </button>
    </div>
  {:else}
    <div class="bv-review-detail__body">
      <main class="bv-review-detail__gallery" id="bv-review-gallery">
        {#if manifest.items.length === 0}
          <EmptyState />
        {:else}
          <StackedLayout items={manifest.items} />
        {/if}
      </main>
      <aside
        class="bv-review-detail__rail"
        aria-label="Annotations in this review"
      >
        <header class="bv-review-detail__rail-header">
          <h2>Annotations</h2>
          <span class="bv-review-detail__rail-count">
            {pluralize(state.review.annotations.length, "item", "items")}
          </span>
        </header>
        {#if state.review.annotations.length === 0}
          <p class="bv-review-detail__empty">
            This review has no annotations on record.
          </p>
        {:else}
          <ol class="bv-review-detail__rail-list">
            {#each state.review.annotations as a (a.annotation_id)}
              <li class="bv-review-detail__rail-item">
                <div class="bv-review-detail__rail-item-meta">
                  <span class="bv-review-detail__rail-item-type">
                    {labelFor({
                      draft_id: a.annotation_id,
                      type: a.type,
                      comment: a.comment,
                      ...(a.item_index !== null ? { item_index: a.item_index } : {}),
                      ...(a.region_shape !== null ? { region_shape: a.region_shape } : {}),
                      ...(a.text_scope !== null ? { text_scope: a.text_scope } : {}),
                    })}
                  </span>
                </div>
                {#if snippetFor({
                  draft_id: a.annotation_id,
                  type: a.type,
                  comment: a.comment,
                  ...(a.text_snippet !== null ? { text_snippet: a.text_snippet } : {}),
                  ...(a.text_scope !== null ? { text_scope: a.text_scope } : {}),
                })}
                  <blockquote class="bv-review-detail__rail-snippet">
                    “{truncate(a.text_snippet ?? "")}”
                  </blockquote>
                {/if}
                <p class="bv-review-detail__rail-comment">{a.comment}</p>
              </li>
            {/each}
          </ol>
        {/if}
      </aside>
    </div>
  {/if}
</div>

<style>
  .bv-review-detail {
    display: flex;
    flex-direction: column;
    min-height: 100dvh;
    background: var(--bv-bg);
    color: var(--bv-text);
  }

  /* Top frame — visually distinct from the editor topbar so the user
   * can see at a glance they are NOT in the editor. */
  .bv-review-frame {
    background: var(--bv-surface-2);
    border-bottom: 1px solid var(--bv-border);
    padding: var(--bv-space-3) var(--bv-space-5);
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-2);
    position: sticky;
    top: 0;
    z-index: 20;
  }
  .bv-review-frame__row {
    display: flex;
    align-items: center;
    gap: var(--bv-space-3);
    flex-wrap: wrap;
  }
  .bv-review-frame__back {
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
  .bv-review-frame__back:hover,
  .bv-review-frame__back:focus-visible {
    background: var(--bv-surface);
    border-color: var(--bv-border-strong);
    color: var(--bv-text);
  }
  .bv-review-frame__title {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .bv-review-frame__eyebrow {
    font-size: var(--bv-text-xs);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--bv-text-dim);
  }
  .bv-review-frame__id {
    font-family: var(--bv-font-mono);
    font-size: var(--bv-text-sm);
    color: var(--bv-text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 24rem;
  }
  .bv-review-frame__meta {
    display: flex;
    align-items: center;
    gap: var(--bv-space-2);
    flex-wrap: wrap;
    font-size: var(--bv-text-sm);
    color: var(--bv-text-dim);
  }
  .bv-review-frame__timestamp,
  .bv-review-frame__count {
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

  .bv-review-detail__status {
    margin: var(--bv-space-6) auto;
    color: var(--bv-text-dim);
    font-size: var(--bv-text-sm);
  }
  .bv-review-detail__error {
    margin: var(--bv-space-6) auto;
    max-width: 36rem;
    background: var(--bv-surface);
    border: 1px solid var(--bv-border);
    border-radius: var(--bv-radius-md);
    padding: var(--bv-space-4) var(--bv-space-5);
    color: var(--bv-text);
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-3);
  }
  .bv-review-detail__error p {
    margin: 0;
  }
  .bv-review-detail__back-cta {
    align-self: flex-start;
    background: var(--bv-accent);
    color: #fff;
    border: 0;
    border-radius: var(--bv-radius-md);
    padding: var(--bv-space-2) var(--bv-space-4);
    font-size: var(--bv-text-sm);
    cursor: pointer;
  }

  .bv-review-detail__body {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .bv-review-detail__gallery {
    flex: 1;
    width: 100%;
    min-width: 0;
  }
  .bv-review-detail__rail {
    padding: var(--bv-space-5);
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-3);
  }
  .bv-review-detail__rail-header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--bv-space-3);
  }
  .bv-review-detail__rail-header h2 {
    margin: 0;
    font-size: var(--bv-text-lg);
    color: var(--bv-text);
  }
  .bv-review-detail__rail-count {
    font-size: var(--bv-text-sm);
    color: var(--bv-text-dim);
  }
  .bv-review-detail__empty {
    margin: 0;
    color: var(--bv-text-dim);
    font-size: var(--bv-text-sm);
  }
  .bv-review-detail__rail-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-2);
  }
  .bv-review-detail__rail-item {
    background: var(--bv-surface);
    border: 1px solid var(--bv-border);
    border-radius: var(--bv-radius-sm);
    padding: var(--bv-space-2) var(--bv-space-3);
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-1);
  }
  .bv-review-detail__rail-item-meta {
    display: flex;
    align-items: center;
    gap: var(--bv-space-2);
  }
  .bv-review-detail__rail-item-type {
    font-size: var(--bv-text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--bv-text-muted, var(--bv-text-dim));
  }
  .bv-review-detail__rail-snippet {
    margin: 0;
    padding: var(--bv-space-1) var(--bv-space-2);
    background: var(--bv-bg);
    border-left: 3px solid var(--bv-accent);
    font-size: var(--bv-text-xs);
    color: var(--bv-text-dim);
    font-style: italic;
  }
  .bv-review-detail__rail-comment {
    margin: 0;
    color: var(--bv-text);
    font-size: var(--bv-text-sm);
    word-break: break-word;
  }

  /* Desktop: two-column layout (gallery + rail). Matches the editor's
   * grid breakpoint so the page feels consistent. */
  @media (min-width: 1024px) {
    .bv-review-detail__body {
      display: grid;
      grid-template-columns: minmax(0, 1fr) 24rem;
      gap: 0 var(--bv-space-5);
      padding-right: var(--bv-space-5);
    }
    .bv-review-detail__rail {
      position: sticky;
      top: calc(var(--bv-space-5) + 4rem);
      align-self: start;
      max-height: calc(100dvh - 6rem);
      overflow-y: auto;
    }
  }
</style>
