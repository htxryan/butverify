<script lang="ts">
  /* Annotation overlay — visual structure ONLY. The annotation client
   * (epic pebble-4nwj / EV2-E-7) will replace this with an interactive
   * canvas + region drawing logic. We ship the overlay shell now so the
   * design tokens, z-stack, and DOM structure are stable when interaction
   * lands.
   *
   * This component renders nothing visible by default. It exists at the
   * right z-index inside the item-media container so future annotations
   * can be appended without restructuring the DOM. The element accepts
   * pointer events on the click target only — the overlay itself is
   * pointer-events:none so the underlying <img> / <video> stays
   * interactive (right-click save, controls).
   */
  // Accept extra props (currently `item`, reserved for E3 annotation
  // interaction logic) without warning. Svelte 5's $props() rest-spread
  // lets us forward future props to a future implementation without
  // breaking the prop contract.
  let props: { index: number; [key: string]: unknown } = $props();
  let index = $derived(props.index);
</script>

<div
  class="bv-annotation-overlay"
  aria-hidden="true"
  data-bv-annotation-host="true"
  data-bv-item-index={index}
></div>

<style>
  .bv-annotation-overlay {
    position: absolute;
    inset: 0;
    pointer-events: none;
    /* Future: annotation interaction surface. Children that flip
     * pointer-events:auto will receive clicks. This wrapper just
     * provides the coordinate space (matching the underlying image)
     * for normalized region positioning. */
    background: transparent;
    z-index: 1;
  }
</style>
