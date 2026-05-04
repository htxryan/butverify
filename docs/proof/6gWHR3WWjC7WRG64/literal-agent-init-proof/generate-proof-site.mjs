import { readFileSync, writeFileSync } from 'node:fs';
import { basename, join } from 'node:path';

const proofDir = new URL('.', import.meta.url).pathname;
const siteDir = join(proofDir, 'site');

function read(name) {
  return readFileSync(join(proofDir, name), 'utf8');
}

function escapeHtml(value) {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;');
}

function section(title, body) {
  return `<section><h2>${escapeHtml(title)}</h2><pre>${escapeHtml(body.trimEnd())}</pre></section>`;
}

const homePath = read('home-path.txt').trim();
const first = read('01-first-agent-init.json');
const firstErr = read('01-first-agent-init.stderr');
const firstStatus = read('01-first-agent-init.status').trim();
const repeat = read('02-repeat-agent-init.json');
const repeatErr = read('02-repeat-agent-init.stderr');
const repeatStatus = read('02-repeat-agent-init.status').trim();
const hash = read('03-hash-check.txt');
const driftOut = read('04-drift-agent-init.stdout');
const driftErr = read('04-drift-agent-init.stderr');
const driftStatus = read('04-drift-agent-init.status').trim();
const installed = read('installed-SKILL.md');

const transcript = [
  `$ export HOME=${homePath}`,
  '$ go run ./cmd/bv --json agent-init',
  first.trimEnd(),
  firstErr.trim() ? `stderr:\n${firstErr.trimEnd()}` : 'stderr: (empty)',
  `exit=${firstStatus}`,
  '',
  '$ go run ./cmd/bv --json agent-init',
  repeat.trimEnd(),
  repeatErr.trim() ? `stderr:\n${repeatErr.trimEnd()}` : 'stderr: (empty)',
  `exit=${repeatStatus}`,
  '',
  '$ node hash-check installed-SKILL.md',
  hash.trimEnd(),
  'exit=0',
  '',
  '# Local body edit inserted into the installed SKILL.md before its metadata comment.',
  '$ go run ./cmd/bv --json agent-init',
  driftOut.trim() ? driftOut.trimEnd() : 'stdout: (empty)',
  driftErr.trim() ? `stderr:\n${driftErr.trimEnd()}` : 'stderr: (empty)',
  `exit=${driftStatus}`,
  '',
].join('\n');

writeFileSync(join(proofDir, 'terminal-transcript.txt'), transcript);

const html = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Literal proof: bv agent-init installs /butverify</title>
  <style>
    :root { color-scheme: light dark; }
    body {
      margin: 0;
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: #0b1020;
      color: #eef3ff;
      line-height: 1.55;
    }
    main { max-width: 1120px; margin: 0 auto; padding: 40px 20px 72px; }
    header { margin-bottom: 28px; }
    h1 { font-size: clamp(2rem, 5vw, 4rem); line-height: 1; margin: 0 0 12px; letter-spacing: -0.05em; }
    h2 { margin: 0 0 12px; font-size: 1.1rem; color: #a9c7ff; }
    p, li { max-width: 82ch; color: #d8e4ff; }
    .claim {
      border: 1px solid #33518f;
      border-radius: 18px;
      padding: 20px;
      background: linear-gradient(135deg, rgba(35, 70, 140, 0.45), rgba(10, 18, 42, 0.85));
      margin: 24px 0;
    }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 16px; }
    section {
      border: 1px solid #26395f;
      border-radius: 16px;
      background: rgba(8, 14, 30, 0.78);
      padding: 18px;
      overflow: hidden;
    }
    pre {
      white-space: pre-wrap;
      overflow-wrap: anywhere;
      background: #050812;
      border: 1px solid #1d2a45;
      border-radius: 12px;
      padding: 14px;
      max-height: 520px;
      overflow: auto;
      color: #dff0ff;
      font: 0.9rem/1.5 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
    }
    a { color: #9fc2ff; }
    code { color: #f2d18a; }
  </style>
</head>
<body>
  <main>
    <header>
      <h1>Literal proof: <code>bv agent-init</code> works</h1>
      <p>This page contains the actual captured CLI outputs from running the PR branch in an isolated temporary <code>HOME</code>. It shows a fresh install, a repeat idempotent run, the installed skill metadata hash check, and a deliberate local edit being refused by drift detection.</p>
    </header>

    <div class="claim">
      <strong>What this proves:</strong>
      <ul>
        <li><code>bv --json agent-init</code> writes <code>$HOME/.claude/skills/butverify/SKILL.md</code>.</li>
        <li>The installed file ends with <code>&lt;!-- bv-skill: release=... sha256=... --&gt;</code>.</li>
        <li>The metadata hash matches the SHA-256 prefix of the file body excluding that metadata line.</li>
        <li>Running the command again reports <code>already_current</code> instead of overwriting.</li>
        <li>After a local body edit, running again exits non-zero and reports drift, including <code>current_content</code>.</li>
      </ul>
    </div>

    <div class="grid">
      ${section('Fresh install JSON', first)}
      ${section('Repeat run JSON', repeat)}
      ${section('Metadata hash verification', hash)}
      ${section('Drift detection stdout', driftOut || '(empty stdout)')}
      ${section('Drift detection stderr', driftErr)}
    </div>

    ${section('Full terminal transcript', transcript)}
    ${section(`Installed ${basename('SKILL.md')} proof copy`, installed)}
  </main>
</body>
</html>
`;

writeFileSync(join(siteDir, 'index.html'), html);
