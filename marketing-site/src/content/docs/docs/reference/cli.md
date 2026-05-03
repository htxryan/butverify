---
title: CLI commands
description: Every bv subcommand, its flags, and what it returns.
---

The `bv` CLI is a single Go binary. It supports a handful of global flags
(notably `--json` for machine-readable output) and the subcommands below.

## Global flags

These can be passed before any subcommand:

- `--json` — emit a single JSON document on stdout instead of human-
  readable text. Human progress/status output is suppressed.
- `--api-url=<url>` — override the control-plane endpoint (defaults to
  `https://api.butverify.dev`, or whatever was captured during `bv login`).
- `--token=<token>` — override the bearer token (defaults to the value
  saved by `bv login` in the config file).
- `--help`, `-h` — show usage.
- `--version`, `-v` — print the CLI version.

## `bv login`

Authenticates the CLI and switches the default publish mode to `remote`.
Without `--token`, `bv login` resolves a GitHub user token and exchanges it
with the control plane. With `--token`, it verifies an existing installation
token with `/v1/auth/whoami`. Both paths write
`~/.config/butverify/config.json` (mode `0600`).

Flags:

- `--api-url=<url>` — override the API endpoint.
- `--token=<token>` — supply the token inline (otherwise read from
  `BV_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`, the GitHub CLI, or stdin).

## `bv logout`

Clears saved authentication fields and switches the default publish mode to
`local`.

## `bv mode [local|remote]`

Prints the current default publish mode, or persists `local`/`remote` when a
value is supplied. Fresh installs default to `local`; authenticated legacy
configs and new `bv login` configs default to `remote`.

## `bv push <dir>`

Publishes `<dir>` in the selected mode. In `local` mode, `bv` serves the
filtered publish bundle on `127.0.0.1` until interrupted. In `remote` mode,
it tarballs `<dir>` and uploads it as a new private site. Each remote
invocation provisions a fresh `site_id`; pass `--upload-id` to retry the same
logical upload idempotently.

Flags:

- `--upload-id=<id>` — explicit upload identifier for idempotent retry.
- `--ttl-seconds=<n>` — site TTL in seconds (paid plan; `0` uses the
  server default).
- `--include-hidden` — include dot-files in the bundle.
- `--mode local|remote` — override the configured publish mode for this
  invocation.

## `bv ls`

Lists sites in the authenticated tenant.

## `bv get <site-id> <dest>`

Downloads every file in a site into `<dest>`.

## `bv cat <site-id> <path>`

Streams a single file to stdout.

## `bv rm <site-id>`

Soft-deletes a site (the asynchronous janitor reaps assets within ~30s).

## `bv manifest <site-id>`

Prints the site's `manifest.json`.

## `bv pin <site-id>` / `bv unpin <site-id>`

Pin a site to disable TTL-based expiry, or unpin to re-stamp the default
TTL.

## `bv whoami`

Prints the resolved tenant for the configured token.

## `bv version`

Prints the CLI version.

## `bv report --from <out.json>` / `bv dashboard --from <data.csv>`

Render a templated report or dashboard site from a JSON or CSV input.
Pass `--push` to publish the rendered site in the configured mode, or add
`--mode local|remote` to override it for one invocation.

## Exit codes

| code | meaning                            |
| ---- | ---------------------------------- |
| 0    | success                            |
| 1    | API or runtime error (server-side) |
| 2    | invalid arguments                  |
