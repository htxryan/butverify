<script lang="ts">
  /* Gallery shell: orchestrates layout switching, header, and item rendering.
   * pebble-4nwj wires the review-session context so when
   * `manifest.enable_reviews` is true, GalleryItem can render annotation
   * tools and the right-rail ReviewPanel can collect drafts.
   */
  import StackedLayout from "./StackedLayout.svelte";
  import CarouselLayout from "./CarouselLayout.svelte";
  import GalleryHeader from "./GalleryHeader.svelte";
  import EmptyState from "./EmptyState.svelte";
  import ReviewPanel from "./ReviewPanel.svelte";
  import CommentForm from "./CommentForm.svelte";
  import type { EvidenceManifest } from "../lib/manifest.js";
  import type { AnnotationDraft, AnnotationInput } from "../lib/annotations.js";
  import { createDraftStore } from "../lib/draft-store.js";
  import { setReviewSession, siteCommentInput } from "../lib/review-session.js";
  import { resolveApiBaseFromWindow } from "../lib/api-base.js";
  import { submitReview, type SubmitReviewResult } from "../lib/review-api.js";

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

  // ─── Review session ───────────────────────────────────────────────
  // Resolve the site_id from the host. The customer-site wildcard
  // routes `<site_id>(.|<env>.)butverify.dev` so the leftmost label
  // (minus an optional `-<env>` suffix) is the site_id.
  function deriveSiteId(): string {
    if (typeof window === "undefined" || !window.location?.host) return "unknown";
    const host = window.location.host.toLowerCase().split(":")[0] ?? "";
    const firstLabel = host.split(".")[0] ?? "";
    // Strip env suffix on dev/qa: <id>-dev / <id>-qa.
    const suffixMatch = firstLabel.match(/^(.+)-(dev|qa)$/);
    return suffixMatch?.[1] ?? firstLabel;
  }

  let siteId = deriveSiteId();
  // `enable_reviews` is fixed for the life of this component (set by the
  // CLI at publish time). Treat it as a constant rather than $derived
  // so we can use it inside the component-init `if` below — Gallery is
  // mounted once per page load with a fixed manifest.
  // svelte-ignore state_referenced_locally
  const reviewsEnabled = manifest.enable_reviews === true;
  let store = createDraftStore(siteId);
  let drafts = $state<AnnotationDraft[]>(reviewsEnabled ? store.load() : []);

  // Setting on the context tree requires a stable object identity.
  // We provide getter functions for state that should track the latest
  // $state value.
  if (reviewsEnabled) {
    setReviewSession({
      enabled: true,
      siteId,
      drafts: () => drafts,
      add: (input: AnnotationInput) => {
        const d = store.add(input);
        drafts = store.load();
        return d;
      },
      remove: (draftId: string) => {
        store.remove(draftId);
        drafts = store.load();
      },
    });
  }

  function removeDraft(draftId: string) {
    store.remove(draftId);
    drafts = store.load();
  }

  // ─── Site-level comment form ───────────────────────────────────────
  // Anchored to the gallery header. Toggled open on demand; closed by
  // default to keep the header tidy.
  let siteCommentOpen = $state(false);

  function openSiteComment() {
    siteCommentOpen = true;
  }
  function cancelSiteComment() {
    siteCommentOpen = false;
  }
  function saveSiteComment(comment: string) {
    store.add(siteCommentInput(comment));
    drafts = store.load();
    siteCommentOpen = false;
  }

  // ─── Submit ────────────────────────────────────────────────────────
  let submitting = $state(false);
  let submitResult = $state<SubmitReviewResult | null>(null);

  async function onSubmit() {
    if (drafts.length === 0 || submitting) return;
    submitting = true;
    submitResult = null;
    const apiBase = resolveApiBaseFromWindow();
    const result = await submitReview({
      apiBase,
      siteId,
      drafts,
    });
    submitResult = result;
    submitting = false;
    // EV2-S-1 says drafts are local-only until submit. On success we
    // clear the local store so the user doesn't accidentally
    // re-submit. On failure (esp. expired-site / network), we keep
    // the draft so they can copy it elsewhere or retry.
    if (result.ok) {
      store.clear();
      drafts = [];
    }
  }
</script>

<a href="#bv-gallery-content" class="bv-skip-link">Skip to gallery</a>

<div
  class="bv-gallery-shell"
  class:bv-gallery-shell--with-review={reviewsEnabled}
>
  <div class="bv-gallery-main">
    <GalleryHeader {manifest} bind:layout />

    {#if reviewsEnabled}
      <div class="bv-site-comment-region">
        {#if siteCommentOpen}
          <CommentForm
            title="Comment on the whole site"
            placeholder="Overall thoughts on this gallery?"
            onSave={saveSiteComment}
            onCancel={cancelSiteComment}
          />
        {:else}
          <button
            type="button"
            class="bv-site-comment-trigger"
            onclick={openSiteComment}
          >
            + Site comment
          </button>
        {/if}
      </div>
    {/if}

    {#if manifest.items.length === 0}
      <EmptyState />
    {:else}
      <main id="bv-gallery-content" class="bv-gallery-content" tabindex="-1">
        {#if layout === "stacked"}
          <StackedLayout items={manifest.items} />
        {:else}
          <CarouselLayout items={manifest.items} />
        {/if}
      </main>
    {/if}
  </div>

  {#if reviewsEnabled}
    <div class="bv-gallery-review-rail">
      <ReviewPanel
        {drafts}
        {submitting}
        result={submitResult}
        onRemove={removeDraft}
        {onSubmit}
      />
    </div>
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

  /* Two-column desktop layout when reviews are enabled. The review
   * rail is sticky inside its column so it stays visible as the
   * gallery scrolls. */
  @media (min-width: 1024px) {
    .bv-gallery-shell--with-review {
      display: grid;
      grid-template-columns: minmax(0, 1fr) 24rem;
      gap: var(--bv-space-5);
      padding-right: var(--bv-space-5);
    }
  }

  .bv-gallery-main {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .bv-gallery-review-rail {
    /* On mobile, render below the gallery. The ReviewPanel itself is
     * sticky (bottom of viewport) within this column on narrow screens
     * and at the top on wide screens. */
    padding: var(--bv-space-5);
  }

  .bv-site-comment-region {
    max-width: var(--bv-max-content);
    width: 100%;
    margin: 0 auto;
    padding: 0 var(--bv-space-5) var(--bv-space-3);
  }
  .bv-site-comment-trigger {
    background: var(--bv-surface);
    border: 1px dashed var(--bv-border-strong);
    color: var(--bv-text-dim);
    border-radius: var(--bv-radius-pill);
    padding: var(--bv-space-2) var(--bv-space-4);
    font-size: var(--bv-text-sm);
    cursor: pointer;
  }
  .bv-site-comment-trigger:hover,
  .bv-site-comment-trigger:focus-visible {
    color: var(--bv-text);
    background: var(--bv-surface-2);
    border-style: solid;
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
