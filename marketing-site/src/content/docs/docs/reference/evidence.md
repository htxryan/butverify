---
title: bv evidence
description: Render screenshot/video proof locally or publish it as a private gallery on butverify.dev
---

`bv evidence` is the CLI subcommand that turns a small `evidence.json`
manifest plus local screenshot/video files into a static gallery. Agents run
it after finishing a piece of work to surface clickable proof — captioned,
ordered, and either served locally or published as an identity-gated remote
microsite — instead of dumping a wall of screenshots into chat.

```bash
bv evidence --from evidence.json --push                 # configured mode
bv evidence --from evidence.json --push --mode remote   # private remote URL
```

In remote mode, `bv` prints JSON with the published URL and its expiry:

```json
{ "url": "https://<site>.butverify.dev", "expires_at": "2026-05-04T17:31:02Z" }
```

The render is fully client-side: the control plane never sees your raw
screenshots. The published bundle is HTML, CSS, and copied assets — no
JavaScript runtime — so the gallery first-paints without network round
trips after the page itself loads.

## Input contract — `evidence.json`

A minimal manifest:

```json
{
  "title": "Login page redesign",
  "subtitle": "Ticket DELIVERY-1234 · 2026-04-27",
  "summary": "Updated the login form to match the new identity. All states pass automated tests; here is the human-visible proof.",
  "items": [
    {
      "src": "./screenshots/01-empty.png",
      "title": "Empty state",
      "description": "Page loads with no validation errors visible.",
      "sequence": 1
    },
    {
      "src": "./screenshots/02-error.png",
      "title": "Inline validation",
      "description": "Empty-email submit shows the helper inline; field gets aria-describedby.",
      "sequence": 2
    },
    {
      "src": "./videos/03-success.webm",
      "title": "Successful sign-in",
      "description": "End-to-end flow from form fill to dashboard redirect (3s clip).",
      "sequence": 3
    }
  ]
}
```

### Top-level fields

| Field      | Type   | Required | Notes                                                                                                        |
| ---------- | ------ | -------- | ------------------------------------------------------------------------------------------------------------ |
| `title`    | string | yes      | Page `<title>` and the H1 above the gallery. ≤200 chars.                                                     |
| `subtitle` | string | no       | Single secondary line below the title. ≤300 chars.                                                           |
| `summary`  | string | no       | Short paragraph above the gallery (e.g. ticket ref, what was delivered). ≤2000 chars; preserves line breaks. |
| `items`    | array  | yes (≥1) | One gallery entry per element.                                                                               |

### Item fields

| Field         | Type    | Required | Notes                                                                                                                                                                  |
| ------------- | ------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `src`         | string  | yes      | Local relative path to the asset. Resolved against the directory of the `--from` file (or CWD when `--from -`). MUST stay inside that directory — lexical and symlink. |
| `title`       | string  | no       | Heading shown above the asset. ≤200 chars.                                                                                                                             |
| `description` | string  | no       | Body text shown beneath the asset. ≤2000 chars.                                                                                                                        |
| `sequence`    | integer | no       | Explicit ordering. Sequenced items sort ascending; un-sequenced items keep JSON-array order and follow. Stable sort.                                                   |
| `alt`         | string  | no       | Alt text for images. Defaults to the item's `title`; never falls back to `description`.                                                                                |

The JSON is parsed in **strict mode**: unknown top-level or item fields
fail at parse time with a path pointing at the offending JSON node. The
canonical schema is bundled into the binary and printable on demand:

```bash
bv evidence --schema
```

That writes a Draft 2020-12 JSON Schema to stdout and exits 0 — useful
for editor integration and for asserting your manifest is valid before
running the renderer.

## Flags

### `--from <path|->` (required unless `--schema`)

Reads the manifest from a JSON file, or from stdin when the value is
`-`. Stdin must be piped — running `bv evidence --from -` from an
interactive TTY exits with a usage error rather than hanging. Stdin
payloads are capped at 4 MiB (the manifest is text — assets stay
out-of-line on disk).

### `--out <dir>`

Render-only mode. Produces a self-contained static bundle at `<dir>`
(`index.html`, `styles.css`, `assets/`) that opens in a browser without
network. The renderer writes to a sibling temp directory and atomically
renames into place on success — your `--out` path is never partially
written and never `rm -rf`'d.

### `--push`

Renders into a CLI-owned temp directory and runs the standard push pipeline.
In local mode, `bv` serves the rendered gallery on `127.0.0.1` until
interrupted. In remote mode, it returns the published private URL:

```json
{ "url": "https://<site>.butverify.dev", "expires_at": "2026-05-04T17:31:02Z" }
```

The temp directory is cleaned up whether the push succeeds or fails.
`--push` and `--out` are mutually exclusive in spirit — pick whichever
end you want.

### `--mode local|remote`

Overrides the configured publish mode for this invocation. Fresh installs
default to `local`; `bv login` switches the default to `remote`.

### `--schema`

Prints the JSON Schema for `evidence.json` to stdout and exits 0. The
example payload in this page validates against it.

### `--layout {stacked|carousel}`

Picks the rendered layout. Default is `stacked` (vertical figure-stack).
`carousel` opts into a CSS-only horizontal snap-scroll variant. Any
other value exits with a usage error listing the supported set.

### `--ttl-seconds N`

Paid-plan TTL override for the published site. Free-plan accounts get
the default platform TTL.

### `--upload-id <id>`

Idempotent retry token for `--push`. If a push is interrupted between
upload and finalize, re-running with the same `--upload-id` reuses the
in-flight upload instead of starting over.

## Containment & containment root

Every asset `src` is resolved inside a single **containment root**:

- `--from <path>` — the directory of the manifest file.
- `--from -` (stdin) — the current working directory.

The renderer enforces this in three steps: it canonicalises the root
with `filepath.EvalSymlinks`, joins each `src` against it, canonicalises
the result, and rejects anything whose relative path starts with `..`
or resolves absolute. Both lexical (`../../etc/passwd`) and
symlink-based traversal fail at parse time before any bytes are copied.

**Recommended**: prefer `--from <path>` for narrow containment. Running
`bv evidence --from -` from a broad CWD (e.g. a repo root) widens the
containment root and gives prompt-injected paths more surface to play
with. `--from <path>` keeps the root scoped to a dedicated evidence
working directory.

## Supported MIME types

The accepted asset types are a closed allowlist:

| Extension       | MIME              |
| --------------- | ----------------- |
| `.png`          | `image/png`       |
| `.jpg`, `.jpeg` | `image/jpeg`      |
| `.webp`         | `image/webp`      |
| `.gif`          | `image/gif`       |
| `.mp4`          | `video/mp4`       |
| `.webm`         | `video/webm`      |
| `.mov`          | `video/quicktime` |

Each asset passes a **two-gate check**: the file extension AND the
result of `http.DetectContentType` over the first 512 bytes must both
be in the allowlist and must agree. Polyglot files and renamed
extensions fail one or both gates and abort the render.

**SVG is excluded by design.** SVG is an executable XML format
(`<script>` tags, event handlers, external `href`); serving an
attacker-supplied SVG verbatim from the published site would create an
XSS surface inside its origin. Agents producing screenshot evidence
should output PNG/JPEG/WebP.

## Layouts

### Stacked (default)

A vertical figure-stack. Each item renders as title, asset, and
description in JSON-resolved order. Best for "here is what changed,
walked top-to-bottom."

### Carousel (`--layout carousel`)

A CSS-only horizontal snap-scroll. Pager links anchor-jump between
items; arrow keys scroll natively. No JavaScript — same JSON contract
as stacked. Both layouts are covered by golden-snapshot tests so
template edits cannot silently produce the wrong output.

## Bundle properties

- **Deterministic.** Re-running the renderer on the same input
  produces byte-identical output. No wall-clock timestamps are
  embedded.
- **No JavaScript.** First paint and gallery navigation work without a
  JS runtime; `<img loading="lazy" decoding="async">` and
  `<video preload="metadata" controls>` are the only "smarts."
- **Small.** A typical input renders to under 50 KB of HTML+CSS;
  copied assets are the bulk of the bundle.

## Caps

| Limit                            | Value                |
| -------------------------------- | -------------------- |
| Per-asset file size              | 1 GiB                |
| Items per evidence site          | 1..500               |
| Stdin manifest size (`--from -`) | 4 MiB                |
| Bundle upload (free tier)        | 100 MB               |
| Bundle upload (paid tier)        | 1 GB                 |
| Free-tier evidence sites         | 30 / tenant / month  |
| Paid-tier evidence sites         | 300 / tenant / month |

## Error envelopes worth knowing

- **HTTP 402** — fairness cap exceeded for the month (free tier 30,
  paid 300 evidence sites/tenant/month). The CLI surfaces a clear
  envelope; no site is created and the temp directory is cleaned.
- **HTTP 413** — bundle exceeds the per-tier upload cap (100 MB free /
  1 GB paid). Trim videos or split into multiple evidence sites.
- **EV-E-8 (HTTP 400, distinctive)** — `evidence template not yet
enabled on this control plane; retry after the server-side rollout
completes`. The control plane gates `template=evidence` behind a
  rollout flag; if you hit this, the CLI is ahead of the server and
  will work once the Worker rolls out. The CLI does **not** silently
  retry without `template`.

See [error codes](/docs/reference/error-codes) for the full list of
non-zero exits from `bv`.

## See also

- [`bv agent-init`](/docs/reference/install-skill) — the
  `/butverify` skill calls `bv evidence` automatically once installed.
- [`bv` CLI reference](/docs/reference/cli) — every subcommand.
- [Error codes](/docs/reference/error-codes) — what each non-zero exit
  from `bv` means.
