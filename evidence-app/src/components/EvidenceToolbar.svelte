<script lang="ts">
  /* pebble-eaku — Unified evidence toolbar.
   *
   * One horizontal row above the gallery content that consolidates:
   *   - Site comment trigger
   *   - Carousel pager (carousel layout only)
   *   - Annotation dropdown (carousel layout only — operates on the
   *     active item via the review-session registry)
   *   - Pending count badge (drafts.length when > 0)
   *   - "Pending submission" indicator (subtle dot when drafts > 0)
   *   - Past reviews count badge (when > 0)
   *
   * In stacked mode, the per-item ReviewTools handles the annotation
   * dropdown (no concept of "active item"); the toolbar still shows
   * site-comment + counts for cross-item concerns.
   */

  import type { ItemHandlers } from "../lib/review-session.js";

  let {
    layout,
    pendingCount,
    pastReviewsCount,
    siteCommentOpen,
    onSiteComment,
    // Carousel-mode props (ignored in stacked mode)
    carouselActiveIndex = 0,
    carouselTotal = 0,
    onCarouselPrev = () => {},
    onCarouselNext = () => {},
    // Active-item handlers (carousel-mode only). Null when no review
    // session, or when the active item has not yet registered.
    activeItemHandlers = null,
  }: {
    layout: "stacked" | "carousel";
    pendingCount: number;
    pastReviewsCount: number;
    siteCommentOpen: boolean;
    onSiteComment: () => void;
    carouselActiveIndex?: number;
    carouselTotal?: number;
    onCarouselPrev?: () => void;
    onCarouselNext?: () => void;
    activeItemHandlers?: ItemHandlers | null;
  } = $props();

  let dropdownOpen = $state(false);

  // Re-read draw mode reactively from the active item's registered
  // getter. When the active item changes (carousel scroll), the getter
  // identity changes so this $derived recomputes.
  let drawMode = $derived(activeItemHandlers?.getDrawMode() ?? "off");
  let canCaptureHighlight = $derived(
    activeItemHandlers?.getCanCaptureHighlight() ?? false,
  );

  function selectComment() {
    dropdownOpen = false;
    activeItemHandlers?.openComment();
  }
  function selectMode(m: "circle" | "rect") {
    dropdownOpen = false;
    if (!activeItemHandlers) return;
    activeItemHandlers.setDrawMode(drawMode === m ? "off" : m);
  }
  function selectHighlight() {
    if (!canCaptureHighlight) return;
    dropdownOpen = false;
    activeItemHandlers?.captureHighlight();
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.stopPropagation();
      dropdownOpen = false;
    }
  }

  function handleFocusOut(e: FocusEvent) {
    if (!(e.currentTarget as Element).contains(e.relatedTarget as Node | null)) {
      dropdownOpen = false;
    }
  }

  let showPager = $derived(layout === "carousel" && carouselTotal > 1);
  let showAnnotate = $derived(layout === "carousel" && activeItemHandlers !== null);
  let pagerAtStart = $derived(carouselActiveIndex <= 0);
  let pagerAtEnd = $derived(carouselActiveIndex >= carouselTotal - 1);
</script>

<div class="bv-toolbar" role="toolbar" aria-label="Evidence toolbar">
  <!-- Left: site comment -->
  <button
    type="button"
    class="bv-toolbar-btn bv-toolbar-site-comment"
    onclick={onSiteComment}
    disabled={siteCommentOpen}
    aria-pressed={siteCommentOpen}
  >
    <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" fill="none"
      stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
      <path d="M3 3h10a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H6l-3 3V4a1 1 0 0 1 1-1Z" />
    </svg>
    <span>Site comment</span>
  </button>

  <!-- Center: carousel pager -->
  {#if showPager}
    <div class="bv-toolbar-pager" role="group" aria-label="Carousel position">
      <button
        type="button"
        class="bv-toolbar-pager-btn"
        aria-label="Previous item"
        onclick={onCarouselPrev}
        disabled={pagerAtStart}
      >
        <svg viewBox="0 0 16 16" aria-hidden="true" width="16" height="16">
          <path d="M10 3 5 8l5 5" stroke="currentColor" stroke-width="1.5" fill="none" />
        </svg>
      </button>
      <span class="bv-toolbar-pager-status" aria-live="polite">
        {carouselActiveIndex + 1} / {carouselTotal}
      </span>
      <button
        type="button"
        class="bv-toolbar-pager-btn"
        aria-label="Next item"
        onclick={onCarouselNext}
        disabled={pagerAtEnd}
      >
        <svg viewBox="0 0 16 16" aria-hidden="true" width="16" height="16">
          <path d="M6 3l5 5-5 5" stroke="currentColor" stroke-width="1.5" fill="none" />
        </svg>
      </button>
    </div>
  {/if}

  <!-- Right: counts + annotate dropdown -->
  <div class="bv-toolbar-right">
    {#if pendingCount > 0}
      <span
        class="bv-toolbar-count bv-toolbar-count--pending"
        aria-label={`${pendingCount} pending ${pendingCount === 1 ? "comment" : "comments"} awaiting submission`}
        title="Pending — not yet submitted"
      >
        <span class="bv-toolbar-pending-dot" aria-hidden="true"></span>
        {pendingCount} pending
      </span>
    {/if}

    {#if pastReviewsCount > 0}
      <span
        class="bv-toolbar-count bv-toolbar-count--past"
        aria-label={`${pastReviewsCount} past ${pastReviewsCount === 1 ? "review" : "reviews"}`}
        title="Past reviews on this site"
      >
        {pastReviewsCount} past
      </span>
    {/if}

    {#if showAnnotate}
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <div
        class="bv-toolbar-annotate"
        role="presentation"
        onkeydown={handleKeydown}
        onfocusout={handleFocusOut}
      >
        <button
          type="button"
          class="bv-toolbar-btn bv-toolbar-annotate-trigger"
          class:bv-toolbar-btn--active={drawMode !== "off"}
          aria-haspopup="true"
          aria-expanded={dropdownOpen}
          aria-label="Annotation tools"
          onclick={() => (dropdownOpen = !dropdownOpen)}
        >
          <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" fill="none"
            stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M11 3 13 5l-7 7H4v-2l7-7Z" />
            <path d="M10 4l2 2" />
          </svg>
          <span>Annotate</span>
          <svg
            class="bv-toolbar-chevron"
            class:bv-toolbar-chevron--open={dropdownOpen}
            viewBox="0 0 16 16" width="12" height="12" aria-hidden="true" fill="none"
            stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M4 6l4 4 4-4" />
          </svg>
        </button>

        {#if dropdownOpen}
          <ul class="bv-toolbar-menu" aria-label="Annotation tools">
            <li>
              <button
                type="button"
                class="bv-toolbar-menu-item"
                onclick={selectComment}
              >Comment on this item</button>
            </li>
            <li>
              <button
                type="button"
                class="bv-toolbar-menu-item"
                class:bv-toolbar-menu-item--active={drawMode === "circle"}
                aria-pressed={drawMode === "circle"}
                onclick={() => selectMode("circle")}
              >Draw circle</button>
            </li>
            <li>
              <button
                type="button"
                class="bv-toolbar-menu-item"
                class:bv-toolbar-menu-item--active={drawMode === "rect"}
                aria-pressed={drawMode === "rect"}
                onclick={() => selectMode("rect")}
              >Draw rectangle</button>
            </li>
            <li>
              <button
                type="button"
                class="bv-toolbar-menu-item"
                onclick={selectHighlight}
                disabled={!canCaptureHighlight}
                title={canCaptureHighlight ? "Annotate the highlighted text" : "Select text in the description first"}
              >Highlight selected text</button>
            </li>
          </ul>
        {/if}
      </div>
    {/if}
  </div>
</div>

<style>
  .bv-toolbar {
    display: flex;
    align-items: center;
    gap: var(--bv-space-3);
    flex-wrap: wrap;
    background: var(--bv-surface);
    border: 1px solid var(--bv-border);
    border-radius: var(--bv-radius-md);
    padding: var(--bv-space-2) var(--bv-space-3);
    margin: 0 auto var(--bv-space-4);
    max-width: var(--bv-max-content);
    width: calc(100% - var(--bv-space-5) * 2);
  }

  .bv-toolbar-btn {
    display: inline-flex;
    align-items: center;
    gap: var(--bv-space-2);
    background: var(--bv-bg);
    border: 1px solid var(--bv-border);
    color: var(--bv-text);
    border-radius: var(--bv-radius-md);
    padding: var(--bv-space-2) var(--bv-space-3);
    font-size: var(--bv-text-sm);
    cursor: pointer;
    min-height: 44px;
    transition:
      background-color var(--bv-duration-quick) var(--bv-ease-out),
      border-color var(--bv-duration-quick) var(--bv-ease-out),
      color var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-toolbar-btn:hover:not(:disabled),
  .bv-toolbar-btn:focus-visible {
    background: var(--bv-surface-2);
    border-color: var(--bv-border-strong);
  }
  .bv-toolbar-btn:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }
  .bv-toolbar-btn--active {
    background: var(--bv-accent-soft);
    border-color: var(--bv-accent);
    color: var(--bv-accent-strong);
  }

  .bv-toolbar-site-comment {
    color: var(--bv-text-dim);
  }
  .bv-toolbar-site-comment:hover:not(:disabled),
  .bv-toolbar-site-comment:focus-visible {
    color: var(--bv-text);
  }

  /* ── Pager ─────────────────────────────────────────────────────── */
  .bv-toolbar-pager {
    display: inline-flex;
    align-items: center;
    gap: var(--bv-space-2);
    margin: 0 auto;
  }
  .bv-toolbar-pager-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: var(--bv-radius-pill);
    border: 1px solid var(--bv-border);
    background: var(--bv-bg);
    color: var(--bv-text);
    cursor: pointer;
    transition:
      background var(--bv-duration-quick) var(--bv-ease-out),
      border-color var(--bv-duration-quick) var(--bv-ease-out),
      color var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-toolbar-pager-btn:hover:not(:disabled) {
    background: var(--bv-surface-2);
    border-color: var(--bv-border-strong);
  }
  .bv-toolbar-pager-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
  .bv-toolbar-pager-status {
    font-family: var(--bv-font-mono);
    font-size: var(--bv-text-sm);
    color: var(--bv-text-dim);
    min-width: 4ch;
    text-align: center;
  }

  /* ── Right cluster ─────────────────────────────────────────────── */
  .bv-toolbar-right {
    display: inline-flex;
    align-items: center;
    gap: var(--bv-space-2);
    margin-left: auto;
  }

  .bv-toolbar-count {
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
  .bv-toolbar-count--pending {
    color: var(--bv-accent-strong);
    border-color: var(--bv-accent);
    background: var(--bv-accent-soft);
  }
  .bv-toolbar-pending-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--bv-accent);
    box-shadow: 0 0 0 0 rgba(0, 0, 0, 0);
    animation: bv-pending-pulse 2s ease-in-out infinite;
  }
  @keyframes bv-pending-pulse {
    0%, 100% {
      box-shadow: 0 0 0 0 var(--bv-accent-soft);
    }
    50% {
      box-shadow: 0 0 0 4px transparent;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .bv-toolbar-pending-dot {
      animation: none;
    }
  }

  /* ── Annotate dropdown ─────────────────────────────────────────── */
  .bv-toolbar-annotate {
    position: relative;
  }
  .bv-toolbar-chevron {
    transition: transform var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-toolbar-chevron--open {
    transform: rotate(180deg);
  }

  .bv-toolbar-menu {
    position: absolute;
    top: calc(100% + var(--bv-space-1));
    right: 0;
    z-index: 100;
    list-style: none;
    margin: 0;
    padding: var(--bv-space-1);
    min-width: 14rem;
    background: var(--bv-bg);
    border: 1px solid var(--bv-border-strong);
    border-radius: var(--bv-radius-md);
    box-shadow: var(--bv-shadow-md);
  }
  .bv-toolbar-menu li {
    list-style: none;
  }
  .bv-toolbar-menu-item {
    display: block;
    width: 100%;
    text-align: left;
    background: transparent;
    border: none;
    color: var(--bv-text);
    font-size: var(--bv-text-sm);
    padding: var(--bv-space-2) var(--bv-space-3);
    border-radius: var(--bv-radius-sm);
    cursor: pointer;
    min-height: 44px;
    transition:
      background var(--bv-duration-quick) var(--bv-ease-out),
      color var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-toolbar-menu-item:hover:not(:disabled),
  .bv-toolbar-menu-item:focus-visible {
    background: var(--bv-surface-2);
  }
  .bv-toolbar-menu-item--active {
    color: var(--bv-accent-strong);
    background: var(--bv-accent-soft);
  }
  .bv-toolbar-menu-item:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
