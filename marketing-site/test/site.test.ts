import { describe, expect, it } from 'vitest';
import { AGENTS, CHANGELOG, NAV, PLANS, SITE, resolveInstallUrl } from '../src/lib/site.ts';

describe('SITE constants', () => {
  it('uses butverify.dev as the canonical domain', () => {
    expect(SITE.domain).toBe('butverify.dev');
    expect(SITE.appHost).toBe('app.butverify.dev');
    expect(SITE.apiHost).toBe('api.butverify.dev');
  });

  it('ships a single Brew install command for the CLI', () => {
    expect(SITE.installCmd).toMatch(/^brew install /);
    expect(SITE.installCmd).toContain(SITE.cliInstallTap);
    expect(SITE.installCmd.endsWith(SITE.cliBinary)).toBe(true);
  });
});

describe('NAV', () => {
  it('links every header item to a page that exists in /pages or /docs', () => {
    const validPrefixes = ['/pricing', '/agents', '/changelog', '/docs'];
    for (const item of NAV) {
      const href: string = item.href;
      const ok = href === '/' || validPrefixes.some((p) => href.startsWith(p));
      expect(ok, `nav href ${href} not under a known prefix`).toBe(true);
    }
  });

  it('uses unique labels and unique hrefs (no dupes in the menu)', () => {
    expect(new Set(NAV.map((n) => n.label)).size).toBe(NAV.length);
    expect(new Set(NAV.map((n) => n.href)).size).toBe(NAV.length);
  });
});

describe('PLANS', () => {
  it('defines exactly free / pro / team', () => {
    expect(PLANS.map((p) => p.id)).toEqual(['free', 'pro', 'team']);
  });

  it('highlights exactly one plan (the conversion target)', () => {
    expect(PLANS.filter((p) => p.highlight === true).length).toBe(1);
  });

  it('provides a CTA on every plan card', () => {
    for (const plan of PLANS) {
      expect(plan.ctaLabel.length).toBeGreaterThan(0);
      expect(plan.ctaHref.length).toBeGreaterThan(0);
    }
  });

  it('free + pro CTAs route to the GitHub App install flow', () => {
    const free = PLANS.find((p) => p.id === 'free')!;
    const pro = PLANS.find((p) => p.id === 'pro')!;
    expect(resolveInstallUrl(free.ctaHref)).toMatch(/^https?:\/\//);
    expect(resolveInstallUrl(pro.ctaHref)).toMatch(/^https?:\/\//);
    expect(resolveInstallUrl(free.ctaHref)).toContain('github.com');
    expect(resolveInstallUrl(pro.ctaHref)).toContain('github.com');
  });
});

describe('AGENTS recipes', () => {
  it('ships the three integrations the epic calls out', () => {
    const slugs = AGENTS.map((a) => a.slug).sort();
    expect(slugs).toEqual(['claude-code', 'codex', 'cursor']);
  });

  it('each recipe links to a Starlight slug under /docs/agents/', () => {
    for (const agent of AGENTS) {
      expect(agent.docsHref).toMatch(/^\/docs\/agents\/[a-z-]+$/);
    }
  });
});

describe('CHANGELOG', () => {
  it('uses ISO YYYY-MM-DD dates so the RSS pubDate parses', () => {
    for (const entry of CHANGELOG) {
      expect(entry.date).toMatch(/^\d{4}-\d{2}-\d{2}$/);
      expect(Number.isFinite(new Date(entry.date).getTime())).toBe(true);
    }
  });

  it('has at least one entry (we ship the marketing site itself)', () => {
    expect(CHANGELOG.length).toBeGreaterThan(0);
  });
});

describe('resolveInstallUrl', () => {
  it('substitutes the placeholder with the env URL when present', () => {
    const out = resolveInstallUrl('{{INSTALL_URL}}?ref=test');
    expect(out.includes('{{INSTALL_URL}}')).toBe(false);
    expect(out).toMatch(/^https?:\/\//);
  });

  it('passes through hrefs without the placeholder', () => {
    expect(resolveInstallUrl('mailto:hello@butverify.dev')).toBe('mailto:hello@butverify.dev');
    expect(resolveInstallUrl('/docs/quickstart/install')).toBe('/docs/quickstart/install');
  });

  it('always resolves to a github.com URL even with a placeholder', () => {
    // Defense-in-depth: regardless of build env, the resolved CTA target
    // must be a GitHub install URL — not an attacker-supplied domain.
    const out = resolveInstallUrl('{{INSTALL_URL}}');
    expect(out.startsWith('https://')).toBe(true);
    const u = new URL(out);
    expect(u.host === 'github.com' || u.host.endsWith('.github.com')).toBe(true);
  });
});
