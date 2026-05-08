<script lang="ts">
  /* Gallery shell: orchestrates page routing, layout switching, and
   * review session context.
   *
   * Two pages:
   *   evidence (default) — gallery items, sticky topbar
   *   details  (#details) — full metadata, same sticky topbar
   *
   * pebble-eaku — site comment trigger, carousel pager, annotation
   * dropdown, and review counts are unified into EvidenceToolbar above
   * the gallery content. The active carousel index lives here so the
   * toolbar can drive the carousel and the per-item registry.
   */
  import StackedLayout from "./StackedLayout.svelte";
  import CarouselLayout from "./CarouselLayout.svelte";
  import GalleryHeader from "./GalleryHeader.svelte";
  import MetadataPage from "./MetadataPage.svelte";
  import EmptyState from "./EmptyState.svelte";
  import ReviewPanel from "./ReviewPanel.svelte";
  import CommentForm from "./CommentForm.svelte";
  import EvidenceToolbar from "./EvidenceToolbar.svelte";
  import type { EvidenceManifest } from "../lib/manifest.js";
  import type { AnnotationDraft, AnnotationInput } from "../lib/annotations.js";
  import { createDraftStore } from "../lib/draft-store.js";
  import {
    setReviewSession,
    siteCommentInput,
    type ItemHandlers,
  } from "../lib/review-session.js";
  import { resolveApiBaseFromWindow } from "../lib/api-base.js";
  import { submitReview, type SubmitReviewResult } from "../lib/review-api.js";

  type Layout = "stacked" | "carousel";
  type Page = "evidence" | "details";

  let { manifest }: { manifest: EvidenceManifest } = $props();

  // ─── Page routing (hash-based) ────────────────────────────────────
  function readPage(): Page {
    if (typeof window === "undefined") return "evidence";
    return window.location.hash === "#details" ? "details" : "evidence";
  }

  let page = $state<Page>(readPage());

  $effect(() => {
    if (typeof window === "undefined") return;
    function onHashChange() {
      page = readPage();
    }
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  });

  function navigateTo(p: Page) {
    if (typeof window === "undefined") return;
    history.pushState(null, "", p === "details" ? "#details" : "#");
    page = p;
    // Scroll to top on page change.
    window.scrollTo({ top: 0, behavior: "instant" });
  }

  // ─── Layout preference ─────────────────────────────────────────────
  function readInitialLayout(): Layout {
    if (typeof window === "undefined") return "stacked";
    const param = new URLSearchParams(window.location.search).get("layout");
    if (param === "carousel" || param === "stacked") return param;
    try {
      const stored = window.localStorage.getItem("bv-evidence-layout");
      if (stored === "carousel" || stored === "stacked") return stored;
    } catch {
      // localStorage unavailable — fall through to default.
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
  function deriveSiteId(): string {
    if (typeof window === "undefined" || !window.location?.host) return "unknown";
    const host = window.location.host.toLowerCase().split(":")[0] ?? "";
    const firstLabel = host.split(".")[0] ?? "";
    const suffixMatch = firstLabel.match(/^(.+)-(dev|qa)$/);
    return suffixMatch?.[1] ?? firstLabel;
  }

  let siteId = deriveSiteId();
  // svelte-ignore state_referenced_locally
  const reviewsEnabled = manifest.enable_reviews === true;
  let store = createDraftStore(siteId);
  let drafts = $state<AnnotationDraft[]>(reviewsEnabled ? store.load() : []);

  // pebble-eaku — per-item registry. Plain Map for the storage with a
  // reactive version counter so Svelte $derived re-runs when items
  // mount/unmount. Map mutations don't trigger fine-grained reactivity
  // on their own; the counter is the explicit dependency.
  const itemRegistry = new Map<number, ItemHandlers>();
  let registryVersion = $state(0);

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
      registerItem: (itemIndex, handlers) => {
        itemRegistry.set(itemIndex, handlers);
        registryVersion = registryVersion + 1;
      },
      unregisterItem: (itemIndex) => {
        itemRegistry.delete(itemIndex);
        registryVersion = registryVersion + 1;
      },
      getItem: (itemIndex) => itemRegistry.get(itemIndex) ?? null,
    });
  }

  function removeDraft(draftId: string) {
    store.remove(draftId);
    drafts = store.load();
  }

  // ─── Site-level comment form ───────────────────────────────────────
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

  // ─── Carousel pager state (lifted from CarouselLayout) ────────────
  let carouselActiveIndex = $state(0);

  function carouselPrev() {
    if (carouselActiveIndex > 0) {
      carouselActiveIndex = carouselActiveIndex - 1;
    }
  }
  function carouselNext() {
    if (carouselActiveIndex < manifest.items.length - 1) {
      carouselActiveIndex = carouselActiveIndex + 1;
    }
  }

  // pebble-eaku — re-read active item handlers when the index or the
  // registry membership changes. registryVersion is the explicit
  // reactive dependency for Map mutation; carouselActiveIndex selects
  // the active row.
  let activeItemHandlers = $derived.by(() => {
    if (!reviewsEnabled || layout !== "carousel") return null;
    void registryVersion;
    return itemRegistry.get(carouselActiveIndex) ?? null;
  });

  // pebble-eaku — past reviews count placeholder. Wiring of an actual
  // viewer-accessible "list my reviews on this site" endpoint is
  // tracked in pebble-60yj. For now we expose 0 so the badge is
  // hidden; the toolbar branch is in place for the follow-up.
  let pastReviewsCount = $state(0);

  // ─── Submit ────────────────────────────────────────────────────────
  let submitting = $state(false);
  let submitResult = $state<SubmitReviewResult | null>(null);

  async function onSubmit() {
    if (drafts.length === 0 || submitting) return;
    submitting = true;
    submitResult = null;
    const apiBase = resolveApiBaseFromWindow();
    const result = await submitReview({ apiBase, siteId, drafts });
    submitResult = result;
    submitting = false;
    if (result.ok) {
      store.clear();
      drafts = [];
      pastReviewsCount = pastReviewsCount + 1;
    }
  }
</script>

<a href="#bv-gallery-content" class="bv-skip-link">Skip to content</a>

<div
  class="bv-shell"
  class:bv-shell--with-review={reviewsEnabled && page === "evidence"}
>
  <!-- Persistent topbar: sticky across both pages -->
  <GalleryHeader {manifest} bind:layout {page} onNavigate={navigateTo} />

  {#if page === "details"}
    <MetadataPage {manifest} />
  {:else}
    <!-- Evidence page -->
    <div class="bv-gallery-main">
      {#if reviewsEnabled}
        <div class="bv-toolbar-region">
          <EvidenceToolbar
            {layout}
            pendingCount={drafts.length}
            {pastReviewsCount}
            {siteCommentOpen}
            onSiteComment={openSiteComment}
            {carouselActiveIndex}
            carouselTotal={manifest.items.length}
            onCarouselPrev={carouselPrev}
            onCarouselNext={carouselNext}
            {activeItemHandlers}
          />
          {#if siteCommentOpen}
            <div class="bv-site-comment-region">
              <CommentForm
                title="Comment on the whole site"
                placeholder="Overall thoughts on this gallery?"
                onSave={saveSiteComment}
                onCancel={cancelSiteComment}
              />
            </div>
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
            <CarouselLayout
              items={manifest.items}
              bind:activeIndex={carouselActiveIndex}
            />
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
  {/if}
</div>

<style>
  .bv-shell {
    display: flex;
    flex-direction: column;
    min-height: 100dvh;
    background: var(--bv-bg);
    color: var(--bv-text);
  }

  /* Two-column desktop layout when reviews are active on the evidence page. */
  @media (min-width: 1024px) {
    .bv-shell--with-review {
      display: grid;
      /* topbar spans full width via its own sticky positioning; the
       * grid only governs the content rows below it. We use
       * subgrid-like approach: header gets its own row. */
      grid-template-columns: minmax(0, 1fr) 24rem;
      grid-template-rows: auto 1fr;
      gap: 0 var(--bv-space-5);
      padding-right: var(--bv-space-5);
    }
    .bv-shell--with-review > :global(.bv-topbar) {
      grid-column: 1 / -1;
    }
  }

  .bv-gallery-main {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .bv-gallery-review-rail {
    padding: var(--bv-space-5);
  }

  .bv-toolbar-region {
    padding: var(--bv-space-3) 0 0;
  }
  .bv-site-comment-region {
    max-width: var(--bv-max-content);
    width: 100%;
    margin: 0 auto;
    padding: 0 var(--bv-space-5) var(--bv-space-3);
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
