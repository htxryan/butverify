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
  import ReviewsPage from "./ReviewsPage.svelte";
  import ReviewDetailsPage from "./ReviewDetailsPage.svelte";
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
  import { listReviewsForViewer, type ViewerReview } from "../lib/review-list.js";
  import {
    getReviewDetail,
    reviewAnnotationsAsDrafts,
    type ReviewDetail,
  } from "../lib/review-detail.js";

  type Layout = "stacked" | "carousel";
  // pebble-6fux — `review-detail` is a fourth page that shares the
  // gallery shell but renders ReviewDetailsPage with annotations
  // fetched from the server (read-only). The active review_id is held
  // in `activeReviewId`; when set we fetch its detail into
  // `reviewDetailState`.
  type Page = "evidence" | "details" | "reviews" | "review-detail";

  let { manifest }: { manifest: EvidenceManifest } = $props();

  // ─── Page routing (hash-based) ────────────────────────────────────
  // pebble-6fux — adds a fourth route shape: `#review/<review_id>`
  // navigates to the Review Details page for that review. The
  // review_id is captured in `activeReviewId` and drives the fetch.
  // We keep the routing read-only here (no validation of the id
  // shape) — the server returns 404 for malformed ids, which the
  // page surfaces as a "not found" error state.
  const REVIEW_HASH_PREFIX = "#review/";

  function readPage(): { page: Page; review_id?: string } {
    if (typeof window === "undefined") return { page: "evidence" };
    const h = window.location.hash;
    if (h === "#details") return { page: "details" };
    if (h === "#reviews") return { page: "reviews" };
    if (h.startsWith(REVIEW_HASH_PREFIX)) {
      const id = decodeURIComponent(h.slice(REVIEW_HASH_PREFIX.length));
      if (id.length > 0) return { page: "review-detail", review_id: id };
    }
    return { page: "evidence" };
  }

  const initial = readPage();
  let page = $state<Page>(initial.page);
  let activeReviewId = $state<string | null>(initial.review_id ?? null);

  $effect(() => {
    if (typeof window === "undefined") return;
    function onHashChange() {
      const next = readPage();
      page = next.page;
      activeReviewId = next.review_id ?? null;
    }
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  });

  function navigateTo(p: Exclude<Page, "review-detail">) {
    if (typeof window === "undefined") return;
    const hash = p === "details" ? "#details" : p === "reviews" ? "#reviews" : "#";
    history.pushState(null, "", hash);
    page = p;
    activeReviewId = null;
    // Scroll to top on page change.
    window.scrollTo({ top: 0, behavior: "instant" });
  }

  function navigateToReviewDetail(reviewId: string): void {
    if (typeof window === "undefined") return;
    const hash = `${REVIEW_HASH_PREFIX}${encodeURIComponent(reviewId)}`;
    history.pushState(null, "", hash);
    page = "review-detail";
    activeReviewId = reviewId;
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

  // pebble-6fux — review-detail page state. When the route is
  // `#review/<id>` we fetch the full review (annotations included)
  // and swap the session's drafts() callback to surface those
  // annotations through the same overlay/registry path the editor
  // uses. The session is set to readOnly so editing affordances are
  // suppressed in GalleryItem and the right-rail panel is hidden.
  type ReviewDetailLoadState =
    | { kind: "idle" }
    | { kind: "loading" }
    | { kind: "ready"; review: ReviewDetail }
    | { kind: "error"; message: string; cause: "not_found" | "unauthenticated" | "other" };

  let reviewDetailState = $state<ReviewDetailLoadState>({ kind: "idle" });
  // Pre-converted drafts so the session.drafts() callback returns the
  // same array reference until the underlying review changes. Saves
  // re-running reviewAnnotationsAsDrafts on every overlay re-render.
  let reviewDetailDrafts = $derived<AnnotationDraft[]>(
    reviewDetailState.kind === "ready"
      ? reviewAnnotationsAsDrafts(reviewDetailState.review.annotations)
      : [],
  );

  if (reviewsEnabled) {
    setReviewSession({
      enabled: true,
      siteId,
      // pebble-6fux — the session swaps its drafts source based on the
      // active page. On the editor (`evidence`) it returns the local
      // draft list; on the Review Details page it returns the
      // server-fetched review's annotations as AnnotationDrafts.
      // `readOnly` is also page-derived; GalleryItem reads it once on
      // mount which is fine because Svelte tears down + remounts
      // GalleryItem when the page changes (different parent).
      get readOnly() {
        return page === "review-detail";
      },
      drafts: () =>
        page === "review-detail" ? reviewDetailDrafts : drafts,
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

  // pebble-60yj — past reviews are fetched from the control-plane
  // (GET /v1/sites/:id/reviews, viewer cookie). We keep both the array
  // (for the Reviews page) and a derived count (for the toolbar badge)
  // so the Reviews page does not need to refetch on navigation.
  let pastReviews = $state<ViewerReview[]>([]);
  let pastReviewsCount = $derived(pastReviews.length);
  let pastReviewsLoading = $state(false);
  let pastReviewsError = $state<string | null>(null);

  async function refreshPastReviews(): Promise<void> {
    if (!reviewsEnabled) return;
    pastReviewsLoading = true;
    pastReviewsError = null;
    const apiBase = resolveApiBaseFromWindow();
    const result = await listReviewsForViewer({ apiBase, siteId });
    pastReviewsLoading = false;
    if (result.ok) {
      pastReviews = result.reviews;
      return;
    }
    // unauthenticated: most viewers without a cookie won't have any
    // past reviews — silently treat as empty rather than surfacing an
    // error. The submit flow still walks the user through OAuth.
    if (result.kind === "unauthenticated") {
      pastReviews = [];
      return;
    }
    if (result.kind === "site_not_found") {
      // enable_reviews=false (server-side) — also silent.
      pastReviews = [];
      return;
    }
    pastReviewsError = result.message;
  }

  $effect(() => {
    if (!reviewsEnabled) return;
    void refreshPastReviews();
  });

  // pebble-6fux — Review-detail fetch effect. Re-runs whenever
  // activeReviewId changes (hash navigation). Aborts in-flight
  // requests when the user navigates away mid-fetch so a stale
  // response doesn't overwrite a freshly-loaded one.
  $effect(() => {
    if (!reviewsEnabled) {
      reviewDetailState = { kind: "idle" };
      return;
    }
    const reviewId = activeReviewId;
    if (page !== "review-detail" || !reviewId) {
      reviewDetailState = { kind: "idle" };
      return;
    }
    const ctrl = new AbortController();
    reviewDetailState = { kind: "loading" };
    const apiBase = resolveApiBaseFromWindow();
    void getReviewDetail({
      apiBase,
      siteId,
      reviewId,
      signal: ctrl.signal,
    }).then((res) => {
      if (ctrl.signal.aborted) return;
      if (res.ok) {
        reviewDetailState = { kind: "ready", review: res.review };
        return;
      }
      const cause: "not_found" | "unauthenticated" | "other" =
        res.kind === "not_found"
          ? "not_found"
          : res.kind === "unauthenticated"
            ? "unauthenticated"
            : "other";
      reviewDetailState = { kind: "error", message: res.message, cause };
    });
    return () => ctrl.abort();
  });

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
      // pebble-60yj — refresh from the server so the count and the
      // Reviews page reflect the persisted row, not just an optimistic
      // bump (which would desync if the server later 409s a retry).
      void refreshPastReviews();
    }
  }
</script>

<a href="#bv-gallery-content" class="bv-skip-link">Skip to content</a>

<div
  class="bv-shell"
  class:bv-shell--with-review={reviewsEnabled && page === "evidence"}
>
  <!-- Persistent topbar: sticky across editor / details / reviews
       pages. The Review Details page (pebble-6fux) provides its own
       frame so we suppress the global topbar there to avoid two
       stacked headers. -->
  {#if page !== "review-detail"}
    <GalleryHeader {manifest} bind:layout page={page} onNavigate={navigateTo} />
  {/if}

  {#if page === "details"}
    <MetadataPage {manifest} />
  {:else if page === "reviews"}
    <ReviewsPage
      {drafts}
      {pastReviews}
      listLoading={pastReviewsLoading}
      listError={pastReviewsError}
      onBack={() => navigateTo("evidence")}
      onOpenReview={navigateToReviewDetail}
    />
  {:else if page === "review-detail"}
    <ReviewDetailsPage
      {manifest}
      state={reviewDetailState}
      onBack={() => navigateTo("reviews")}
    />
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
            onOpenReviews={() => navigateTo("reviews")}
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
