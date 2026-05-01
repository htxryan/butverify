#!/usr/bin/env node
/**
 * Static link checker for the built marketing+docs site.
 *
 * Walks `dist/` for HTML files, parses their internal links (anchors and
 * <link rel="canonical">), and reports any reference that doesn't resolve
 * to a real file in the build output.
 *
 * Exits non-zero on any broken link so CI fails the deploy.
 */
import { readdir, readFile, stat } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const ROOT = resolve(dirname(__filename), '..');
const DIST = resolve(ROOT, 'dist');

const SKIP_HOSTS = new Set([
  // External hosts we deliberately link to but don't fetch in CI.
  'github.com',
  'app.butverify.dev',
  'api.butverify.dev',
  'status.butverify.dev',
  'us.i.posthog.com',
]);

async function walk(dir) {
  const out = [];
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const p = join(dir, entry.name);
    if (entry.isDirectory()) out.push(...(await walk(p)));
    else if (entry.isFile() && p.endsWith('.html')) out.push(p);
  }
  return out;
}

async function exists(p) {
  try {
    await stat(p);
    return true;
  } catch {
    return false;
  }
}

function extractHrefs(html) {
  const out = new Set();
  const re = /(?:href|action)\s*=\s*(?:"([^"#]+)(?:#[^"]*)?"|'([^'#]+)(?:#[^']*)?')/g;
  let m;
  while ((m = re.exec(html)) !== null) {
    out.add(m[1] ?? m[2]);
  }
  return out;
}

function isInternal(href) {
  if (!href) return false;
  if (href.startsWith('/')) return true;
  if (href.startsWith('./') || href.startsWith('../')) return true;
  return false;
}

function isSkippedExternal(href) {
  if (href.startsWith('mailto:')) return true;
  if (href.startsWith('tel:')) return true;
  if (href.startsWith('javascript:')) return true;
  if (href.startsWith('#')) return true;
  try {
    const u = new URL(href);
    if (SKIP_HOSTS.has(u.host)) return true;
    // Same-origin absolute (canonical etc.)
    if (u.host === 'butverify.dev' || u.host === 'docs.butverify.dev') {
      return false; // checked as internal below
    }
    // Other external — skip (not our problem to verify here)
    return true;
  } catch {
    return false;
  }
}

async function resolveHref(href, fileDir) {
  let pathname;
  try {
    if (/^https?:/.test(href)) {
      const u = new URL(href);
      pathname = u.pathname;
    } else if (href.startsWith('/')) {
      pathname = href;
    } else {
      pathname = '/' + resolve(fileDir, href).slice(DIST.length + 1);
    }
  } catch {
    return false;
  }

  // Strip query string.
  const q = pathname.indexOf('?');
  if (q >= 0) pathname = pathname.slice(0, q);

  // /something/  → /something/index.html
  // /something   → /something.html or /something/index.html
  const candidates = [
    join(DIST, pathname),
    join(DIST, pathname, 'index.html'),
    join(DIST, pathname.replace(/\/$/, '') + '.html'),
    join(DIST, pathname.replace(/\/$/, '') + '/index.html'),
  ];
  for (const c of candidates) {
    if (await exists(c)) return true;
  }
  return false;
}

async function main() {
  if (!(await exists(DIST))) {
    console.error('dist/ not found — run `pnpm build` first');
    process.exit(2);
  }

  const files = await walk(DIST);
  const broken = [];
  for (const file of files) {
    const html = await readFile(file, 'utf8');
    const hrefs = extractHrefs(html);
    for (const href of hrefs) {
      if (isSkippedExternal(href)) continue;
      if (!isInternal(href) && !/^https?:/.test(href)) continue;

      const ok = await resolveHref(href, dirname(file));
      if (!ok) {
        broken.push({ file: file.slice(DIST.length + 1), href });
      }
    }
  }

  if (broken.length === 0) {
    console.log(`✓ link-check: ${files.length} HTML file(s) scanned, no broken internal links`);
    process.exit(0);
  }

  console.error(`✗ link-check: ${broken.length} broken link(s)`);
  for (const b of broken) {
    console.error(`  ${b.file} → ${b.href}`);
  }
  process.exit(1);
}

main().catch((err) => {
  console.error('link-check crashed:', err);
  process.exit(2);
});
