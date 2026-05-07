<script lang="ts">
  /* pebble-4nwj — Per-item review-tool button bar.
   *
   * Sits above the image of each gallery item when reviews are
   * enabled, and offers four affordances:
   *   - Add comment (item_comment, no geometry)
   *   - Draw circle  (image_region, shape=circle)
   *   - Draw rect    (image_region, shape=rect)
   *   - Highlight selected text (text_highlight, scope=item)
   *
   * The "draw" buttons toggle the AnnotationOverlay's drawingEnabled
   * state via the shared `mode` prop. This keeps the keyboard path
   * coherent: pressing the button focuses the image and announces
   * the new mode via aria-live.
   */
  import type { RegionShape } from "../lib/annotations.js";

  let {
    mode,
    onSetMode,
    onAddComment,
    onCaptureHighlight,
    canCaptureHighlight,
  }: {
    mode: "off" | "circle" | "rect";
    onSetMode: (mode: "off" | "circle" | "rect") => void;
    onAddComment: () => void;
    onCaptureHighlight: () => void;
    canCaptureHighlight: boolean;
  } = $props();
</script>

<div class="bv-review-tools" role="toolbar" aria-label="Annotation tools">
  <button
    type="button"
    class="bv-review-tools__btn"
    class:bv-review-tools__btn--active={false}
    aria-label="Add a comment about this item"
    onclick={onAddComment}
  >
    Comment
  </button>
  <button
    type="button"
    class="bv-review-tools__btn"
    class:bv-review-tools__btn--active={mode === "circle"}
    aria-pressed={mode === "circle"}
    aria-label="Draw a circle on the image"
    onclick={() => onSetMode(mode === "circle" ? "off" : "circle")}
  >
    Circle
  </button>
  <button
    type="button"
    class="bv-review-tools__btn"
    class:bv-review-tools__btn--active={mode === "rect"}
    aria-pressed={mode === "rect"}
    aria-label="Draw a rectangle on the image"
    onclick={() => onSetMode(mode === "rect" ? "off" : "rect")}
  >
    Rectangle
  </button>
  <button
    type="button"
    class="bv-review-tools__btn"
    aria-label="Annotate selected text in this item's description"
    onclick={onCaptureHighlight}
    disabled={!canCaptureHighlight}
    title={canCaptureHighlight
      ? "Annotate the highlighted text"
      : "Select text in the description first"}
  >
    Highlight
  </button>
</div>

<style>
  .bv-review-tools {
    display: flex;
    flex-wrap: wrap;
    gap: var(--bv-space-2);
    margin-bottom: var(--bv-space-3);
  }

  .bv-review-tools__btn {
    background: var(--bv-surface);
    border: 1px solid var(--bv-border);
    color: var(--bv-text);
    border-radius: var(--bv-radius-pill);
    padding: var(--bv-space-1) var(--bv-space-3);
    font-size: var(--bv-text-sm);
    cursor: pointer;
    transition:
      background-color var(--bv-duration-quick) var(--bv-ease-out),
      border-color var(--bv-duration-quick) var(--bv-ease-out),
      color var(--bv-duration-quick) var(--bv-ease-out);
    min-height: 44px;
  }
  .bv-review-tools__btn:hover:not(:disabled),
  .bv-review-tools__btn:focus-visible {
    background: var(--bv-surface-2);
    border-color: var(--bv-border-strong);
  }
  .bv-review-tools__btn--active {
    background: var(--bv-accent-soft);
    border-color: var(--bv-accent);
    color: var(--bv-accent-strong);
  }
  .bv-review-tools__btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
