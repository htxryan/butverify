---
title: Push your first site
description: Go from a directory of files to a private URL in under a minute.
---

You need a directory containing an `index.html` (or any other static asset
your viewer will load first). Anything that runs in a static-site generator
works — Vite output, an MkDocs build, a hand-written `index.html`, the
output of an agent run.

## Authenticate (one-time)

If you haven't already, run `bv init` to capture an installation token. See
[Install the CLI → Authenticate](/docs/quickstart/install/#authenticate).

## Push

```bash
bv push .
```

The CLI:

1. Bundles the directory into a tarball (`.git/`, `node_modules/`, and
   dot-files are skipped by default; pass `--include-hidden` to include
   them).
2. Asks the API for a presigned R2 upload URL.
3. Streams the tarball.
4. Calls `finalize`, which expands the tarball and publishes the site.
5. Prints the assigned `site_id`, the URL, the manifest SHA, and the
   expiry timestamp.

```text
Provisioned kind-otter-7q (https://kind-otter-7q.butverify.dev)
Bundled 142 files (3568912 bytes)
Uploaded tar to staging
Site:        kind-otter-7q
URL:         https://kind-otter-7q.butverify.dev
Status:      live
Manifest:    sha256:…
Expires:     2026-05-27T00:00:00Z
```

The URL is private by default — only you and any GitHub users granted
access via the [dashboard](https://app.butverify.dev) can open it.

## Each push is a new site

`bv push` mints a fresh `site_id` per invocation. To re-use an existing
`site_id` after a transient error, pass the `--upload-id` from the prior
attempt:

```bash
bv push --upload-id u-1a2b3c4d… .
```

To pin a site so it survives the retention window:

```bash
bv pin <site-id>
```

## Next

- [Open the site in a browser →](/docs/quickstart/view)
- [Wire it into your agent →](/docs/agents/claude-code)
