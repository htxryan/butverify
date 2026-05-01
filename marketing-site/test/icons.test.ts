/**
 * Verifies the feature-card line illustrations
 * and per-agent logos — are present, inline, and importable from the
 * pages that consume them. The tests stay at the source-layout level so
 * they don't require a full Astro build.
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

describe('icons directory layout', () => {
  it('ships the three feature-card icons', () => {
    expect(existsSync(join(ROOT, 'src/components/icons/TerminalIcon.astro'))).toBe(true);
    expect(existsSync(join(ROOT, 'src/components/icons/LockIcon.astro'))).toBe(true);
    expect(existsSync(join(ROOT, 'src/components/icons/HourglassIcon.astro'))).toBe(true);
  });

  it('ships an inline-SVG logo per supported agent', () => {
    expect(existsSync(join(ROOT, 'src/components/icons/ClaudeLogo.astro'))).toBe(true);
    expect(existsSync(join(ROOT, 'src/components/icons/CursorLogo.astro'))).toBe(true);
    expect(existsSync(join(ROOT, 'src/components/icons/CodexLogo.astro'))).toBe(true);
  });

  it('exposes a slug-keyed AgentLogo dispatcher', () => {
    const src = read('src/components/icons/AgentLogo.astro');
    for (const slug of ['claude-code', 'cursor', 'codex']) {
      expect(src).toContain(slug);
    }
  });
});

describe('icons render as inline SVG (no icon-font dep, currentColor)', () => {
  const allIcons = [
    'TerminalIcon.astro',
    'LockIcon.astro',
    'HourglassIcon.astro',
    'ClaudeLogo.astro',
    'CursorLogo.astro',
    'CodexLogo.astro',
  ];

  for (const file of allIcons) {
    it(`${file} is an inline <svg> using currentColor`, () => {
      const body = read(`src/components/icons/${file}`);
      expect(body).toMatch(/<svg\b/);
      expect(body).toContain('currentColor');
      // No raster references — SVG only.
      expect(body).not.toMatch(/<img\s/);
    });
  }

  it('decorative feature icons are aria-hidden', () => {
    for (const file of ['TerminalIcon.astro', 'LockIcon.astro', 'HourglassIcon.astro']) {
      const body = read(`src/components/icons/${file}`);
      expect(body).toMatch(/aria-hidden=/);
    }
  });

  it('agent logos are aria-hidden — the agent name renders as adjacent text', () => {
    // Both index.astro and agents.astro render the logo right next to
    // the agent name (`<AgentLogo /> {agent.name}`). Treating the SVG
    // as a labelled image would make a screen reader announce e.g.
    // "Claude Code logo, Claude Code" — name twice.
    for (const file of ['ClaudeLogo.astro', 'CursorLogo.astro', 'CodexLogo.astro']) {
      const body = read(`src/components/icons/${file}`);
      expect(body).toMatch(/aria-hidden="true"/);
      expect(body).not.toMatch(/role="img"/);
      expect(body).not.toMatch(/aria-label=/);
    }
  });
});

describe('homepage wires the new icons into both card rows', () => {
  const src = read('src/pages/index.astro');

  it('imports each feature-card icon', () => {
    expect(src).toContain('TerminalIcon');
    expect(src).toContain('LockIcon');
    expect(src).toContain('HourglassIcon');
  });

  it('imports the AgentLogo dispatcher for the agent cards', () => {
    expect(src).toContain('AgentLogo');
  });

  it('renders a feature icon next to each feature card heading', () => {
    expect(src).toMatch(/data-feature-icon="terminal"/);
    expect(src).toMatch(/data-feature-icon="lock"/);
    expect(src).toMatch(/data-feature-icon="hourglass"/);
  });
});

describe('agents page renders each agent logo', () => {
  const src = read('src/pages/agents.astro');
  it('imports AgentLogo and renders it inside the recipe card', () => {
    expect(src).toContain('AgentLogo');
    expect(src).toMatch(/<AgentLogo[^>]*slug=/);
  });
});
