<script lang="ts">
  import { onMount } from "svelte";

  interface NavItem {
    label: string;
    href: string;
  }

  interface Props {
    nav: ReadonlyArray<NavItem>;
    appHost: string;
  }

  let { nav, appHost }: Props = $props();

  let open = $state(false);
  let panel: HTMLElement | undefined = $state(undefined);
  let trigger: HTMLButtonElement | undefined = $state(undefined);

  function toggle() {
    open = !open;
  }

  function close() {
    open = false;
  }

  $effect(() => {
    if (typeof document === "undefined") return;
    document.body.style.overflow = open ? "hidden" : "";
    return () => {
      document.body.style.overflow = "";
    };
  });

  $effect(() => {
    if (!open) return;
    // After paint, move focus into the panel.
    queueMicrotask(() => {
      const first = panel?.querySelector<HTMLElement>(
        'a[href], button:not([disabled])',
      );
      first?.focus();
    });
  });

  function onKeydown(e: KeyboardEvent) {
    if (!open) return;
    if (e.key === "Escape") {
      e.preventDefault();
      close();
      trigger?.focus();
      return;
    }
    if (e.key === "Tab" && panel) {
      const focusable = Array.from(
        panel.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((el) => !el.hasAttribute("disabled"));
      if (focusable.length === 0) return;
      const first = focusable[0]!;
      const last = focusable[focusable.length - 1]!;
      const active = document.activeElement as HTMLElement | null;
      if (e.shiftKey && active === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && active === last) {
        e.preventDefault();
        first.focus();
      }
    }
  }

  onMount(() => {
    document.addEventListener("keydown", onKeydown);
    return () => document.removeEventListener("keydown", onKeydown);
  });

  function onLinkClick() {
    close();
  }
</script>

<button
  bind:this={trigger}
  type="button"
  class="trigger"
  aria-label={open ? "Close menu" : "Open menu"}
  aria-expanded={open}
  aria-controls="mobile-nav-panel"
  onclick={toggle}
>
  <svg
    width="20"
    height="20"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    {#if open}
      <line x1="6" y1="6" x2="18" y2="18"></line>
      <line x1="18" y1="6" x2="6" y2="18"></line>
    {:else}
      <line x1="3" y1="6" x2="21" y2="6"></line>
      <line x1="3" y1="12" x2="21" y2="12"></line>
      <line x1="3" y1="18" x2="21" y2="18"></line>
    {/if}
  </svg>
</button>

{#if open}
  <button
    type="button"
    class="scrim"
    aria-label="Close menu"
    onclick={close}
    tabindex="-1"
  ></button>
  <div
    bind:this={panel}
    id="mobile-nav-panel"
    class="panel"
    role="dialog"
    aria-modal="true"
    aria-label="Site navigation"
  >
    <ul class="links" role="list">
      {#each nav as item (item.href)}
        <li>
          <a href={item.href} onclick={onLinkClick}>{item.label}</a>
        </li>
      {/each}
      <li>
        <a href={`https://${appHost}`} onclick={onLinkClick}>Sign in</a>
      </li>
    </ul>
  </div>
{/if}

<style>
  .trigger {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    /* 44x44 minimum touch target (WCAG 2.5.5 / iOS HIG) */
    min-width: 44px;
    min-height: 44px;
    padding: 0;
    background: transparent;
    border: 1px solid var(--bv-border, #232735);
    border-radius: 6px;
    color: var(--bv-text, #e6e8ef);
    cursor: pointer;
  }
  .trigger:hover {
    background: var(--bv-surface, #11141b);
  }
  .trigger:focus-visible {
    outline: 2px solid var(--bv-accent-strong, #5b85ff);
    outline-offset: 2px;
  }

  .scrim {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    border: 0;
    padding: 0;
    margin: 0;
    cursor: default;
    z-index: 40;
  }

  .panel {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    width: min(20rem, 90vw);
    background: var(--bv-bg, #0b0d12);
    border-left: 1px solid var(--bv-border, #232735);
    padding: 4.5rem 1.25rem 1.25rem;
    z-index: 50;
    overflow-y: auto;
  }

  .links {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .links a {
    display: flex;
    align-items: center;
    min-height: 44px;
    padding: 0.5rem 0.75rem;
    color: var(--bv-text, #e6e8ef);
    text-decoration: none;
    border-radius: 6px;
    font-size: 1rem;
  }
  .links a:hover {
    background: var(--bv-surface, #11141b);
  }
  .links a:focus-visible {
    outline: 2px solid var(--bv-accent-strong, #5b85ff);
    outline-offset: -2px;
  }
</style>
