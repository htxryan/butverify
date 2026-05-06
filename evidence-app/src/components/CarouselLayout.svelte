<script lang="ts">
  /* Carousel layout: horizontal scroll-snap row + prev/next buttons +
   * keyboard arrow nav. Built on CSS scroll-snap rather than transform-
   * positioned tracks so reduced-motion users get a non-animated snap
   * (browser handles scroll-behavior:smooth via the global override) and
   * touch devices get native momentum scrolling.
   *
   * Accessibility: items are still rendered in DOM order so screen
   * readers traverse the gallery sequentially regardless of which
   * card is centered. The scroll position is purely visual.
   */
  import type { EvidenceItem } from "../lib/manifest.js";
  import GalleryItem from "./GalleryItem.svelte";
  import { prefersReducedMotion } from "../lib/manifest.js";

  let { items }: { items: EvidenceItem[] } = $props();

  let railEl: HTMLElement | null = $state(null);
  let activeIndex = $state(0);

  function scrollToIndex(idx: number) {
    if (!railEl) return;
    const child = railEl.children[idx] as HTMLElement | undefined;
    if (!child) return;
    const reduced = prefersReducedMotion();
    railEl.scrollTo({
      left: child.offsetLeft - railEl.offsetLeft,
      behavior: reduced ? "auto" : "smooth",
    });
    activeIndex = idx;
  }

  function prev() {
    scrollToIndex(Math.max(0, activeIndex - 1));
  }
  function next() {
    scrollToIndex(Math.min(items.length - 1, activeIndex + 1));
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === "ArrowLeft") {
      e.preventDefault();
      prev();
    } else if (e.key === "ArrowRight") {
      e.preventDefault();
      next();
    } else if (e.key === "Home") {
      e.preventDefault();
      scrollToIndex(0);
    } else if (e.key === "End") {
      e.preventDefault();
      scrollToIndex(items.length - 1);
    }
  }

  // Keep activeIndex in sync with scroll position. Throttled by rAF so we
  // don't update state on every scroll tick.
  let rafId: number | null = null;
  function onScroll() {
    if (!railEl) return;
    if (rafId !== null) return;
    rafId = requestAnimationFrame(() => {
      rafId = null;
      if (!railEl) return;
      // Find the child whose left edge is closest to the rail's left edge.
      const railLeft = railEl.scrollLeft;
      let bestIdx = 0;
      let bestDist = Infinity;
      for (let i = 0; i < railEl.children.length; i++) {
        const c = railEl.children[i] as HTMLElement;
        const dist = Math.abs(c.offsetLeft - railEl.offsetLeft - railLeft);
        if (dist < bestDist) {
          bestDist = dist;
          bestIdx = i;
        }
      }
      activeIndex = bestIdx;
    });
  }
</script>

<div class="bv-carousel" role="region" aria-label="Evidence items, carousel layout">
  <div class="bv-carousel-controls">
    <button
      type="button"
      class="bv-carousel-button"
      aria-label="Previous item"
      onclick={prev}
      disabled={activeIndex === 0}
    >
      <svg viewBox="0 0 16 16" aria-hidden="true" width="16" height="16">
        <path d="M10 3 5 8l5 5" stroke="currentColor" stroke-width="1.5" fill="none" />
      </svg>
    </button>
    <span class="bv-carousel-status" aria-live="polite">
      {activeIndex + 1} / {items.length}
    </span>
    <button
      type="button"
      class="bv-carousel-button"
      aria-label="Next item"
      onclick={next}
      disabled={activeIndex === items.length - 1}
    >
      <svg viewBox="0 0 16 16" aria-hidden="true" width="16" height="16">
        <path d="M6 3l5 5-5 5" stroke="currentColor" stroke-width="1.5" fill="none" />
      </svg>
    </button>
  </div>

  <!--
    The carousel rail is a horizontally-scrolling list. We give it a
    tabindex + role=group + key handlers so keyboard users can step
    between items with arrow keys without needing to focus a child
    button. The list semantics (<ol>) remain so a screen reader still
    announces "list of N items" when traversing the gallery.

    svelte-ignore reasoning: a11y_no_noninteractive_tabindex and
    a11y_no_noninteractive_element_interactions both fire on <ol> here.
    The carousel pattern legitimately needs both — without tabindex=0
    keyboard users can't enter the rail; without onkeydown they can't
    step. The role="group" + aria-label combination satisfies the AT
    contract.
  -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <ol
    bind:this={railEl}
    class="bv-carousel-rail"
    onkeydown={onKeydown}
    onscroll={onScroll}
    tabindex="0"
    role="group"
    aria-label="Carousel items"
  >
    {#each items as item, idx (idx)}
      <li class="bv-carousel-cell" aria-current={idx === activeIndex ? "true" : null}>
        <GalleryItem {item} index={idx} layout="carousel" />
      </li>
    {/each}
  </ol>

  <div class="bv-carousel-dots" role="tablist" aria-label="Carousel position">
    {#each items as _, idx (idx)}
      <button
        type="button"
        role="tab"
        class="bv-carousel-dot"
        class:bv-active={idx === activeIndex}
        aria-selected={idx === activeIndex}
        aria-label={`Go to item ${idx + 1}`}
        onclick={() => scrollToIndex(idx)}
      ></button>
    {/each}
  </div>
</div>

<style>
  .bv-carousel {
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-3);
  }

  .bv-carousel-controls {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--bv-space-3);
    padding: 0 var(--bv-space-5);
  }

  .bv-carousel-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: var(--bv-radius-pill);
    border: 1px solid var(--bv-border);
    background: var(--bv-surface);
    color: var(--bv-text);
    transition: background var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-carousel-button:hover:not(:disabled) {
    background: var(--bv-surface-2);
  }
  .bv-carousel-button:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .bv-carousel-status {
    font-family: var(--bv-font-mono);
    font-size: var(--bv-text-sm);
    color: var(--bv-text-dim);
    min-width: 4ch;
    text-align: center;
  }

  .bv-carousel-rail {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    overflow-x: auto;
    scroll-snap-type: x mandatory;
    scrollbar-width: thin;
    /* Touch friction; honoring browser-default scroll-behavior so
     * reduced-motion users get instant snap. */
    -webkit-overflow-scrolling: touch;
  }
  .bv-carousel-rail:focus-visible {
    outline: 2px solid var(--bv-accent);
    outline-offset: 4px;
    border-radius: var(--bv-radius-md);
  }

  .bv-carousel-cell {
    flex: 0 0 100%;
    scroll-snap-align: start;
  }
  @media (min-width: 720px) {
    .bv-carousel-cell {
      flex-basis: 80%;
      max-width: 60rem;
      margin: 0 auto;
    }
  }

  .bv-carousel-dots {
    display: flex;
    justify-content: center;
    gap: var(--bv-space-2);
    padding: var(--bv-space-3) var(--bv-space-5) var(--bv-space-6);
  }
  .bv-carousel-dot {
    width: 8px;
    height: 8px;
    padding: 0;
    border-radius: 50%;
    background: var(--bv-border-strong);
    border: 0;
    transition: background var(--bv-duration-quick) var(--bv-ease-out),
      transform var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-carousel-dot.bv-active {
    background: var(--bv-accent);
    transform: scale(1.25);
  }
</style>
