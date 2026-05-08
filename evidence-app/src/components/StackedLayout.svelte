<script lang="ts">
  import type { EvidenceItem } from "../lib/manifest.js";
  import GalleryItem from "./GalleryItem.svelte";

  // Manifest-derived props (e.g., enable_reviews-driven UI) will land in
  // E3; for now we take only the items list.
  let { items }: { items: EvidenceItem[] } = $props();
</script>

<ol class="bv-stacked" aria-label="Evidence items, stacked layout">
  {#each items as item, idx (idx)}
    <li class="bv-stacked-item">
      <GalleryItem {item} index={idx} layout="stacked" />
    </li>
  {/each}
</ol>

<style>
  .bv-stacked {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
  }
  .bv-stacked-item {
    margin: 0;
  }
</style>
