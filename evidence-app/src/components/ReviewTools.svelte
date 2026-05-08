<script lang="ts">
  /* pebble-4nwj — Per-item review-tool button bar.
   *
   * Sits above the image of each gallery item when reviews are
   * enabled, and offers four affordances behind a single dropdown:
   *   - Add comment (item_comment, no geometry)
   *   - Draw circle  (image_region, shape=circle)
   *   - Draw rect    (image_region, shape=rect)
   *   - Highlight selected text (text_highlight, scope=item)
   *
   * pebble-ogho — Always render as the collapsed dropdown trigger,
   * regardless of viewport width.
   * pebble-qxdc — Render the trigger as a standard icon button:
   * leading pencil icon, label, trailing chevron. Square-rounded
   * (not pill) to match other toolbar controls.
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
    if (!(e.currentTarget as Element).contains(e.relatedTarget as Node | null)) {
      dropdownOpen = false;
    }
  }
</script>

<div class="bv-review-tools" role="toolbar" aria-label="Annotation tools">
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
      <svg
        class="bv-review-tools__icon"
        viewBox="0 0 16 16"
        width="14"
        height="14"
        aria-hidden="true"
        fill="none"
        stroke="currentColor"
        stroke-width="1.5"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M11 1.5l3.5 3.5L5 14.5l-3.5.5.5-3.5L11 1.5z" />
        <path d="M9 3.5l3.5 3.5" />
      </svg>
      <span class="bv-review-tools__label">{triggerLabel}</span>
      <svg
        class="bv-review-tools__chevron"
        class:bv-review-tools__chevron--open={dropdownOpen}
        viewBox="0 0 16 16"
        width="12"
        height="12"
        aria-hidden="true"
        fill="none"
        stroke="currentColor"
        stroke-width="1.75"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <polyline points="3,6 8,11 13,6" />
      </svg>
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

  .bv-review-tools__compact {
    position: relative;
    display: block;
  }

  /* ── Shared button base ─────────────────────────────────────────── */
  .bv-review-tools__btn {
    background: var(--bv-surface);
    border: 1px solid var(--bv-border);
    color: var(--bv-text);
    border-radius: var(--bv-radius-md);
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
    gap: var(--bv-space-2);
  }
  .bv-review-tools__icon {
    flex: none;
    color: var(--bv-text-dim);
  }
  .bv-review-tools__btn--active .bv-review-tools__icon {
    color: inherit;
  }
  .bv-review-tools__label {
    line-height: 1;
  }
  .bv-review-tools__chevron {
    flex: none;
    color: var(--bv-text-dim);
    transition: transform var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-review-tools__chevron--open {
    transform: rotate(180deg);
  }
  .bv-review-tools__btn--active .bv-review-tools__chevron {
    color: inherit;
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
