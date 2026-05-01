/**
 * Source-level checks for the social-proof / trust strip.
 *
 * Acceptance criteria from the epic:
 *   - One section between agent recipes and the closing CTA does this work
 *   - All logos optimized inline SVG, all factually accurate
 *   - No fake testimonials or invented logos
 */
import { describe, expect, it } from 'vitest';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, '..');

function read(rel: string): string {
  return readFileSync(join(ROOT, rel), 'utf8');
}

describe('TrustStrip component', () => {
  const src = read('src/components/TrustStrip.astro');

  it('exists at src/components/TrustStrip.astro', () => {
    expect(existsSync(join(ROOT, 'src/components/TrustStrip.astro'))).toBe(true);
  });

  it("uses an honest 'works with' framing — no testimonials, no logos for products that can't run a CLI", () => {
    expect(src).toMatch(/Works with the agents you already use/i);
    // Guard against inventing customer quotes if someone edits this later.
    expect(src).not.toMatch(/testimonial/i);
    expect(src).not.toMatch(/customers? say/i);
  });

  it('renders a list of compatible agent logos via the AgentLogo dispatcher', () => {
    expect(src).toContain('AgentLogo');
    expect(src).toMatch(/data-trust-logo-grid/);
  });

  it('lands recipe-bearing agents at /docs/agents and exposes posthog cta hooks', () => {
    expect(src).toMatch(/recipeHref/);
    // Astro template syntax wraps the dynamic attribute in `={`...`}`.
    expect(src).toMatch(/data-cta=\{`trust-/);
  });

  it('reinforces the JSON contract for agents that lack a dedicated recipe', () => {
    expect(src).toMatch(/shell out and parse JSON/i);
    expect(src).toMatch(/\/docs\/quickstart\/json-output/);
  });

  it('renders a security/compliance row that links each claim to /security', () => {
    expect(src).toMatch(/data-trust-claims/);
    expect(src).toMatch(/TRUST_CLAIMS/);
  });

  it('participates in the scroll-reveal animation (data-bv-reveal)', () => {
    expect(src).toMatch(/data-bv-reveal/);
  });

  it('exposes an accessible labelled landmark via aria-labelledby', () => {
    expect(src).toMatch(/aria-labelledby="trust-strip-heading"/);
    expect(src).toMatch(/id="trust-strip-heading"/);
  });

  it('keeps the agent grid as a list (semantic) with role=list', () => {
    // Tailwind utility classes (`grid`) on a <ul> can strip implicit list
    // semantics in some browsers. role="list" pins them on. We assert each
    // attribute independently so the test is robust to attribute reordering.
    const logoGridUl = src.match(/<ul[^>]*data-trust-logo-grid[^>]*>/);
    expect(logoGridUl?.[0]).toMatch(/role="list"/);
    const claimsUl = src.match(/<ul[^>]*data-trust-claims[^>]*>/);
    expect(claimsUl?.[0]).toMatch(/role="list"/);
  });

  it('gives every trust-claim card a visible focus-visible ring (keyboard a11y)', () => {
    // bv-card alone has no focus ring; without this, keyboard users tabbing
    // through the security row would have no indicator.
    expect(src).toMatch(/focus-visible:ring-2/);
    expect(src).toMatch(/focus-visible:ring-bv-accent-strong/);
  });

  it('honors prefers-reduced-motion for the logo grayscale-to-color transition', () => {
    // The site established a motion-budget pattern — every
    // transition we add should opt out under reduce-motion. (Captured as
    // commit 7e6ce57 / a lessons entry.)
    expect(src).toMatch(/@media\s*\(prefers-reduced-motion:\s*reduce\)/);
    const reducedBlock = src.split('@media (prefers-reduced-motion: reduce)')[1] ?? '';
    expect(reducedBlock).toMatch(/transition:\s*none/);
  });
});

describe('homepage wires the TrustStrip between agents and closing CTA', () => {
  const src = read('src/pages/index.astro');

  it('imports TrustStrip', () => {
    expect(src).toMatch(/import\s+TrustStrip\s+from\s+["'][^"']*TrustStrip\.astro["']/);
  });

  it('renders <TrustStrip /> exactly once', () => {
    const matches = src.match(/<TrustStrip\b/g) ?? [];
    expect(matches.length).toBe(1);
  });

  it('positions the strip after the agents recipe section and before the closing CTA', () => {
    const trustStripAt = src.indexOf('<TrustStrip');
    const agentsSectionAt = src.indexOf('Drop into the agent you already use');
    const closingCtaAt = src.indexOf('Stop screenshotting deploy logs');
    expect(agentsSectionAt).toBeGreaterThan(0);
    expect(trustStripAt).toBeGreaterThan(agentsSectionAt);
    expect(closingCtaAt).toBeGreaterThan(trustStripAt);
  });
});

describe('site.ts trust-strip data', () => {
  const src = read('src/lib/site.ts');

  it('exports COMPATIBLE_AGENTS with at least 6 entries (epic asks for 6-8)', () => {
    expect(src).toMatch(/export const COMPATIBLE_AGENTS:/);
    // Count `slug:` occurrences inside the COMPATIBLE_AGENTS block.
    const block = src.match(/COMPATIBLE_AGENTS:[\s\S]*?\];/)?.[0] ?? '';
    const slugCount = (block.match(/slug:/g) ?? []).length;
    expect(slugCount).toBeGreaterThanOrEqual(6);
    expect(slugCount).toBeLessThanOrEqual(8);
  });

  it('only flags entries with real recipes via recipeHref — no fake docs', () => {
    const block = src.match(/COMPATIBLE_AGENTS:[\s\S]*?\];/)?.[0] ?? '';
    // The three named agents (claude-code, cursor, codex) have recipes shipped.
    for (const slug of ['claude-code', 'cursor', 'codex']) {
      expect(block).toContain(slug);
    }
    for (const recipe of [
      '/docs/agents/claude-code',
      '/docs/agents/cursor',
      '/docs/agents/codex',
    ]) {
      expect(block).toContain(recipe);
    }
  });

  it('does NOT claim recipes for agents we have not written one for', () => {
    // Adding a recipeHref for copilot/aider/continue without shipping the
    // doc would be a 404 link and a fake claim. Catch that regression here.
    const block = src.match(/COMPATIBLE_AGENTS:[\s\S]*?\];/)?.[0] ?? '';
    for (const slug of ['copilot', 'aider', 'continue']) {
      // Find the entry for this slug and check it has no recipeHref key.
      const entryMatch = block.match(new RegExp(`\\{[^}]*slug:\\s*'${slug}'[^}]*\\}`));
      expect(entryMatch).not.toBeNull();
      expect(entryMatch?.[0]).not.toMatch(/recipeHref/);
    }
  });

  it('exports TRUST_CLAIMS with the three factually-accurate compliance claims', () => {
    expect(src).toMatch(/export const TRUST_CLAIMS:/);
    const block = src.match(/TRUST_CLAIMS:[\s\S]*?\];/)?.[0] ?? '';
    expect(block).toMatch(/GitHub OAuth/);
    expect(block).toMatch(/Cloudflare Access/);
    expect(block).toMatch(/cosign-signed/);
    // Each claim links to a per-item fragment on /security so screen-reader
    // link lists distinguish the three rather than reading three identical
    // "/security" entries.
    const hrefs = block.match(/href:\s*'([^']+)'/g) ?? [];
    expect(hrefs.length).toBeGreaterThanOrEqual(3);
    for (const h of hrefs) {
      expect(h).toMatch(/\/security#[a-z-]+/);
    }
  });
});

describe('/security backs every trust-strip claim it links to', () => {
  // The trust strip routes every claim to /security#<anchor>. If /security
  // doesn't expand the claim or the anchor is missing, the link is broken.
  const security = read('src/pages/security.astro');
  const siteSrc = read('src/lib/site.ts');

  it('expands GitHub OAuth, Cloudflare Access, and cosign-signed releases', () => {
    expect(security).toMatch(/GitHub OAuth/);
    expect(security).toMatch(/Cloudflare Access/);
    expect(security).toMatch(/cosign[- ]signed releases/i);
  });

  it('groups the claims under a What we ship / trust-model heading anchor', () => {
    expect(security).toMatch(/id="trust-model"/);
  });

  it('hosts every fragment anchor referenced by TRUST_CLAIMS hrefs', () => {
    const block = siteSrc.match(/TRUST_CLAIMS:[\s\S]*?\];/)?.[0] ?? '';
    const hrefs = [...block.matchAll(/href:\s*'\/security#([a-z-]+)'/g)].map((m) => m[1]);
    expect(hrefs.length).toBeGreaterThanOrEqual(3);
    for (const anchor of hrefs) {
      expect(security).toMatch(new RegExp(`id="${anchor}"`));
    }
  });
});

describe('agent logos shipped for every COMPATIBLE_AGENTS slug', () => {
  // The AgentLogo dispatcher must dispatch on each slug; a missing branch
  // would silently render nothing in the trust strip.
  const dispatcher = read('src/components/icons/AgentLogo.astro');

  for (const slug of ['claude-code', 'cursor', 'codex', 'copilot', 'aider', 'continue']) {
    it(`AgentLogo dispatches on slug="${slug}"`, () => {
      expect(dispatcher).toContain(slug);
    });
  }

  for (const file of ['CopilotLogo.astro', 'AiderLogo.astro', 'ContinueLogo.astro']) {
    it(`new logo ${file} ships as inline SVG with currentColor + aria-hidden`, () => {
      const path = `src/components/icons/${file}`;
      expect(existsSync(join(ROOT, path))).toBe(true);
      const body = read(path);
      expect(body).toMatch(/<svg\b/);
      expect(body).toContain('currentColor');
      expect(body).toMatch(/aria-hidden="true"/);
      expect(body).not.toMatch(/<img\s/);
    });
  }
});
