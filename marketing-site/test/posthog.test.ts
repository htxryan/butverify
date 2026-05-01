import { describe, expect, it, vi } from 'vitest';
import { captureCta, posthogSnippet } from '../src/lib/posthog.ts';

describe('posthogSnippet', () => {
  it('embeds the supplied key + host as JSON literals', () => {
    const snippet = posthogSnippet({
      key: 'phc_test123',
      host: 'https://eu.posthog.com',
    });
    expect(snippet).toContain('"phc_test123"');
    expect(snippet).toContain('"https://eu.posthog.com"');
  });

  it('includes the api_host config field that the SDK reads', () => {
    const snippet = posthogSnippet({
      key: 'k',
      host: 'https://us.i.posthog.com',
    });
    expect(snippet).toMatch(/api_host:\s*"https:\/\/us\.i\.posthog\.com"/);
  });

  it('escapes embedded quotes / closing script tags safely', () => {
    const snippet = posthogSnippet({
      key: 'evil"</script>',
      host: 'https://x.example',
    });
    expect(snippet).not.toContain('</script>');
    expect(snippet).toContain('\\"');
  });
});

describe('captureCta', () => {
  it('is a no-op when posthog is not loaded', () => {
    const w = (globalThis as unknown as { window?: object }).window;
    (globalThis as unknown as { window: object }).window = {};
    expect(() => captureCta({ name: 'cta_click', cta: 'x', location: '/' })).not.toThrow();
    if (w === undefined) {
      delete (globalThis as { window?: object }).window;
    } else {
      (globalThis as unknown as { window: object }).window = w;
    }
  });

  it('calls window.posthog.capture when present', () => {
    const capture = vi.fn();
    (globalThis as unknown as { window: unknown }).window = {
      posthog: { capture },
    };
    captureCta({
      name: 'cta_click',
      cta: 'pricing-pro',
      location: '/pricing',
    });
    expect(capture).toHaveBeenCalledWith('cta_click', {
      cta: 'pricing-pro',
      location: '/pricing',
    });
    delete (globalThis as { window?: unknown }).window;
  });
});
