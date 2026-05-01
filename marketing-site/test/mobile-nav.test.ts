/**
 * Source-level checks that the marketing site ships a mobile nav (hamburger
 * menu) below the `md` breakpoint. A prior fix to Header.astro restored the
 * nav on phones (it had been hidden with no replacement); these tests guard
 * the regression.
 */
import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, '..');

const HEADER = readFileSync(join(ROOT, 'src/components/Header.astro'), 'utf8');
const MOBILE_NAV = readFileSync(join(ROOT, 'src/components/MobileNav.svelte'), 'utf8');

describe('Header.astro mounts the mobile nav island', () => {
  it('imports MobileNav.svelte', () => {
    expect(HEADER).toMatch(/import\s+MobileNav\s+from\s+["']\.\/MobileNav\.svelte["']/);
  });

  it('hydrates the island (client: directive)', () => {
    expect(HEADER).toMatch(/<MobileNav[^>]*\sclient:/);
  });

  it('mounts the island only below the md breakpoint', () => {
    // The Tailwind utility "md:hidden" hides at >=md; we want the island
    // visible on phones, which means it must live in a "md:hidden" container.
    expect(HEADER).toMatch(/md:hidden[^>]*>[\s\S]*<MobileNav/);
  });

  it('passes NAV and appHost through to the island', () => {
    expect(HEADER).toMatch(/<MobileNav[^>]*\bnav=\{NAV\}/);
    expect(HEADER).toMatch(/<MobileNav[^>]*\bappHost=\{SITE\.appHost\}/);
  });
});

describe('MobileNav.svelte accessibility contract', () => {
  it('exposes aria-expanded so AT can announce open/closed state', () => {
    expect(MOBILE_NAV).toMatch(/aria-expanded=/);
  });

  it('uses aria-controls to link the trigger to its panel', () => {
    expect(MOBILE_NAV).toMatch(/aria-controls=/);
    // The id named in aria-controls must exist on the panel.
    expect(MOBILE_NAV).toMatch(/id=["']mobile-nav-panel["']/);
    expect(MOBILE_NAV).toMatch(/aria-controls=["']mobile-nav-panel["']/);
  });

  it('marks the panel as a modal dialog', () => {
    expect(MOBILE_NAV).toMatch(/role=["']dialog["']/);
    expect(MOBILE_NAV).toMatch(/aria-modal=["']true["']/);
  });

  it('handles Escape to close', () => {
    expect(MOBILE_NAV).toContain('Escape');
  });

  it('implements a Tab focus trap', () => {
    // The trap is the reason this island exists as JS rather than
    // CSS-only details/summary; assert it stays in place.
    expect(MOBILE_NAV).toMatch(/\bTab\b/);
    expect(MOBILE_NAV).toMatch(/shiftKey/);
  });

  it('declares a 44x44 minimum touch target on the trigger', () => {
    // WCAG 2.5.5 / iOS HIG minimum.
    expect(MOBILE_NAV).toMatch(/min-width:\s*44px/);
    expect(MOBILE_NAV).toMatch(/min-height:\s*44px/);
  });

  it('closes on link click so the panel disappears after navigation', () => {
    expect(MOBILE_NAV).toMatch(/onLinkClick|onclick=\{close\}/);
  });

  it('renders Sign in alongside the NAV entries', () => {
    expect(MOBILE_NAV).toContain('Sign in');
  });
});
