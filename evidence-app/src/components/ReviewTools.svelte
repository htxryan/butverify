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
   * At wide viewports the four buttons render inline.
   * At narrow viewports (< 640px) they collapse into a single
   * "Annotate ▾" trigger that opens a dropdown — prevents the
   * Highlight button from wrapping to a second line on mobile.
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

  let dropdownOpen = $state(false);

  const triggerLabel = $derived(
    mode === "circle" ? "Circle" :
    mode === "rect"   ? "Rectangle" :
    "Annotate"
  );

  function selectComment() { dropdownOpen = false; onAddComment(); }
  function selectMode(m: "circle" | "rect") { dropdownOpen = false; onSetMode(mode === m ? "off" : m); }
  function selectHighlight() { if (!canCaptureHighlight) return; dropdownOpen = false; onCaptureHighlight(); }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") { e.stopPropagation(); dropdownOpen = false; }
  }

  function handleFocusOut(e: FocusEvent) {
    // Close when focus leaves the compact wrapper entirely.
    if (!(e.currentTarget as Element).contains(e.relatedTarget as Node | null)) {
      dropdownOpen = false;
    }
  }
</script>

<div class="bv-review-tools" role="toolbar" aria-label="Annotation tools">

  <!-- ── Full button row — hidden below 640 px ──────────────────── -->
  <div class="bv-review-tools__full" role="presentation">
    <button
      type="button"
      class="bv-review-tools__btn"
      aria-label="Add a comment about this item"
      onclick={onAddComment}
    >Comment</button>
    <button
      type="button"
      class="bv-review-tools__btn"
      class:bv-review-tools__btn--active={mode === "circle"}
      aria-pressed={mode === "circle"}
      aria-label="Draw a circle on the image"
      onclick={() => onSetMode(mode === "circle" ? "off" : "circle")}
    >Circle</button>
    <button
      type="button"
      class="bv-review-tools__btn"
      class:bv-review-tools__btn--active={mode === "rect"}
      aria-pressed={mode === "rect"}
      aria-label="Draw a rectangle on the image"
      onclick={() => onSetMode(mode === "rect" ? "off" : "rect")}
    >Rectangle</button>
    <button
      type="button"
      class="bv-review-tools__btn"
      aria-label="Annotate selected text in this item's description"
      onclick={onCaptureHighlight}
      disabled={!canCaptureHighlight}
      title={canCaptureHighlight ? "Annotate the highlighted text" : "Select text in the description first"}
    >Highlight</button>
  </div>

  <!-- ── Compact dropdown — shown below 640 px ──────────────────── -->
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div
    class="bv-review-tools__compact"
    role="presentation"
    onkeydown={handleKeydown}
    onfocusout={handleFocusOut}
  >
    <button
      type="button"
      class="bv-review-tools__btn bv-review-tools__trigger"
      class:bv-review-tools__btn--active={mode !== "off"}
      aria-haspopup="true"
      aria-expanded={dropdownOpen}
      aria-label="Annotation tools"
      onclick={() => (dropdownOpen = !dropdownOpen)}
    >
      {triggerLabel}<span class="bv-chevron" aria-hidden="true">{dropdownOpen ? "▴" : "▾"}</span>
    </button>

    {#if dropdownOpen}
      <ul class="bv-review-tools__menu" aria-label="Annotation tools">
        <li>
          <button type="button" class="bv-review-tools__menu-item" onclick={selectComment}>
            Comment
          </button>
        </li>
        <li>
          <button
            type="button"
            class="bv-review-tools__menu-item"
            class:bv-review-tools__menu-item--active={mode === "circle"}
            aria-pressed={mode === "circle"}
            onclick={() => selectMode("circle")}
          >Circle</button>
        </li>
        <li>
          <button
            type="button"
            class="bv-review-tools__menu-item"
            class:bv-review-tools__menu-item--active={mode === "rect"}
            aria-pressed={mode === "rect"}
            onclick={() => selectMode("rect")}
          >Rectangle</button>
        </li>
        <li>
          <button
            type="button"
            class="bv-review-tools__menu-item"
            onclick={selectHighlight}
            disabled={!canCaptureHighlight}
            title={canCaptureHighlight ? "Annotate the highlighted text" : "Select text in the description first"}
          >Highlight</button>
        </li>
      </ul>
    {/if}
  </div>

</div>

<style>
  .bv-review-tools {
    margin-bottom: var(--bv-space-3);
  }

  /* ── Full button row ────────────────────────────────────────────── */
  .bv-review-tools__full {
    display: flex;
    flex-wrap: nowrap;
    gap: var(--bv-space-2);
  }

  /* ── Compact dropdown ───────────────────────────────────────────── */
  .bv-review-tools__compact {
    display: none;
    position: relative;
  }

  @media (max-width: 639px) {
    .bv-review-tools__full    { display: none; }
    .bv-review-tools__compact { display: block; }
  }

  /* ── Shared button base ─────────────────────────────────────────── */
  .bv-review-tools__btn {
    background: var(--bv-surface);
    border: 1px solid var(--bv-border);
    color: var(--bv-text);
    border-radius: var(--bv-radius-pill);
    padding: var(--bv-space-1) var(--bv-space-3);
    font-size: var(--bv-text-sm);
    cursor: pointer;
    min-height: 44px;
    transition:
      background-color var(--bv-duration-quick) var(--bv-ease-out),
      border-color     var(--bv-duration-quick) var(--bv-ease-out),
      color            var(--bv-duration-quick) var(--bv-ease-out);
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

  /* ── Compact trigger ────────────────────────────────────────────── */
  .bv-review-tools__trigger {
    display: inline-flex;
    align-items: center;
    gap: var(--bv-space-1);
  }
  .bv-chevron {
    font-size: 0.65em;
  }

  /* ── Dropdown menu ──────────────────────────────────────────────── */
  .bv-review-tools__menu {
    position: absolute;
    top: calc(100% + var(--bv-space-1));
    left: 0;
    z-index: 100;
    list-style: none;
    margin: 0;
    padding: var(--bv-space-1);
    min-width: 10rem;
    background: var(--bv-bg);
    border: 1px solid var(--bv-border-strong);
    border-radius: var(--bv-radius-md);
    box-shadow: var(--bv-shadow-md);
  }
  .bv-review-tools__menu li { list-style: none; }

  .bv-review-tools__menu-item {
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
      color      var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-review-tools__menu-item:hover:not(:disabled),
  .bv-review-tools__menu-item:focus-visible {
    background: var(--bv-surface-2);
  }
  .bv-review-tools__menu-item--active {
    color: var(--bv-accent-strong);
    background: var(--bv-accent-soft);
  }
  .bv-review-tools__menu-item:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
