<script lang="ts">
  let {
    src,
    alt,
    onClose,
  }: { src: string; alt: string; onClose: () => void } = $props();

  $effect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = prev;
    };
  });

  function onBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose();
  }
</script>

<div
  class="bv-lightbox"
  role="dialog"
  aria-modal="true"
  aria-label="Full-size image"
  tabindex="-1"
  onclick={onBackdropClick}
  onkeydown={(e) => { if (e.key === "Escape") onClose(); }}
>
  <button
    class="bv-lightbox-close"
    type="button"
    onclick={onClose}
    aria-label="Close"
  >
    <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
      <line x1="3" y1="3" x2="13" y2="13" />
      <line x1="13" y1="3" x2="3" y2="13" />
    </svg>
  </button>
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_noninteractive_element_interactions -->
  <img class="bv-lightbox-img" {src} {alt} onclick={(e) => e.stopPropagation()} />
</div>

<style>
  .bv-lightbox {
    position: fixed;
    inset: 0;
    z-index: 200;
    background: rgba(0, 0, 0, 0.9);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--bv-space-8);
    cursor: zoom-out;
    animation: bv-lb-in var(--bv-duration-quick) var(--bv-ease-out);
  }
  @keyframes bv-lb-in {
    from { opacity: 0; }
    to { opacity: 1; }
  }
  @media (prefers-reduced-motion: reduce) {
    .bv-lightbox { animation: none; }
  }

  .bv-lightbox-img {
    max-width: min(100%, 95vw);
    max-height: 90vh;
    object-fit: contain;
    border-radius: var(--bv-radius-md);
    cursor: default;
    box-shadow: 0 16px 64px rgba(0, 0, 0, 0.8);
  }

  .bv-lightbox-close {
    position: absolute;
    top: var(--bv-space-4);
    right: var(--bv-space-4);
    width: 2.5rem;
    height: 2.5rem;
    border-radius: var(--bv-radius-pill);
    background: rgba(255, 255, 255, 0.12);
    border: 1px solid rgba(255, 255, 255, 0.18);
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: background var(--bv-duration-quick);
    flex-shrink: 0;
  }
  .bv-lightbox-close:hover {
    background: rgba(255, 255, 255, 0.22);
  }
  .bv-lightbox-close:focus-visible {
    outline: 2px solid rgba(255, 255, 255, 0.7);
    outline-offset: 2px;
  }
</style>
