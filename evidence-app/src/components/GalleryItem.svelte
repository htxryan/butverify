<script lang="ts">
  import type { EvidenceItem } from "../lib/manifest.js";
  import { isImageItem, isVideoItem } from "../lib/manifest.js";
  import AnnotationOverlay from "./AnnotationOverlay.svelte";
  import ItemDetails from "./ItemDetails.svelte";

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

  <div class="bv-item-media">
    {#if isImage}
      <img
        src={item.src}
        alt={altText}
        loading={index < 2 ? "eager" : "lazy"}
        decoding="async"
      />
      <!-- Annotation-overlay shells: visual structure only, no
           interaction in this epic. The annotation client (E3) will
           upgrade these to interactive canvas. -->
      <AnnotationOverlay {item} {index} />
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

  {#if item.description || (item.properties && Object.keys(item.properties).length > 0) || item.metadata?.issue_url}
    <ItemDetails {item} />
  {/if}
</article>

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
    padding: var(--bv-space-6) var(--bv-space-5);
    border-bottom: 1px solid var(--bv-border);
  }
  .bv-item-stacked:last-child {
    border-bottom: 0;
    padding-bottom: var(--bv-space-10);
  }
  .bv-item-carousel {
    flex: 0 0 100%;
    scroll-snap-align: start;
    padding: var(--bv-space-5);
    height: 100%;
    box-sizing: border-box;
  }
  @media (min-width: 720px) {
    .bv-item-carousel {
      flex-basis: 80%;
      max-width: 60rem;
    }
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
    font-size: var(--bv-text-xl);
    color: var(--bv-text);
    line-height: var(--bv-line-snug);
  }

  .bv-item-media {
    position: relative;
    background: var(--bv-surface);
    border: 1px solid var(--bv-border);
    border-radius: var(--bv-radius-md);
    overflow: hidden;
    box-shadow: var(--bv-shadow-sm);
  }
  .bv-item-media img,
  .bv-item-media video {
    width: 100%;
    height: auto;
    display: block;
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
</style>
