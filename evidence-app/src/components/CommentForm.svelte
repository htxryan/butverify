<script lang="ts">
  /* pebble-4nwj — Generic comment form used for site_comment and
   * item_comment annotations. The image_region and text_highlight
   * forms reuse this component to capture the comment portion after
   * the geometry is captured.
   *
   * The form is minimal by design — a textarea and two buttons
   * (Cancel + Save). Submit-on-Enter is disabled because comments are
   * multi-line; users hit Save explicitly. This matches every other
   * "compose review" interaction reviewers expect (GitHub PR review,
   * Linear inline comment, Figma comment).
   */

  let {
    title,
    label = "Your comment",
    placeholder = "What do you want to call out?",
    onSave,
    onCancel,
    initialValue = "",
    busy = false,
  }: {
    title: string;
    label?: string;
    placeholder?: string;
    onSave: (comment: string) => void;
    onCancel: () => void;
    initialValue?: string;
    busy?: boolean;
  } = $props();

  // `initialValue` is captured once at mount; the form is short-lived
  // and the parent unmounts/remounts to reset content. Suppress the
  // state_referenced_locally warning explicitly because the behavior
  // is intentional (we don't want every keystroke from the parent
  // to clobber the user's in-progress text).
  // svelte-ignore state_referenced_locally
  let value = $state(initialValue);
  let inputEl: HTMLTextAreaElement | null = $state(null);

  // Auto-focus the textarea when the form mounts. Keeps the
  // keyboard-only path fast: the user invoked the form deliberately
  // (clicked "Add comment" or finished drawing a region), so taking
  // focus is expected behavior, not a focus-grab.
  $effect(() => {
    if (inputEl && !busy) {
      inputEl.focus();
    }
  });

  let canSave = $derived(value.trim().length > 0 && !busy);

  function save() {
    if (!canSave) return;
    onSave(value.trim());
  }

  function onKeydown(e: KeyboardEvent) {
    // Cmd/Ctrl+Enter saves — keyboard accelerator that matches the
    // GitHub PR-review convention. Plain Enter still inserts a newline.
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      save();
    } else if (e.key === "Escape") {
      e.preventDefault();
      onCancel();
    }
  }
</script>

<div
  class="bv-comment-form"
  role="dialog"
  aria-modal="false"
  aria-label={title}
>
  <header class="bv-comment-form__header">
    <h3>{title}</h3>
  </header>
  <label class="bv-comment-form__label">
    <span class="bv-visually-hidden">{label}</span>
    <textarea
      bind:this={inputEl}
      bind:value
      placeholder={placeholder}
      rows="4"
      maxlength="4096"
      aria-label={label}
      onkeydown={onKeydown}
      disabled={busy}
    ></textarea>
  </label>
  <footer class="bv-comment-form__footer">
    <div class="bv-comment-form__actions">
      <button type="button" class="bv-btn-icon bv-btn-cancel" onclick={onCancel} disabled={busy} aria-label="Cancel">
        <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <line x1="3" y1="3" x2="13" y2="13" />
          <line x1="13" y1="3" x2="3" y2="13" />
        </svg>
      </button>
      <button type="button" class="bv-btn-icon bv-btn-save" onclick={save} disabled={!canSave} aria-label="Save">
        <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="2,8.5 6,12.5 14,4" />
        </svg>
      </button>
    </div>
  </footer>
</div>

<style>
  .bv-comment-form {
    background: var(--bv-bg);
    border: 1px solid var(--bv-border-strong);
    border-radius: var(--bv-radius-md);
    box-shadow: var(--bv-shadow-md);
    padding: var(--bv-space-3) var(--bv-space-4) var(--bv-space-3);
    display: flex;
    flex-direction: column;
    gap: var(--bv-space-2);
    width: 100%;
    max-width: 26rem;
  }

  .bv-comment-form__header h3 {
    margin: 0;
    font-size: var(--bv-text-base);
    color: var(--bv-text);
  }

  .bv-comment-form__label {
    display: block;
  }
  .bv-comment-form__label textarea {
    width: 100%;
    box-sizing: border-box;
    resize: vertical;
    min-height: 5rem;
    border: 1px solid var(--bv-border);
    border-radius: var(--bv-radius-sm);
    padding: var(--bv-space-2);
    font-family: var(--bv-font-sans);
    font-size: var(--bv-text-base);
    color: var(--bv-text);
    background: var(--bv-bg);
    line-height: var(--bv-line-snug);
  }
  .bv-comment-form__label textarea:focus-visible {
    outline: 2px solid var(--bv-accent);
    outline-offset: 0;
    border-color: var(--bv-accent);
  }

  .bv-comment-form__footer {
    display: flex;
    justify-content: flex-end;
    align-items: center;
  }

  .bv-comment-form__actions {
    display: flex;
    gap: var(--bv-space-2);
  }

  .bv-btn-icon {
    width: 2.25rem;
    height: 2.25rem;
    border-radius: var(--bv-radius-sm);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    border: 1px solid var(--bv-border);
    background: transparent;
    transition: background var(--bv-duration-quick) var(--bv-ease-out),
                color var(--bv-duration-quick) var(--bv-ease-out),
                border-color var(--bv-duration-quick) var(--bv-ease-out);
  }
  .bv-btn-cancel {
    color: var(--bv-text-muted);
  }
  .bv-btn-cancel:hover:not(:disabled),
  .bv-btn-cancel:focus-visible {
    background: var(--bv-surface-2);
    color: var(--bv-text);
  }
  .bv-btn-save {
    color: #ffffff;
    background: var(--bv-accent);
    border-color: var(--bv-accent);
  }
  .bv-btn-save:hover:not(:disabled),
  .bv-btn-save:focus-visible {
    background: var(--bv-accent-strong);
    border-color: var(--bv-accent-strong);
  }
  .bv-btn-save:disabled {
    background: var(--bv-border-strong);
    border-color: var(--bv-border-strong);
    cursor: not-allowed;
  }
  .bv-btn-cancel:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .bv-visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
</style>
