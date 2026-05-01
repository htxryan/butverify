/**
 * Cross-cutting reduced-motion policy.
 *
 * Acceptance: a single global @media (prefers-reduced-motion: reduce) rule
 * in global.css strips animation, transition, and scroll-behavior from
 * everything on the page. JS-driven motion routes through the
 * prefersReducedMotion() helper in src/lib/motion.ts so we don't sprinkle
 * `window.matchMedia('(prefers-reduced-motion: reduce)')` across the codebase.
 */
import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { prefersReducedMotion } from '../src/lib/motion.ts';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, '..');

function read(rel: string): string {
  return readFileSync(join(ROOT, rel), 'utf8');
}

/** Extract the body of the *blanket* (universal selector) reduced-motion
 *  block — the one that targets `*, *::before, *::after`. There may be
 *  other component-scoped @media (prefers-reduced-motion: reduce) blocks
 *  in the same file; this picks out the global one specifically. */
function extractBlanketBlock(css: string): string {
  const re =
    /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{\s*\*\s*,\s*\*::before\s*,\s*\*::after\s*\{([^}]+)\}\s*\}/;
  const m = css.match(re);
  expect(
    m,
    'expected a blanket @media reduce block targeting *, *::before, *::after',
  ).not.toBeNull();
  // Capture group 1 exists by construction of the regex (the assertion
  // above narrows m to non-null). Coerce to string for callers.
  return (m as RegExpMatchArray)[1] as string;
}

describe('global.css ships the blanket reduced-motion gate', () => {
  const css = read('src/styles/global.css');
  const block = extractBlanketBlock(css);

  it('disables animation with !important so utility classes cannot override', () => {
    expect(block).toMatch(/animation:\s*none\s*!important/);
  });

  it('disables transition with !important', () => {
    expect(block).toMatch(/transition:\s*none\s*!important/);
  });

  it('forces auto scroll-behavior so smooth-scroll never fires', () => {
    expect(block).toMatch(/scroll-behavior:\s*auto\s*!important/);
  });
});

describe('prefersReducedMotion() helper', () => {
  type FakeMQ = { matches: boolean; media: string };
  type WindowSlot = { matchMedia?: (q: string) => FakeMQ };

  // Allow explicit `undefined` so we can simulate SSR (no window) under
  // exactOptionalPropertyTypes. The `?` alone treats undefined as
  // "absent vs present", which would force `delete g.window` for restore.
  const g = globalThis as { window?: WindowSlot | undefined };

  it('returns false when window is undefined (SSR safety)', () => {
    // The helper is the only entry point components should use; if it
    // throws or returns truthy under SSR, server-rendered output would
    // pre-suppress motion the client might want. The vitest config runs
    // marketing-site tests under the node environment, so window is
    // undefined by default — exactly the SSR shape we want to verify.
    const originalWindow = g.window;
    g.window = undefined;
    try {
      expect(prefersReducedMotion()).toBe(false);
    } finally {
      g.window = originalWindow;
    }
  });

  it('returns false when matchMedia is unavailable', () => {
    const originalWindow = g.window;
    g.window = {};
    try {
      expect(prefersReducedMotion()).toBe(false);
    } finally {
      g.window = originalWindow;
    }
  });

  it('returns the matchMedia result when matchMedia is available', () => {
    const originalWindow = g.window;
    g.window = {
      matchMedia: (query: string) => ({
        matches: query === '(prefers-reduced-motion: reduce)',
        media: query,
      }),
    };
    try {
      expect(prefersReducedMotion()).toBe(true);
    } finally {
      g.window = originalWindow;
    }
  });
});
