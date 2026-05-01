/**
 * Source-level checks for the scroll-triggered section reveal.
 *
 * Acceptance criteria from the epic:
 *  - The 3 below-the-fold sections (features / agents / closing CTA) fade in
 *    on scroll
 *  - Reduced-motion users see no transitions
 *  - No CLS — animation is transform-only, never height
 */
import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, '..');

function read(rel: string): string {
  return readFileSync(join(ROOT, rel), 'utf8');
}

describe('homepage marks the 3 below-the-fold sections for reveal', () => {
  const src = read('src/pages/index.astro');
  const matches = src.match(/data-bv-reveal\b/g) ?? [];

  it('has exactly three reveal targets — features, agents, closing CTA', () => {
    expect(matches.length).toBe(3);
  });

  it('does not mark the hero (which is above the fold)', () => {
    // Crude but effective: the hero is the first <section> in the file.
    // Anything before the first call to "border-t" is the hero region.
    const heroEnd = src.indexOf('border-t border-bv-border');
    expect(heroEnd).toBeGreaterThan(0);
    const heroRegion = src.slice(0, heroEnd);
    expect(heroRegion).not.toContain('data-bv-reveal');
  });
});

describe('global.css ships the reveal styles', () => {
  const css = read('src/styles/global.css');

  it('targets [data-bv-reveal] elements', () => {
    expect(css).toMatch(/\[data-bv-reveal\]/);
  });

  it('reveals on data-revealed="true" (set by the IntersectionObserver)', () => {
    expect(css).toMatch(/data-revealed=["']true["']/);
  });

  it('gates the initial hidden state on html[data-bv-js] so no-JS users see content', () => {
    expect(css).toMatch(/html\[data-bv-js\]/);
  });

  it('honors prefers-reduced-motion for the reveal target (final state shown)', () => {
    // The blanket reduced-motion rule strips
    // animation/transition globally. The reveal-specific block must still
    // ship its *own* @media block so [data-bv-reveal] sections end up at
    // opacity:1 (visible) under reduce-motion — without it, the initial
    // hidden state would persist and the section would never appear.
    const m = css.match(
      /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{\s*html\[data-bv-js\]\s+\[data-bv-reveal\]\s*\{([^}]+)\}/,
    );
    expect(m, 'expected a reveal-scoped reduced-motion block').not.toBeNull();
    const body = (m as RegExpMatchArray)[1];
    expect(body).toMatch(/transition:\s*none/);
    expect(body).toMatch(/opacity:\s*1/);
  });

  it('animates opacity + transform only — no height/width transitions (avoid CLS)', () => {
    // Find the [data-bv-reveal] base rule's transition property.
    const m = css.match(/\[data-bv-reveal\]\s*\{[^}]*transition:\s*([^;]+);/);
    expect(m).not.toBeNull();
    const transitionValue = (m as RegExpMatchArray)[1];
    expect(transitionValue).toMatch(/opacity/);
    expect(transitionValue).toMatch(/transform/);
    expect(transitionValue).not.toMatch(/\bheight\b/);
    expect(transitionValue).not.toMatch(/\bwidth\b/);
    expect(transitionValue).not.toMatch(/\btop\b/);
  });
});

describe('BaseLayout wires the IntersectionObserver', () => {
  const layout = read('src/layouts/BaseLayout.astro');

  it('sets html[data-bv-js] only when IntersectionObserver is supported', () => {
    expect(layout).toMatch(/IntersectionObserver["']?\s+in\s+window/);
    expect(layout).toMatch(/dataset\.bvJs/);
  });

  it('gates the observer on prefers-reduced-motion via the shared helper', () => {
    // The matchMedia call lives in the prefersReducedMotion()
    // helper so JS-driven motion across the marketing-site uses one gate.
    expect(layout).toMatch(/prefersReducedMotion\s*\(/);
    expect(layout).toMatch(/from\s+["']\.\.\/lib\/motion["']/);
  });

  it('observes [data-bv-reveal] and toggles data-revealed on intersection', () => {
    expect(layout).toMatch(/\[data-bv-reveal\]/);
    expect(layout).toMatch(/new IntersectionObserver/);
    expect(layout).toMatch(/dataset\.revealed/);
  });

  it('unobserves each element after first reveal (one-shot animation)', () => {
    expect(layout).toMatch(/io\.unobserve/);
  });
});
