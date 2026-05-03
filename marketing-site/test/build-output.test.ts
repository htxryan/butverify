/**
 * Verifies that the marketing site has the right docs structure.
 * Run after `pnpm build` to also assert the generated HTML — but the
 * build is heavy, so this test stays at the source layout level.
 */
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, '..');

function exists(rel: string): boolean {
  return existsSync(join(ROOT, rel));
}

function collectDocsContentFiles(dir: string, baseDir = dir): string[] {
  const files: string[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...collectDocsContentFiles(path, baseDir));
    } else if (/\.mdx?$/.test(entry.name)) {
      files.push(relative(baseDir, path).replace(/\\/g, '/'));
    }
  }
  return files;
}

describe('source layout', () => {
  it('has the four marketing pages', () => {
    expect(exists('src/pages/index.astro')).toBe(true);
    expect(exists('src/pages/pricing.astro')).toBe(true);
    expect(exists('src/pages/agents.astro')).toBe(true);
    expect(exists('src/pages/changelog.astro')).toBe(true);
  });

  it('ships a 404 page for SPA-style not-found UX', () => {
    expect(exists('src/pages/404.astro')).toBe(true);
  });

  it('declares the docs content collection (Starlight schema)', () => {
    expect(exists('src/content.config.ts')).toBe(true);
  });

  it('does not define duplicate Starlight docs content ids', () => {
    const docsRoot = join(ROOT, 'src/content/docs');
    const ids = new Map<string, string[]>();

    for (const file of collectDocsContentFiles(docsRoot)) {
      const id = file.replace(/\.mdx?$/, '');
      ids.set(id, [...(ids.get(id) ?? []), file]);
    }

    const duplicates = [...ids.entries()]
      .filter(([, files]) => files.length > 1)
      .map(([id, files]) => ({ id, files }));

    expect(duplicates).toEqual([]);
  });

  it('includes the four required quickstart pages', () => {
    for (const slug of ['install', 'first-push', 'view', 'json-output']) {
      expect(exists(`src/content/docs/docs/quickstart/${slug}.md`)).toBe(true);
    }
  });

  it('includes a recipe for each of the three named agents', () => {
    for (const slug of ['claude-code', 'cursor', 'codex']) {
      expect(exists(`src/content/docs/docs/agents/${slug}.md`)).toBe(true);
    }
  });

  it('includes the reference docs the epic calls out', () => {
    expect(exists('src/content/docs/docs/reference/cli.md')).toBe(true);
    expect(exists('src/content/docs/docs/reference/error-codes.md')).toBe(true);
    expect(exists('src/content/docs/docs/reference/site-lifecycle.md')).toBe(true);
  });

  it('includes a troubleshooting page', () => {
    expect(exists('src/content/docs/docs/troubleshooting.md')).toBe(true);
  });

  it('ships robots.txt and a favicon (SEO/accessibility hygiene)', () => {
    expect(exists('public/robots.txt')).toBe(true);
    expect(exists('public/favicon.svg')).toBe(true);
  });

  it('ships an OG default card so social-share previews show a hero, not a favicon', () => {
    expect(exists('public/og/default.png')).toBe(true);
  });

  it('emits Cloudflare Pages _headers with HSTS, nosniff, and CSP', () => {
    const headers = readFileSync(join(ROOT, 'public/_headers'), 'utf8');
    expect(headers).toMatch(/Strict-Transport-Security/);
    expect(headers).toMatch(/X-Content-Type-Options:\s*nosniff/);
    expect(headers).toMatch(/X-Frame-Options:\s*DENY/);
    expect(headers).toMatch(/Content-Security-Policy:/);
    // PostHog is the only third-party origin we whitelist for scripts.
    expect(headers).toMatch(/script-src[^;]*https:\/\/us\.i\.posthog\.com/);
  });
});

describe('404 page', () => {
  const notFoundSrc = readFileSync(join(ROOT, 'src/pages/404.astro'), 'utf8');

  it('ships a non-default illustration (terminal mock with bv view)', () => {
    // The default Astro 404 has no figure / illustration. Ours echoes
    // HeroTerminal's window/chrome motif but shows the failure path.
    expect(notFoundSrc).toMatch(/role="img"/);
    expect(notFoundSrc).toMatch(/bv view/);
    expect(notFoundSrc).toMatch(/site_not_found/);
  });

  it('uses brand-voice copy that reads as ours, not generic', () => {
    expect(notFoundSrc).toMatch(/snapshot/i);
    // The 30-day expiry reference grounds the page in real product behavior.
    expect(notFoundSrc).toMatch(/30 days/);
  });

  it('keeps the existing Back-home and Quickstart CTAs', () => {
    expect(notFoundSrc).toMatch(/href="\/"/);
    expect(notFoundSrc).toMatch(/href="\/docs\/quickstart\/install"/);
  });

  it('marks the decorative window chrome aria-hidden so SR users hear the figure label', () => {
    expect(notFoundSrc).toMatch(/aria-label="Terminal/);
    expect(notFoundSrc).toMatch(/class="chrome"\s+aria-hidden="true"/);
  });

  it('patches the missed-URL into the terminal mock client-side, since SSG bakes "/404" into the static HTML', () => {
    // Static fallback text — must not include {Astro.url.pathname}, which
    // resolves to "/404" at build time and would mislead users. (We only
    // want to flag the JSX-style interpolation, not commentary about it.)
    expect(notFoundSrc).not.toMatch(/\{Astro\.url\.pathname\}/);
    expect(notFoundSrc).toMatch(/data-not-found-path/);
    expect(notFoundSrc).toMatch(/window\.location\.pathname/);
    // Length cap so a pathological URL can't blow out the layout.
    expect(notFoundSrc).toMatch(/p\.slice\(0,\s*45\)/);
  });
});

describe('pricing page', () => {
  const pricingSrc = readFileSync(join(ROOT, 'src/pages/pricing.astro'), 'utf8');

  it("anchors the Pro tier with a 'Most popular' pill", () => {
    expect(pricingSrc).toMatch(/Most popular/);
  });

  it('marks the decorative pill aria-hidden', () => {
    // The text label is exposed to screen readers via the article's
    // aria-label, so the visual ribbon itself must be aria-hidden.
    expect(pricingSrc).toMatch(/aria-hidden="true"[\s\S]*?Most popular/);
  });

  it('reveals "most popular" to screen readers via the Pro article aria-label', () => {
    expect(pricingSrc).toMatch(/most popular/);
  });
});

describe('docs reference: error codes', () => {
  it('at least covers the codes the JSON output quickstart names', () => {
    const errorDoc = readFileSync(
      join(ROOT, 'src/content/docs/docs/reference/error-codes.md'),
      'utf8',
    );
    for (const code of [
      'auth_required',
      'quota_exceeded',
      'payload_too_large',
      'site_not_found',
      'rate_limited',
      'network',
      'internal',
    ]) {
      expect(errorDoc).toContain(code);
    }
  });
});

describe('typography continuity', () => {
  // Without these overrides Starlight falls back to its default
  // ui-sans-serif/system-ui stack and the typeface visibly changes when
  // crossing from marketing into /docs.
  const starlightCss = readFileSync(join(ROOT, 'src/styles/starlight.css'), 'utf8');

  it("overrides Starlight's --sl-font with the marketing Inter stack", () => {
    expect(starlightCss).toMatch(/--sl-font:\s*[^;]*['"]Inter['"]/);
    expect(starlightCss).toMatch(/--sl-font:\s*[^;]*ui-sans-serif/);
  });

  it("overrides Starlight's --sl-font-mono with the marketing mono stack", () => {
    expect(starlightCss).toMatch(/--sl-font-mono:\s*ui-monospace/);
    expect(starlightCss).toMatch(/--sl-font-mono:[^;]*Menlo/);
  });
});

describe('docs default to dark theme', () => {
  // Without these overrides Starlight's ThemeProvider falls back to
  // `prefers-color-scheme`, which resolves to 'light' in headless
  // Chromium and on macOS users who never opted in to dark mode —
  // a brand-jarring flash when crossing from butverify.dev → /docs.
  it('wires the ThemeProvider override in astro.config.mjs', () => {
    const cfg = readFileSync(join(ROOT, 'astro.config.mjs'), 'utf8');
    expect(cfg).toMatch(
      /ThemeProvider:\s*['"]\.\/src\/components\/starlight\/ThemeProvider\.astro['"]/,
    );
    expect(cfg).toMatch(
      /ThemeSelect:\s*['"]\.\/src\/components\/starlight\/ThemeSelect\.astro['"]/,
    );
  });

  it('ThemeProvider override defaults to dark when nothing is stored', () => {
    const provider = readFileSync(
      join(ROOT, 'src/components/starlight/ThemeProvider.astro'),
      'utf8',
    );
    // Stored 'auto' must still resolve via prefers-color-scheme so
    // the explicit "auto" picker option keeps following the system.
    expect(provider).toMatch(/storedTheme === 'auto'/);
    // Unset / empty / unrecognized values must fall through to dark.
    expect(provider).toMatch(/theme = 'dark'/);
  });

  it('ThemeSelect override treats unset localStorage as dark', () => {
    const select = readFileSync(join(ROOT, 'src/components/starlight/ThemeSelect.astro'), 'utf8');
    // The original returned 'auto' for null/empty; the override
    // must short-circuit to 'dark' before the parseTheme fallback.
    expect(select).toMatch(/raw === null \|\| raw === ''/);
    expect(select).toMatch(/return 'dark'/);
    // 'auto' must still persist verbatim so the picker round-trips.
    expect(select).toMatch(/localStorage\.setItem\(storageKey, theme\)/);
  });
});

describe('agent recipes mention the CLI commands they rely on', () => {
  const recipes = readdirSync(join(ROOT, 'src/content/docs/docs/agents'));
  for (const file of recipes) {
    it(`${file} references bv --json push`, () => {
      const body = readFileSync(join(ROOT, 'src/content/docs/docs/agents', file), 'utf8');
      expect(body).toMatch(/bv --json push/);
    });
  }
});
