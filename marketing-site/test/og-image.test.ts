/**
 * Asserts that the marketing site emits a complete Open Graph + Twitter
 * image card so links to butverify.dev render a 2:1 hero on Twitter,
 * Slack, Discord, and LinkedIn instead of a tiny default favicon.
 *
 * Source-level checks (BaseLayout meta, per-page overrides, public assets)
 * — the build itself is validated by the og generator script during dev.
 */
import { describe, expect, it } from 'vitest';
import { existsSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, '..');

function read(rel: string): string {
  return readFileSync(join(ROOT, rel), 'utf8');
}

const PNG_SIGNATURE = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

function readPng(rel: string): { width: number; height: number; size: number } {
  const buf = readFileSync(join(ROOT, rel));
  expect(buf.subarray(0, 8).equals(PNG_SIGNATURE)).toBe(true);
  // PNG IHDR chunk starts at byte 8 (length=4, type=4, then 13 bytes of data:
  // width(4), height(4), bit_depth, color_type, ...). See RFC 2083 §3.2.
  const width = buf.readUInt32BE(16);
  const height = buf.readUInt32BE(20);
  return { width, height, size: buf.length };
}

describe('public/og PNG assets', () => {
  for (const name of ['default', 'pricing', 'agents']) {
    it(`ships /og/${name}.png as a 1200x630 PNG`, () => {
      const rel = `public/og/${name}.png`;
      expect(existsSync(join(ROOT, rel))).toBe(true);
      const { width, height, size } = readPng(rel);
      expect(width).toBe(1200);
      expect(height).toBe(630);
      // Twitter and LinkedIn cap social-card images at 5 MB. Stay well under.
      expect(size).toBeLessThan(1_000_000);
      // Empty/blank cards compress to a few KB; require non-trivial content.
      expect(size).toBeGreaterThan(20_000);
    });
  }

  it('og generator script is checked in for re-runs', () => {
    const scriptPath = join(ROOT, 'scripts/generate-og.mjs');
    expect(existsSync(scriptPath)).toBe(true);
    expect(statSync(scriptPath).size).toBeGreaterThan(0);
  });
});

describe('BaseLayout OG meta tags', () => {
  const layout = read('src/layouts/BaseLayout.astro');

  it('declares og:image with width/height matching the 1200x630 asset', () => {
    expect(layout).toMatch(/property="og:image"/);
    expect(layout).toMatch(/property="og:image:width"\s+content="1200"/);
    expect(layout).toMatch(/property="og:image:height"\s+content="630"/);
    expect(layout).toMatch(/property="og:image:type"\s+content="image\/png"/);
  });

  it('declares og:image:alt for screen readers and crawlers', () => {
    expect(layout).toMatch(/property="og:image:alt"/);
  });

  it('mirrors the og:image as twitter:image so Twitter renders the same card', () => {
    expect(layout).toMatch(/name="twitter:image"/);
    expect(layout).toMatch(/name="twitter:card"\s+content="summary_large_image"/);
  });

  it('takes a Props.ogImage override that defaults to /og/default.png', () => {
    expect(layout).toMatch(/ogImage\?:\s*string/);
    expect(layout).toMatch(/ogImage\s*=\s*["']\/og\/default\.png["']/);
  });

  it('resolves ogImage against the site origin so og:image is absolute', () => {
    // OG crawlers (Facebook, LinkedIn) require absolute URLs.
    expect(layout).toMatch(/new URL\(ogImage,\s*siteOrigin\)/);
  });

  it('declares og:site_name so previews label the source', () => {
    expect(layout).toMatch(/property="og:site_name"/);
  });
});

describe('per-page OG image overrides', () => {
  it('pricing.astro passes the pricing OG card', () => {
    const src = read('src/pages/pricing.astro');
    expect(src).toMatch(/ogImage=["']\/og\/pricing\.png["']/);
  });

  it('agents.astro passes the agents OG card', () => {
    const src = read('src/pages/agents.astro');
    expect(src).toMatch(/ogImage=["']\/og\/agents\.png["']/);
  });

  it('the home page does not need to override (uses /og/default.png)', () => {
    const src = read('src/pages/index.astro');
    // Should not pass ogImage explicitly — it inherits the default.
    expect(src).not.toMatch(/ogImage=/);
  });
});
