<script lang="ts">
  interface Props {
    plan: string;
    href: string;
    label: string;
    primary?: boolean;
  }
  let { plan, href, label, primary = false }: Props = $props();
  // Click capture is handled globally in BaseLayout via the [data-cta]
  // listener — no per-element handler here, otherwise the event fires twice.
</script>

<a
  href={href}
  class="cta"
  data-primary={primary ? "true" : "false"}
  data-cta={`pricing-${plan}`}
  rel={href.startsWith("http") ? "noopener noreferrer" : null}
  target={href.startsWith("http") ? "_blank" : null}
>
  {label}
</a>

<style>
  .cta {
    display: inline-flex;
    justify-content: center;
    align-items: center;
    width: 100%;
    text-align: center;
    padding: 0.7rem 1rem;
    border-radius: 8px;
    font-size: 0.9rem;
    font-weight: 500;
    text-decoration: none;
    transition: background 120ms ease, color 120ms ease;
  }
  .cta[data-primary="true"] {
    background: #5b85ff;
    color: #0b0d12;
  }
  .cta[data-primary="true"]:hover {
    background: #8aa9ff;
  }
  .cta[data-primary="false"] {
    border: 1px solid #232735;
    color: #e6e8ef;
  }
  .cta[data-primary="false"]:hover {
    background: #181c25;
  }
</style>
