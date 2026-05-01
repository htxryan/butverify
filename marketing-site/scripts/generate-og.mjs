#!/usr/bin/env node
/**
 * Build-time generator for OG (Open Graph) social cards.
 *
 * Renders 1200x630 PNGs from inline SVG templates so that links to
 * butverify.dev render a 2:1 hero card on Twitter/X, Slack, Discord,
 * and LinkedIn instead of a tiny favicon.
 *
 * Output: public/og/{default,pricing,agents}.png
 *
 * Re-run after copy or palette changes:
 *     pnpm --filter @butverify/marketing-site og:build
 *
 * The generated PNGs are checked into the repo so the deploy doesn't
 * depend on Sharp's native binary being present at build time.
 */
import sharp from 'sharp';
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, '..');
const OUT_DIR = join(ROOT, 'public/og');

const W = 1200;
const H = 630;

const TOKENS = {
  bg0: '#0b0d12',
  bg1: '#11141b',
  surface: '#181c25',
  border: '#232735',
  textDim: '#94a0b3',
  text: '#e6e8ef',
  accent: '#8aa9ff',
  accentStrong: '#5b85ff',
};

const FONT_SANS =
  "Inter, ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif";
const FONT_MONO = 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace';

function escapeXml(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;');
}

/**
 * Wrap a long string to multiple <tspan> rows so titles don't overflow
 * the 1120-pixel safe area. Naive word-wrap; sufficient for static copy.
 */
function wrap(text, maxChars) {
  const words = text.split(/\s+/);
  const lines = [];
  let line = '';
  for (const w of words) {
    if (line.length === 0) {
      line = w;
      continue;
    }
    if (line.length + 1 + w.length > maxChars) {
      lines.push(line);
      line = w;
    } else {
      line += ' ' + w;
    }
  }
  if (line) lines.push(line);
  return lines;
}

function buildSvg({ eyebrow, title, subtitle }) {
  const titleLines = wrap(title, 24);
  const subtitleLines = wrap(subtitle, 56);
  const titleStartY = 330;
  const titleLineH = 84;
  const subtitleStartY = titleStartY + (titleLines.length - 1) * titleLineH + 60;
  const subtitleLineH = 40;

  const titleTspans = titleLines
    .map(
      (line, i) => `<tspan x="80" y="${titleStartY + i * titleLineH}">${escapeXml(line)}</tspan>`,
    )
    .join('');
  const subtitleTspans = subtitleLines
    .map(
      (line, i) =>
        `<tspan x="80" y="${subtitleStartY + i * subtitleLineH}">${escapeXml(line)}</tspan>`,
    )
    .join('');

  return `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%" stop-color="${TOKENS.bg0}"/>
      <stop offset="100%" stop-color="${TOKENS.bg1}"/>
    </linearGradient>
    <pattern id="grid" width="48" height="48" patternUnits="userSpaceOnUse">
      <path d="M 48 0 L 0 0 0 48" fill="none" stroke="${TOKENS.surface}" stroke-width="1"/>
    </pattern>
    <radialGradient id="glow" cx="20%" cy="25%" r="55%">
      <stop offset="0%" stop-color="${TOKENS.accentStrong}" stop-opacity="0.18"/>
      <stop offset="100%" stop-color="${TOKENS.accentStrong}" stop-opacity="0"/>
    </radialGradient>
  </defs>

  <rect width="${W}" height="${H}" fill="url(#bg)"/>
  <rect width="${W}" height="${H}" fill="url(#grid)" opacity="0.55"/>
  <rect width="${W}" height="${H}" fill="url(#glow)"/>

  <rect x="40" y="40" width="${W - 80}" height="${H - 80}" rx="20" fill="none" stroke="${TOKENS.border}" stroke-width="2"/>

  <g font-family="${FONT_MONO}" font-weight="700">
    <text x="80" y="170" font-size="100" fill="${TOKENS.accentStrong}">{ }</text>
  </g>

  <g font-family="${FONT_MONO}">
    <text x="80" y="240" font-size="22" letter-spacing="3" fill="${TOKENS.accent}">${escapeXml(
      eyebrow.toUpperCase(),
    )}</text>
  </g>

  <g font-family="${FONT_SANS}" font-weight="700" fill="${TOKENS.text}">
    <text font-size="72">${titleTspans}</text>
  </g>

  <g font-family="${FONT_SANS}" font-weight="400" fill="${TOKENS.textDim}">
    <text font-size="28">${subtitleTspans}</text>
  </g>

  <g font-family="${FONT_MONO}" font-weight="500">
    <text x="80" y="${H - 70}" font-size="22" fill="${TOKENS.accentStrong}">butverify.dev</text>
    <text x="${W - 80}" y="${H - 70}" font-size="22" fill="${TOKENS.textDim}" text-anchor="end">static hosting / for agents</text>
  </g>
</svg>`;
}

const cards = [
  {
    name: 'default',
    eyebrow: 'butverify',
    title: 'Your agent ships work. You see it instantly.',
    subtitle: 'One CLI command, one private URL — no DNS, no buckets, no IAM.',
  },
  {
    name: 'pricing',
    eyebrow: 'pricing',
    title: 'Free for 5 sites. Pro at $10/mo.',
    subtitle: 'No credit card to start. No surprise overages.',
  },
  {
    name: 'agents',
    eyebrow: 'agent recipes',
    title: 'Drop into the agent you already use.',
    subtitle: 'Claude Code, Cursor, OpenAI Codex — five-minute paste, no SDKs.',
  },
];

async function main() {
  await mkdir(OUT_DIR, { recursive: true });
  for (const card of cards) {
    const svg = buildSvg(card);
    const png = await sharp(Buffer.from(svg)).png().toBuffer();
    const out = join(OUT_DIR, `${card.name}.png`);
    await writeFile(out, png);
    console.log(`✓ ${out.slice(ROOT.length + 1)} (${png.length} bytes)`);
  }
}

main().catch((err) => {
  console.error('og generator crashed:', err);
  process.exit(1);
});
