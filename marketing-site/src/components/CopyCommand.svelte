<script lang="ts">
  import { captureCta } from "../lib/posthog";

  interface Props {
    command: string;
    label?: string;
    cta?: string;
  }
  let { command, label = "Copy", cta = "copy-command" }: Props = $props();

  let state = $state<"idle" | "copied" | "failed">("idle");
  let timer: ReturnType<typeof setTimeout> | null = null;

  async function onCopy() {
    try {
      await navigator.clipboard.writeText(command);
      state = "copied";
    } catch {
      state = "failed";
    }
    captureCta({ name: "cta_click", cta, location: window.location.pathname });
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => (state = "idle"), 1800);
  }
</script>

<div class="copy-row">
  <pre class="cmd"><code>{command}</code></pre>
  <button
    type="button"
    class="copy"
    aria-label={`${label} command to clipboard`}
    onclick={onCopy}
  >
    {state === "copied" ? "Copied" : state === "failed" ? "Press ⌘C" : label}
  </button>
</div>

<style>
  .copy-row {
    display: flex;
    align-items: stretch;
    gap: 0;
    border: 1px solid var(--bv-border, #232735);
    border-radius: 8px;
    overflow: hidden;
    background: var(--bv-surface, #11141b);
  }
  .cmd {
    margin: 0;
    padding: 0.875rem 1rem;
    flex: 1;
    border: 0;
    background: transparent;
    overflow-x: auto;
    font-size: 0.9rem;
  }
  .copy {
    border: 0;
    border-left: 1px solid var(--bv-border, #232735);
    background: var(--bv-surface-2, #181c25);
    color: var(--bv-text, #e6e8ef);
    padding: 0 1rem;
    min-width: 6.5rem;
    cursor: pointer;
    font-size: 0.85rem;
    font-weight: 500;
  }
  .copy:hover {
    background: #1f2433;
  }
</style>
