<script lang="ts">
  type Layout = "stacked" | "carousel";
  let { value = $bindable("stacked") }: { value: Layout } = $props();

  // Two-button radiogroup; arrow keys move selection so the control is
  // keyboard-equivalent to the click/tap path. We use role=radiogroup
  // (not a <select>) because the option set is fixed at two and the
  // visual presentation is icon-led.
  function onKeydown(e: KeyboardEvent) {
    if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
      e.preventDefault();
      value = "stacked";
    } else if (e.key === "ArrowRight" || e.key === "ArrowDown") {
      e.preventDefault();
      value = "carousel";
    }
  }
</script>

<!--
  Roving-tabindex pattern on the two child buttons: only the active button
  is in the tab order; arrow keys move selection. The radiogroup
  container itself does not need a tabindex (children manage focus), but
  Svelte's a11y linter wants one — set to -1 so the container can be
  focused programmatically without entering the tab cycle.
-->
<div
  role="radiogroup"
  aria-label="Gallery layout"
  tabindex="-1"
  class="bv-layout-switcher"
  onkeydown={onKeydown}
>
  <button
    type="button"
    role="radio"
    aria-checked={value === "stacked"}
    tabindex={value === "stacked" ? 0 : -1}
    class="bv-layout-button"
    class:bv-active={value === "stacked"}
    onclick={() => (value = "stacked")}
  >
    <svg viewBox="0 0 16 16" aria-hidden="true" width="16" height="16">
      <rect x="2" y="2" width="12" height="3" rx="1" fill="currentColor" />
      <rect x="2" y="6.5" width="12" height="3" rx="1" fill="currentColor" />
      <rect x="2" y="11" width="12" height="3" rx="1" fill="currentColor" />
    </svg>
    <span>Stacked</span>
  </button>
  <button
    type="button"
    role="radio"
    aria-checked={value === "carousel"}
    tabindex={value === "carousel" ? 0 : -1}
    class="bv-layout-button"
    class:bv-active={value === "carousel"}
    onclick={() => (value = "carousel")}
  >
    <svg viewBox="0 0 16 16" aria-hidden="true" width="16" height="16">
      <rect x="1" y="3" width="4" height="10" rx="1" fill="currentColor" opacity="0.5" />
      <rect x="6" y="3" width="4" height="10" rx="1" fill="currentColor" />
      <rect x="11" y="3" width="4" height="10" rx="1" fill="currentColor" opacity="0.5" />
    </svg>
    <span>Carousel</span>
  </button>
</div>

<style>
  .bv-layout-switcher {
    display: inline-flex;
    align-items: center;
    gap: 0;
    padding: 2px;
    background: var(--bv-surface-2);
    border: 1px solid var(--bv-border);
    border-radius: var(--bv-radius-md);
  }

  .bv-layout-button {
    display: inline-flex;
    align-items: center;
    gap: var(--bv-space-2);
    padding: var(--bv-space-2) var(--bv-space-3);
    font-size: var(--bv-text-sm);
    color: var(--bv-text-dim);
    background: transparent;
    border-radius: calc(var(--bv-radius-md) - 2px);
    transition: background var(--bv-duration-quick) var(--bv-ease-out),
      color var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-layout-button:hover {
    color: var(--bv-text);
  }
  .bv-layout-button.bv-active {
    background: var(--bv-bg);
    color: var(--bv-text);
    box-shadow: var(--bv-shadow-sm);
  }
</style>
