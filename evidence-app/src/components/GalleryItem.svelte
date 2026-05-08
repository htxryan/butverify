<script lang="ts">
  import type { EvidenceItem } from "../lib/manifest.js";
  import { isImageItem, isVideoItem } from "../lib/manifest.js";
  import AnnotationOverlay from "./AnnotationOverlay.svelte";
  import ItemDetails from "./ItemDetails.svelte";
  import ReviewTools from "./ReviewTools.svelte";
  import CommentForm from "./CommentForm.svelte";
  import Lightbox from "./Lightbox.svelte";
  import {
    getReviewSession,
    imageRegionInput,
    itemCommentInput,
    textHighlightInput,
  } from "../lib/review-session.js";
  import { captureSelection } from "../lib/text-highlight.js";
  import type { RegionShape } from "../lib/annotations.js";

  let {
    item,
    index,
    layout,
  }: { item: EvidenceItem; index: number; layout: "stacked" | "carousel" } = $props();

  let isImage = $derived(isImageItem(item));
  let isVideo = $derived(isVideoItem(item));
  let altText = $derived(item.alt ?? item.title ?? `Evidence item ${index + 1}`);
  // Stable id used for keyboard skip-link targets ("Item 3 of 7").
  let itemId = $derived(`bv-item-${index + 1}`);

  // ─── Review session integration ──────────────────────────────────
  // getReviewSession returns null when reviews are disabled; we treat
  // that as the no-review fallback path.
  let session = getReviewSession();
  // pebble-6fux — when the session is read-only (Review Details page)
  // the item still RENDERS annotation overlays from the session's
  // drafts() but does NOT register editing handlers or render any
  // editor UI. The flag is read once (sessions are stable for the
  // lifetime of a page).
  let readOnly = session?.readOnly === true;

  // Per-item slice of drafts (for the AnnotationOverlay to render
  // already-saved regions without it knowing about the global list).
  let itemDrafts = $derived(
    session ? session.drafts().filter((d) => d.item_index === index) : [],
  );

  // Drawing mode: 'off', 'circle', or 'rect'. Owned per-item so two
  // items don't share a draw mode. In read-only mode this never
  // leaves "off" — the overlay's drawingEnabled is computed from it.
  let drawMode = $state<"off" | "circle" | "rect">("off");

  // pebble-eaku — register handlers with the session so the
  // gallery-level toolbar can drive this item's annotation actions
  // when this is the active carousel item.
  $effect(() => {
    if (!session || readOnly) return;
    session.registerItem(index, {
      openComment: () => openItemComment(),
      setDrawMode: (m) => setDrawMode(m),
      getDrawMode: () => drawMode,
      captureHighlight: () => captureHighlight(),
      getCanCaptureHighlight: () => canCaptureHighlight,
    });
    return () => session?.unregisterItem(index);
  });

  // Lightbox state — only images, only when not drawing.
  let lightboxOpen = $state(false);

  // Item-comment popover state.
  let itemCommentOpen = $state(false);

  // Pending text-highlight capture (offsets + snippet) waiting on the
  // user's comment.
  type PendingHighlight =
    | { kind: "idle" }
    | {
        kind: "pending";
        char_start: number;
        char_end: number;
        text_snippet: string;
      };
  let pendingHighlight: PendingHighlight = $state({ kind: "idle" });

  // Description ref for selection capture. Only present when the item
  // has a description rendered.
  let descriptionEl: HTMLParagraphElement | null = $state(null);

  // Track whether the description currently has a non-empty selection
  // so the Highlight button enables. We listen on selectionchange
  // because the Selection API doesn't bubble events to specific
  // elements.
  let canCaptureHighlight = $state(false);
  $effect(() => {
    if (!session || typeof document === "undefined") return;
    function update() {
      if (!descriptionEl) {
        canCaptureHighlight = false;
        return;
      }
      const sel = window.getSelection();
      if (!sel || sel.rangeCount === 0 || sel.isCollapsed) {
        canCaptureHighlight = false;
        return;
      }
      const range = sel.getRangeAt(0);
      canCaptureHighlight =
        descriptionEl.contains(range.startContainer) &&
        descriptionEl.contains(range.endContainer);
    }
    document.addEventListener("selectionchange", update);
    return () => document.removeEventListener("selectionchange", update);
  });

  function setDrawMode(mode: "off" | "circle" | "rect") {
    // Only one editor open at a time — close the comment popover when
    // the user enters draw mode.
    if (mode !== "off") itemCommentOpen = false;
    drawMode = mode;
  }

  function openItemComment() {
    drawMode = "off";
    pendingHighlight = { kind: "idle" };
    itemCommentOpen = true;
  }

  function saveItemComment(comment: string) {
    if (!session) return;
    session.add(itemCommentInput(index, comment));
    itemCommentOpen = false;
  }

  function cancelItemComment() {
    itemCommentOpen = false;
  }

  function addRegion(
    shape: RegionShape,
    coords: { x: number; y: number; width: number; height: number },
    comment: string,
  ) {
    if (!session) return;
    session.add(imageRegionInput(index, shape, coords, comment));
    drawMode = "off";
  }

  function captureHighlight() {
    if (!descriptionEl) return;
    const captured = captureSelection(descriptionEl);
    if (!captured) return;
    pendingHighlight = { kind: "pending", ...captured };
  }

  function saveHighlight(comment: string) {
    if (!session || pendingHighlight.kind !== "pending") return;
    session.add(
      textHighlightInput(
        "item",
        index,
        pendingHighlight.char_start,
        pendingHighlight.char_end,
        pendingHighlight.text_snippet,
        comment,
      ),
    );
    pendingHighlight = { kind: "idle" };
  }

  function cancelHighlight() {
    pendingHighlight = { kind: "idle" };
  }
</script>

<article
  id={itemId}
  class="bv-item"
  class:bv-item-stacked={layout === "stacked"}
  class:bv-item-carousel={layout === "carousel"}
  aria-labelledby={`${itemId}-label`}
>
  <header class="bv-item-header">
    <span class="bv-item-index" aria-hidden="true">{(index + 1).toString().padStart(2, "0")}</span>
    <h2 id={`${itemId}-label`} class="bv-item-title">
      {item.title ?? `Item ${index + 1}`}
    </h2>
  </header>

  {#if session && !readOnly && layout === "stacked" && isImage}
    <ReviewTools
      mode={drawMode}
      onSetMode={setDrawMode}
      onAddComment={openItemComment}
      onCaptureHighlight={captureHighlight}
      {canCaptureHighlight}
    />
  {:else if session && !readOnly && layout === "stacked" && !isImage}
    <ReviewTools
      mode="off"
      onSetMode={() => {}}
      onAddComment={openItemComment}
      onCaptureHighlight={captureHighlight}
      {canCaptureHighlight}
    />
  {/if}

  {#if !readOnly && itemCommentOpen}
    <CommentForm
      title={`Comment on item ${index + 1}`}
      placeholder="What about this item do you want to call out?"
      onSave={saveItemComment}
      onCancel={cancelItemComment}
    />
  {/if}

  {#if !readOnly && pendingHighlight.kind === "pending"}
    <CommentForm
      title="Annotate selected text"
      placeholder={`What about this passage do you want to call out?`}
      onSave={saveHighlight}
      onCancel={cancelHighlight}
    />
  {/if}

  <div class="bv-item-media">
    {#if isImage}
      <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_noninteractive_element_interactions -->
      <img
        src={item.src}
        alt={altText}
        loading={index < 2 ? "eager" : "lazy"}
        decoding="async"
        draggable="false"
        class:bv-item-img-zoomable={drawMode === "off"}
        onclick={drawMode === "off" ? () => (lightboxOpen = true) : undefined}
      />
      {#if drawMode === "off"}
        <button
          class="bv-item-expand"
          type="button"
          aria-label="View full size"
          onclick={(e) => { e.stopPropagation(); lightboxOpen = true; }}
        >
          <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="10,2 14,2 14,6" />
            <polyline points="6,14 2,14 2,10" />
            <line x1="14" y1="2" x2="9" y2="7" />
            <line x1="2" y1="14" x2="7" y2="9" />
          </svg>
        </button>
      {/if}
      {#if session}
        <AnnotationOverlay
          itemIndex={index}
          drafts={itemDrafts}
          onAddRegion={addRegion}
          drawShape={drawMode === "off" ? "circle" : drawMode}
          drawingEnabled={drawMode !== "off"}
        />
      {/if}
    {:else if isVideo}
      <!--
        Captions track intentionally omitted: the gallery's video items
        are screen-recorded UI captures (no spoken narration) provided by
        an automated agent that has no transcript to ship. The
        alternative — generating placeholder captions — would degrade
        the screen-reader experience by claiming captions exist when
        they don't. The aria-label below carries the human-authored alt
        text for assistive tech.
      -->
      <!-- svelte-ignore a11y_media_has_caption -->
      <video
        src={item.src}
        controls
        preload="metadata"
        playsinline
        aria-label={altText}
      ></video>
    {:else}
      <div class="bv-item-unsupported" role="status">
        <p>Unsupported asset type for <code>{item.src}</code></p>
      </div>
    {/if}
  </div>

  {#if item.description}
    <p class="bv-item-description" bind:this={descriptionEl}>{item.description}</p>
  {/if}

  {#if (item.properties && Object.keys(item.properties).length > 0) || item.metadata?.issue_url}
    <ItemDetails {item} />
  {/if}
</article>

{#if lightboxOpen && isImage}
  <Lightbox src={item.src} alt={altText} onClose={() => (lightboxOpen = false)} />
{/if}

<style>
  .bv-item {
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-4);
    background: var(--bv-bg);
    color: var(--bv-text);
  }
  .bv-item-stacked {
    max-width: var(--bv-max-content);
    margin: 0 auto;
    padding: var(--bv-space-8) var(--bv-space-5);
    border-bottom: 1px solid var(--bv-border);
    scroll-margin-top: 3.25rem; /* matches topbar height */
  }
  .bv-item-stacked:last-child {
    border-bottom: 0;
    padding-bottom: var(--bv-space-10);
  }
  /* The carousel cell wrapping this article is always 100% of the rail
   * (one item per page). Cap the inner content at 60rem and center it
   * so wide screens get a comfortable reading column without revealing
   * any portion of the adjacent items. */
  .bv-item-carousel {
    padding: var(--bv-space-5);
    height: 100%;
    box-sizing: border-box;
    max-width: 60rem;
    margin: 0 auto;
    width: 100%;
  }

  .bv-item-header {
    display: flex;
    align-items: baseline;
    gap: var(--bv-space-3);
  }
  .bv-item-index {
    font-family: var(--bv-font-mono);
    font-size: var(--bv-text-sm);
    color: var(--bv-text-muted);
    letter-spacing: 0.04em;
  }
  .bv-item-title {
    font-size: var(--bv-text-2xl);
    color: var(--bv-text);
    line-height: var(--bv-line-snug);
  }

  .bv-item-media {
    position: relative;
    background: var(--bv-surface);
    border: 1px solid var(--bv-border);
    border-radius: var(--bv-radius-md);
    box-shadow: var(--bv-shadow-sm);
  }
  .bv-item-media img,
  .bv-item-media video {
    width: 100%;
    height: auto;
    display: block;
    border-radius: var(--bv-radius-md);
  }
  .bv-item-img-zoomable {
    cursor: zoom-in;
  }

  .bv-item-expand {
    position: absolute;
    top: var(--bv-space-2);
    right: var(--bv-space-2);
    z-index: 3;
    width: 2rem;
    height: 2rem;
    border-radius: var(--bv-radius-md);
    background: rgba(0, 0, 0, 0.5);
    border: 1px solid rgba(255, 255, 255, 0.15);
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    opacity: 0;
    transition:
      opacity var(--bv-duration-quick) var(--bv-ease-out),
      background var(--bv-duration-quick) var(--bv-ease-out),
      transform var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-item-media:hover .bv-item-expand,
  .bv-item-expand:focus-visible {
    opacity: 1;
  }
  .bv-item-expand:hover {
    background: rgba(0, 0, 0, 0.7);
    transform: scale(1.1);
  }
  .bv-item-expand:focus-visible {
    outline: 2px solid var(--bv-accent);
    outline-offset: 2px;
  }
  .bv-item-unsupported {
    padding: var(--bv-space-6);
    color: var(--bv-text-muted);
    text-align: center;
  }
  .bv-item-unsupported code {
    background: var(--bv-surface-2);
    padding: 1px var(--bv-space-1);
    border-radius: var(--bv-radius-sm);
  }

  .bv-item-description {
    margin: 0;
    color: var(--bv-text-dim);
    font-size: var(--bv-text-base);
    line-height: var(--bv-line-normal);
    white-space: pre-wrap;
    max-width: var(--bv-max-narrow);
    /* Allow text selection — required for the text_highlight
     * annotation tool. */
    user-select: text;
  }
</style>
