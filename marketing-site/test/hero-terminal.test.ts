/**
 * Source-level checks for the landing-page hero visual.
 * The text-only hero read as pre-MVP; we now ship an animated terminal
 * mock that demonstrates local-first `bv push` and a browser opening the URL.
 *
 * These tests guard the acceptance criteria from the epic:
 *  - Hero shows a product visual on desktop (2-col layout)
 *  - Reduced-motion users see a still
 *  - The visual is pure CSS/HTML (no large video bundle → LCP stays fast)
 */
import { describe, expect, it } from 'vitest';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, '..');

const HERO_TERMINAL_PATH = join(ROOT, 'src/components/HeroTerminal.astro');
const INDEX_PATH = join(ROOT, 'src/pages/index.astro');

describe('HeroTerminal component exists and is wired into the landing page', () => {
  it('ships a HeroTerminal.astro component', () => {
    expect(existsSync(HERO_TERMINAL_PATH)).toBe(true);
  });

  it('the landing page imports and renders HeroTerminal', () => {
    const index = readFileSync(INDEX_PATH, 'utf8');
    expect(index).toMatch(
      /import\s+HeroTerminal\s+from\s+["']\.\.\/components\/HeroTerminal\.astro["']/,
    );
    expect(index).toMatch(/<HeroTerminal\s*\/>/);
  });

  it('hero uses a 2-column grid on md+ so the visual sits beside the copy', () => {
    const index = readFileSync(INDEX_PATH, 'utf8');
    // The hero <section> must contain a grid that becomes 2 columns at md.
    expect(index).toMatch(/grid[^"']*\bmd:grid-cols-2\b/);
  });
});

describe('HeroTerminal contents reinforce the CLI demo', () => {
  const html = readFileSync(HERO_TERMINAL_PATH, 'utf8');

  it('shows a `bv push` invocation', () => {
    expect(html).toMatch(/bv push/);
  });

  it('shows a localhost preview URL in the demo', () => {
    expect(html).toMatch(/127\.0\.0\.1/);
  });

  it('shows a local status — the default preview mode the hero is selling', () => {
    expect(html).toMatch(/\blocal\b/);
  });

  it('renders a terminal window and a browser frame mock', () => {
    expect(html).toMatch(/class=["'][^"']*\bterminal\b/);
    expect(html).toMatch(/class=["'][^"']*\bbrowser\b/);
  });
});

describe('HeroTerminal accessibility and motion safety', () => {
  const html = readFileSync(HERO_TERMINAL_PATH, 'utf8');

  it('exposes a single accessible name for the figure (role=img + aria-label)', () => {
    // The decorative inner shapes are aria-hidden; the wrapper carries
    // the description so screen readers get one coherent label.
    expect(html).toMatch(/role=["']img["']/);
    expect(html).toMatch(/aria-label=/);
  });

  it('marks decorative inner windows as aria-hidden', () => {
    expect(html).toMatch(/aria-hidden=["']true["']/);
  });

  it('honors prefers-reduced-motion (animations disabled to a still)', () => {
    // The acceptance criterion: "Reduced-motion users see a still."
    expect(html).toMatch(/@media\s*\(prefers-reduced-motion:\s*reduce\)/);
    // Ensure animations are explicitly disabled inside that block.
    const block = html.split('@media (prefers-reduced-motion: reduce)')[1] ?? '';
    expect(block).toMatch(/animation:\s*none/);
    // Reduced-motion final frame must show the browser opaque — a regression
    // that left .browser at opacity:0 would render the visual broken for
    // reduced-motion users (a still terminal with no browser preview).
    expect(block).toMatch(/\.browser[^}]*opacity:\s*1/);
    expect(block).toMatch(/\.out[^}]*opacity:\s*1/);
  });
});

describe('HeroTerminal is LCP-friendly (no heavy media)', () => {
  const html = readFileSync(HERO_TERMINAL_PATH, 'utf8');

  it('uses pure CSS/HTML — no <video>, no <img>', () => {
    // The epic flagged a video approach as one option but warned about LCP.
    // We chose CSS animation instead, which keeps LCP under 2.5s by default
    // because there is no extra media to download.
    expect(html).not.toMatch(/<video\b/);
    expect(html).not.toMatch(/<img\b/);
  });
});
