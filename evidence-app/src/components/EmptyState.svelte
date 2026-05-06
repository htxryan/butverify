<script lang="ts">
  // Empty-state surface. Defensive: the Go CLI rejects manifests with zero
  // items at parse time (templates: evidence.items must contain at least
  // one item), so this should never render in production. But it would
  // render if a hand-edited manifest gets pasted into the inline script
  // tag during a hotfix attempt. We surface a friendly explanation
  // rather than a blank page.
</script>

<section class="bv-empty" aria-label="Empty evidence gallery">
  <div class="bv-empty-card">
    <h2>No evidence captured</h2>
    <p>
      The manifest for this gallery is empty. The publishing CLI normally
      rejects this; if you are previewing a hand-authored manifest, add at
      least one item to the <code>items</code> array.
    </p>
  </div>
</section>

<style>
  .bv-empty {
    max-width: var(--bv-max-content);
    margin: 0 auto;
    padding: var(--bv-space-10) var(--bv-space-5);
  }
  .bv-empty-card {
    border: 1px dashed var(--bv-border-strong);
    border-radius: var(--bv-radius-lg);
    padding: var(--bv-space-8);
    text-align: center;
    color: var(--bv-text-dim);
  }
  .bv-empty-card h2 {
    margin: 0 0 var(--bv-space-3);
    font-size: var(--bv-text-xl);
    color: var(--bv-text);
  }
  .bv-empty-card p {
    max-width: 32rem;
    margin: 0 auto;
    line-height: var(--bv-line-normal);
  }
  .bv-empty-card code {
    background: var(--bv-surface-2);
    padding: 1px var(--bv-space-1);
    border-radius: var(--bv-radius-sm);
    font-size: 0.95em;
  }
</style>
