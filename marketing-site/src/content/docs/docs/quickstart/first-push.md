---
title: Push your first site
description: Go from a directory of files to a local preview or private URL in under a minute.
---

You need a directory containing an `index.html` (or any other static asset
your viewer will load first). Anything that runs in a static-site generator
works — Vite output, an MkDocs build, a hand-written `index.html`, the
output of an agent run.

## Choose Local Or Remote

Fresh installs default to local mode. Local mode serves the filtered publish
bundle on `127.0.0.1` until you interrupt the command. To publish a private
butverify.dev URL instead, run `bv login` once or pass `--mode remote` after
authenticating. See [Install the CLI → Authenticate](/docs/quickstart/install/#authenticate-for-remote-publishing).

## Push

```bash
bv push .                 # local by default on a fresh install
bv push --mode remote .   # remote private URL after bv login
```

The CLI:

1. Bundles the directory into a tarball (`.git/`, `node_modules/`, and
   dot-files are skipped by default; pass `--include-hidden` to include
   them).
2. In local mode, serves the filtered bundle on `127.0.0.1` until interrupted.
3. In remote mode, asks the API for a presigned R2 upload URL, streams the
   tarball, calls `finalize`, and prints the private URL plus metadata.

If stderr is redirected or captured, `bv` writes the same progress as stable
status lines so logs stay readable:

```text
[1/4] [#####---------------] Provisioned: kind-otter-7q ready for upload
[2/4] [##########----------] Bundled: 142 files (3568912 bytes)
[3/4] [###############-----] Uploaded: bundle staged
[4/4] [####################] Published: https://kind-otter-7q.butverify.dev
Published site
  Open URL:   https://kind-otter-7q.butverify.dev

Metadata
  Site ID:    kind-otter-7q
  Status:     live
  Manifest:   sha256:…
  Files:      142
  Size:       3568912 bytes
  Expires:    2026-05-27T00:00:00Z
```

Remote URLs are private by default — only you and any GitHub users granted
access via the [dashboard](https://app.butverify.dev) can open them. Local
URLs are only available on your machine while the `bv` process is running.

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
