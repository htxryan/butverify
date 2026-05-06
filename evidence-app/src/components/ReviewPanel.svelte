<script lang="ts">
  /* pebble-4nwj — Review panel.
   *
   * Renders the accumulated draft annotations and the submit button.
   * EV2-S-1: drafts are local-only until submit. EV2-N-1: submit is
   * disabled until at least one annotation exists. EV2-S-3 / 409:
   * expired sites surface a user-facing message and DO NOT clear the
   * draft (so the user keeps their work).
   *
   * The panel sits in a sticky right rail on wide viewports and as a
   * sheet at the bottom of the page on narrow ones. Drafts are
   * editable individually (delete) and the submit action is undo-able
   * only via "submit again" — submitted reviews are immutable per
   * EV2-N-2.
   */
  import type { AnnotationDraft } from "../lib/annotations.js";
  import type { SubmitReviewResult } from "../lib/review-api.js";

  let {
    drafts,
    onRemove,
    onSubmit,
    submitting = false,
    result = null,
  }: {
    drafts: AnnotationDraft[];
    onRemove: (draftId: string) => void;
    onSubmit: () => void;
    submitting?: boolean;
    result?: SubmitReviewResult | null;
  } = $props();

  let canSubmit = $derived(drafts.length > 0 && !submitting);

  // Map draft type → human label for the per-draft chip.
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

  // Truncate the draft preview so the panel stays scannable when
  // someone writes a paragraph-long comment. The full text is still
  // submitted; only the chip is truncated.
  function preview(text: string): string {
    if (text.length <= 80) return text;
    return text.slice(0, 77) + "…";
  }

  // Surface the spec's domain error codes as user-facing copy. When
  // result is a "submitted successfully" we render the success state;
  // otherwise we render the failure kind. Stored on result?.kind so
  // copy-changes don't need to touch review-api.ts.
  function failureMessage(r: Extract<SubmitReviewResult, { ok: false }>): string {
    switch (r.kind) {
      case "site_not_accepting_reviews":
        return "This site is no longer accepting reviews. Your draft is still saved locally if you want to copy it elsewhere.";
      case "review_already_submitted":
        return "You already submitted a review for this site. Reviews are one-per-viewer.";
      case "empty_review":
        return "Add at least one annotation before submitting.";
      case "unauthenticated":
        return "You're signed out. Reload the page to sign back in, then submit.";
      case "rate_limited":
        return r.retry_after_seconds
          ? `Too many submissions. Try again in ${r.retry_after_seconds}s.`
          : "Too many submissions. Try again shortly.";
      case "validation_failed":
        return `One of your annotations was rejected by the server: ${r.message}`;
      case "site_not_found":
        return "This site can't accept reviews. It may have been removed or never had reviews enabled.";
      case "network":
        return "Network error. Check your connection and try again — your draft is saved.";
      case "unknown":
        return r.message || "Something went wrong submitting your review.";
    }
  }
</script>

<aside
  class="bv-review-panel"
  aria-label="Review draft"
  data-bv-review-panel="true"
>
  <header class="bv-review-panel__header">
    <h2>Review draft</h2>
    <span class="bv-review-panel__count" aria-live="polite">
      {drafts.length}
      {drafts.length === 1 ? "annotation" : "annotations"}
    </span>
  </header>

  {#if drafts.length === 0}
    <p class="bv-review-panel__empty">
      Add a comment, draw a region on an image, or highlight text to start a review.
    </p>
  {:else}
    <ul class="bv-review-panel__list">
      {#each drafts as draft (draft.draft_id)}
        <li class="bv-review-panel__item">
          <div class="bv-review-panel__item-meta">
            <span class="bv-review-panel__item-type">{labelFor(draft)}</span>
            <button
              type="button"
              class="bv-review-panel__remove"
              aria-label={`Remove ${labelFor(draft).toLowerCase()}`}
              onclick={() => onRemove(draft.draft_id)}
              disabled={submitting}
            >
              Remove
            </button>
          </div>
          <p class="bv-review-panel__item-comment">{preview(draft.comment)}</p>
        </li>
      {/each}
    </ul>
  {/if}

  {#if result?.ok === true}
    <div class="bv-review-panel__success" role="status">
      <strong>Review submitted.</strong>
      <p>
        Saved as <code>{result.review_id}</code>. The site owner has been notified.
      </p>
    </div>
  {:else if result && result.ok === false}
    <div class="bv-review-panel__error" role="alert">
      {failureMessage(result)}
    </div>
  {/if}

  <button
    type="button"
    class="bv-review-panel__submit"
    onclick={onSubmit}
    disabled={!canSubmit}
    aria-busy={submitting}
  >
    {submitting ? "Submitting…" : "Submit review"}
  </button>
</aside>

<style>
  .bv-review-panel {
    background: var(--bv-bg);
    border: 1px solid var(--bv-border);
    border-radius: var(--bv-radius-md);
    padding: var(--bv-space-4);
    box-shadow: var(--bv-shadow-md);
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-3);
    /* Mobile sheet: full-width footer at the bottom of the viewport.
     * Desktop layout overrides via @media below. */
    position: sticky;
    bottom: 0;
    z-index: 30;
  }

  .bv-review-panel__header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--bv-space-3);
  }
  .bv-review-panel__header h2 {
    font-size: var(--bv-text-lg);
    margin: 0;
    color: var(--bv-text);
  }
  .bv-review-panel__count {
    font-size: var(--bv-text-sm);
    color: var(--bv-text-muted);
  }

  .bv-review-panel__empty {
    margin: 0;
    color: var(--bv-text-muted);
    font-size: var(--bv-text-sm);
  }

  .bv-review-panel__list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-2);
    /* Cap the list height on desktop so the submit button stays
     * visible without scrolling the entire panel. */
    max-height: 40vh;
    overflow-y: auto;
  }

  .bv-review-panel__item {
    background: var(--bv-surface);
    border: 1px solid var(--bv-border);
    border-radius: var(--bv-radius-sm);
    padding: var(--bv-space-2) var(--bv-space-3);
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-1);
  }

  .bv-review-panel__item-meta {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--bv-space-2);
  }
  .bv-review-panel__item-type {
    font-size: var(--bv-text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--bv-text-muted);
  }

  .bv-review-panel__item-comment {
    margin: 0;
    font-size: var(--bv-text-sm);
    color: var(--bv-text);
    word-break: break-word;
  }

  .bv-review-panel__remove {
    background: transparent;
    border: 0;
    color: var(--bv-text-muted);
    font-size: var(--bv-text-xs);
    cursor: pointer;
    padding: 2px var(--bv-space-1);
    border-radius: var(--bv-radius-sm);
  }
  .bv-review-panel__remove:hover:not(:disabled),
  .bv-review-panel__remove:focus-visible {
    color: var(--bv-danger);
    background: var(--bv-surface-2);
  }
  .bv-review-panel__remove:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .bv-review-panel__submit {
    background: var(--bv-accent);
    color: #ffffff;
    border: 0;
    border-radius: var(--bv-radius-md);
    padding: var(--bv-space-3) var(--bv-space-4);
    font-size: var(--bv-text-base);
    font-weight: 600;
    cursor: pointer;
    transition: background-color 80ms;
  }
  .bv-review-panel__submit:hover:not(:disabled),
  .bv-review-panel__submit:focus-visible {
    background: var(--bv-accent-strong);
  }
  .bv-review-panel__submit:disabled {
    background: var(--bv-border-strong);
    cursor: not-allowed;
  }

  .bv-review-panel__success {
    background: rgba(30, 138, 90, 0.08);
    border: 1px solid rgba(30, 138, 90, 0.4);
    color: var(--bv-good);
    border-radius: var(--bv-radius-sm);
    padding: var(--bv-space-2) var(--bv-space-3);
    font-size: var(--bv-text-sm);
  }
  .bv-review-panel__success strong {
    display: block;
    margin-bottom: 2px;
    color: var(--bv-good);
  }
  .bv-review-panel__success p {
    margin: 0;
    color: var(--bv-text);
  }
  .bv-review-panel__success code {
    font-family: var(--bv-font-mono);
    font-size: var(--bv-text-xs);
    background: var(--bv-surface-2);
    padding: 1px 4px;
    border-radius: 2px;
  }

  .bv-review-panel__error {
    background: rgba(200, 54, 47, 0.08);
    border: 1px solid rgba(200, 54, 47, 0.4);
    color: var(--bv-danger);
    border-radius: var(--bv-radius-sm);
    padding: var(--bv-space-2) var(--bv-space-3);
    font-size: var(--bv-text-sm);
  }

  /* Desktop: rail layout. The Gallery component wraps this in a
   * grid container with this column at the right edge. */
  @media (min-width: 1024px) {
    .bv-review-panel {
      position: sticky;
      top: var(--bv-space-5);
      bottom: auto;
      max-width: 22rem;
    }
  }
</style>
