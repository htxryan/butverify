<script lang="ts">
  /* pebble-4nwj — Interactive image_region drawing overlay.
   *
   * The overlay sits on top of an image inside `.bv-item-media` (which
   * is `position:relative`). We render:
   *   1. Already-saved regions (from the draft list) as static
   *      circle/rect outlines, so the user can see what they've
   *      already marked.
   *   2. While the user is drawing, a live preview of the in-progress
   *      shape.
   *   3. After draw-release, a CommentForm popover positioned near
   *      the region; saving the comment commits the region to the
   *      draft store.
   *
   * Coordinates are normalized 0.0–1.0 against the rendered image
   * size, so they survive viewport resize and CSS object-fit. We
   * read clientWidth/Height from the host element (which matches the
   * image because the image is `width:100%` inside it).
   *
   * Pointer events: we accept all pointer types (mouse, touch, pen)
   * via the Pointer Events API. Touch hit targets are at least 32px
   * because mobile reviewers will draw with their finger.
   *
   * Keyboard alternative: a "Add region" button (rendered above the
   * image when reviews are enabled) opens a centered 20%-wide region
   * the user can confirm-or-cancel — keyboard reviewers do not have
   * to draw. This is also the path screen-reader users take.
   */
  import type { AnnotationDraft, RegionShape } from "../lib/annotations.js";
  import CommentForm from "./CommentForm.svelte";

  let {
    itemIndex,
    drafts,
    onAddRegion,
    drawShape,
    drawingEnabled = true,
  }: {
    itemIndex: number;
    drafts: AnnotationDraft[];
    onAddRegion: (
      shape: RegionShape,
      coords: { x: number; y: number; width: number; height: number },
      comment: string,
    ) => void;
    drawShape: RegionShape;
    drawingEnabled?: boolean;
  } = $props();

  // Existing regions for THIS item only.
  let savedRegions = $derived(
    drafts.filter(
      (d) => d.type === "image_region" && d.item_index === itemIndex,
    ),
  );

  // In-progress draw state.
  type DrawState =
    | { kind: "idle" }
    | { kind: "drawing"; startX: number; startY: number; curX: number; curY: number }
    | {
        kind: "pending";
        shape: RegionShape;
        x: number;
        y: number;
        width: number;
        height: number;
      };

  let draw: DrawState = $state({ kind: "idle" });
  let hostEl: HTMLDivElement | null = $state(null);

  // Translate a pointer event to normalized 0–1 coordinates against
  // the host element. Clamped so a slightly-out-of-bounds drag (the
  // user dragged past the image edge) still lands inside [0,1].
  function normalize(e: PointerEvent): { x: number; y: number } | null {
    if (!hostEl) return null;
    const rect = hostEl.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return null;
    const x = (e.clientX - rect.left) / rect.width;
    const y = (e.clientY - rect.top) / rect.height;
    return {
      x: Math.max(0, Math.min(1, x)),
      y: Math.max(0, Math.min(1, y)),
    };
  }

  function onPointerDown(e: PointerEvent) {
    if (!drawingEnabled) return;
    if (draw.kind !== "idle") return;
    // Left button only; touch + pen don't expose `button`.
    if (e.pointerType === "mouse" && e.button !== 0) return;
    const p = normalize(e);
    if (!p) return;
    e.preventDefault();
    (e.target as Element).setPointerCapture?.(e.pointerId);
    draw = { kind: "drawing", startX: p.x, startY: p.y, curX: p.x, curY: p.y };
  }

  function onPointerMove(e: PointerEvent) {
    if (draw.kind !== "drawing") return;
    const p = normalize(e);
    if (!p) return;
    draw = { ...draw, curX: p.x, curY: p.y };
  }

  function onPointerUp(e: PointerEvent) {
    if (draw.kind !== "drawing") return;
    (e.target as Element).releasePointerCapture?.(e.pointerId);
    const dx = Math.abs(draw.curX - draw.startX);
    const dy = Math.abs(draw.curY - draw.startY);
    // Drag too small → treat as a click and seed a 8% box so single
    // taps aren't lost. Keeps mobile users from needing perfect drag
    // distance.
    const minDrag = 0.01;
    if (dx < minDrag && dy < minDrag) {
      const w = 0.08;
      const h = 0.08;
      const x = Math.max(0, Math.min(1 - w, draw.startX - w / 2));
      const y = Math.max(0, Math.min(1 - h, draw.startY - h / 2));
      draw = { kind: "pending", shape: drawShape, x, y, width: w, height: h };
      return;
    }
    const x = Math.min(draw.startX, draw.curX);
    const y = Math.min(draw.startY, draw.curY);
    const width = dx;
    const height = dy;
    draw = { kind: "pending", shape: drawShape, x, y, width, height };
  }

  function onPointerCancel() {
    if (draw.kind === "drawing") draw = { kind: "idle" };
  }

  function cancelPending() {
    draw = { kind: "idle" };
  }

  function commitPending(comment: string) {
    if (draw.kind !== "pending") return;
    onAddRegion(
      draw.shape,
      { x: draw.x, y: draw.y, width: draw.width, height: draw.height },
      comment,
    );
    draw = { kind: "idle" };
  }

  // Position the popover below (or flip above) the annotation region.
  //
  // Horizontal strategy: snap between left/center/right alignment
  // so the popover never extends past the image edge.
  //   cx < 30%  → left-align  (extends rightward, max-width = space to right edge)
  //   cx > 70%  → right-align (extends leftward,  max-width = space to left edge)
  //   otherwise → center       (max-width = 2× distance to nearest edge)
  // This prevents the translated left value from going negative at
  // narrow viewports where the popover can be wider than the image.
  function popoverStyle(): string {
    if (draw.kind !== "pending") return "";
    const cx = (draw.x + draw.width / 2) * 100;

    let xShift: string;
    let maxW: string;
    if (cx < 30) {
      xShift = "0";
      maxW = `min(26rem, calc(100% - ${cx.toFixed(1)}%))`;
    } else if (cx > 70) {
      xShift = "-100%";
      maxW = `min(26rem, ${cx.toFixed(1)}%)`;
    } else {
      const half = Math.min(cx, 100 - cx);
      xShift = "-50%";
      maxW = `min(26rem, ${(2 * half).toFixed(1)}%)`;
    }

    if (draw.y + draw.height / 2 > 0.5) {
      const top = draw.y * 100;
      return `left:${cx.toFixed(1)}%;top:${top.toFixed(1)}%;max-width:${maxW};transform:translate(${xShift},calc(-100% - var(--bv-space-2)));`;
    }
    const top = (draw.y + draw.height) * 100;
    return `left:${cx.toFixed(1)}%;top:${top.toFixed(1)}%;max-width:${maxW};transform:translate(${xShift},var(--bv-space-2));`;
  }

  // For circles we render a rectangular bounding box during the draw
  // and a circle once committed. The control-plane stores a circle as
  // its bounding box (region_x = left, region_y = top, region_width =
  // diameter, region_height = diameter) so the wire format is the
  // same — region_shape just controls render style.
  function shapePathFor(d: AnnotationDraft): string {
    const x = (d.region_x ?? 0) * 100;
    const y = (d.region_y ?? 0) * 100;
    const w = (d.region_width ?? 0) * 100;
    const h = (d.region_height ?? 0) * 100;
    return `left:${x}%; top:${y}%; width:${w}%; height:${h}%;`;
  }

  // Live preview shape style during drawing.
  function liveStyle(): string {
    if (draw.kind !== "drawing") return "display:none;";
    const x = Math.min(draw.startX, draw.curX) * 100;
    const y = Math.min(draw.startY, draw.curY) * 100;
    const w = Math.abs(draw.curX - draw.startX) * 100;
    const h = Math.abs(draw.curY - draw.startY) * 100;
    return `left:${x}%; top:${y}%; width:${w}%; height:${h}%;`;
  }
</script>

<div
  bind:this={hostEl}
  class="bv-annotation-overlay"
  class:bv-annotation-overlay--enabled={drawingEnabled}
  class:bv-annotation-overlay--drawing={draw.kind === "drawing"}
  data-bv-annotation-host="true"
  data-bv-item-index={itemIndex}
  onpointerdown={onPointerDown}
  onpointermove={onPointerMove}
  onpointerup={onPointerUp}
  onpointercancel={onPointerCancel}
  role={drawingEnabled ? "application" : "presentation"}
  aria-label={drawingEnabled ? `Draw a region on item ${itemIndex + 1}` : ""}
>
  <!-- Saved regions (read-only outlines) -->
  {#each savedRegions as region (region.draft_id)}
    <div
      class="bv-annot-shape bv-annot-shape--saved"
      class:bv-annot-shape--circle={region.region_shape === "circle"}
      class:bv-annot-shape--rect={region.region_shape === "rect"}
      style={shapePathFor(region)}
      aria-hidden="true"
      title={region.comment}
    ></div>
  {/each}

  <!-- Live preview while drawing -->
  <div
    class="bv-annot-shape bv-annot-shape--live"
    class:bv-annot-shape--circle={drawShape === "circle"}
    class:bv-annot-shape--rect={drawShape === "rect"}
    style={liveStyle()}
    aria-hidden="true"
  ></div>

  <!-- Pending region (after release, before save) -->
  {#if draw.kind === "pending"}
    <div
      class="bv-annot-shape bv-annot-shape--pending"
      class:bv-annot-shape--circle={draw.shape === "circle"}
      class:bv-annot-shape--rect={draw.shape === "rect"}
      style={`left:${draw.x * 100}%; top:${draw.y * 100}%; width:${draw.width * 100}%; height:${draw.height * 100}%;`}
      aria-hidden="true"
    ></div>
    <div class="bv-annot-popover" style={popoverStyle()}>
      <CommentForm
        title={`Annotate ${draw.shape} on item ${itemIndex + 1}`}
        placeholder="What stands out about this region?"
        onSave={commitPending}
        onCancel={cancelPending}
      />
    </div>
  {/if}
</div>

<style>
  .bv-annotation-overlay {
    position: absolute;
    inset: 0;
    background: transparent;
    z-index: 2;
    /* Default: no pointer events so the underlying image is
     * interactive. The .--enabled modifier flips this on the host
     * once reviews are active. */
    pointer-events: none;
    touch-action: none;
  }
  .bv-annotation-overlay--enabled {
    pointer-events: auto;
    cursor: crosshair;
  }
  .bv-annotation-overlay--drawing {
    /* Block child pointer events during draw so the live shape
     * doesn't intercept the pointermove. */
    cursor: crosshair;
  }

  .bv-annot-shape {
    position: absolute;
    border: 2px solid var(--bv-annot-ring);
    background: var(--bv-annot-fill);
    pointer-events: none;
    box-sizing: border-box;
  }
  .bv-annot-shape--circle {
    border-radius: 50%;
  }
  .bv-annot-shape--rect {
    border-radius: 2px;
  }
  .bv-annot-shape--saved {
    /* Slightly more solid border for already-committed regions. */
    border-width: 2px;
    box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.4);
    pointer-events: auto;
    cursor: help;
  }
  .bv-annot-shape--live {
    border-style: dashed;
  }
  .bv-annot-shape--pending {
    border-color: var(--bv-accent-strong);
    background: rgba(58, 102, 240, 0.28);
  }

  .bv-annot-popover {
    position: absolute;
    z-index: 4;
    pointer-events: auto;
  }
</style>
