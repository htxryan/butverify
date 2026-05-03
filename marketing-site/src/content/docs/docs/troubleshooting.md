---
title: Troubleshooting
description: Common failure modes and how to recover.
---

If you hit something that isn't here, email
[support@butverify.dev](mailto:support@butverify.dev) with the request ID
the CLI printed. Remote API errors include a `request_id` in their envelopes.

## `bv push` appears to hang after printing a `127.0.0.1` URL

**Cause**: local mode is working. Fresh installs default to local mode, and
`bv push` keeps serving the filtered bundle until you interrupt the process.

**Fix**: open the printed URL in a browser for same-machine review. Press
`Ctrl-C` when you are done, or run `bv push --mode remote <dir>` after
`bv login` when you need a shareable butverify.dev URL.

## A local preview URL stops working

**Cause**: local URLs exist only while the `bv` process is running. They also
work only on the same machine that ran the command.

**Fix**: run `bv push <dir>` again for a new local URL, or use
`bv push --mode remote <dir>` for a private URL other people can open.

## `bv login` fails with `auth_required` or 401

**Cause**: the installation token you supplied is invalid, expired, or
belongs to a GitHub App installation that no longer exists.

**Fix**: open [app.butverify.dev](https://app.butverify.dev), copy a fresh
installation token from the dashboard, and re-run `bv login --token <token>`.

## Remote `bv push` fails with `auth_required`

**Cause**: remote mode needs a valid installation token, and the token in
`~/.config/butverify/config.json` expired between runs. Local mode does not
require auth.

**Fix**: grab a fresh token from the dashboard and re-run `bv login`, or switch
back to local previews with `bv logout` or `bv mode local`.

## Remote `bv push` fails with `quota_exceeded`

**Cause**: you've hit a tier limit (sites, storage, or rate). Local previews do
not consume remote site quota.

**Fix**: the response includes `current`, `limit`, and `upgrade_url`. Either
delete sites (`bv rm <site-id>`) or
[upgrade](https://app.butverify.dev/billing).

## Remote `bv push` returns success but the URL 404s

**Cause**: the edge KV mirror is still propagating (typically <2 seconds).

**Fix**: wait a beat; if it persists past 30 seconds, you've hit a bug —
file an issue with the request ID.

## A dotfile is missing from a local preview

**Cause**: local mode serves the same filtered publish bundle as remote mode.
Dotfiles, `.git/`, and `node_modules/` are skipped by default.

**Fix**: pass `--include-hidden` if the hidden file is intentionally part of
the artifact.

## "TLS handshake failed" / `network` errors in CI

**Cause**: corporate proxies or VPNs that intercept TLS.

**Fix**: set `HTTPS_PROXY` if your environment uses one. The CLI honors
the standard `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY` environment variables
that Go's `net/http` reads.

## `Permission denied` opening a remote site URL

**Cause**: the GitHub identity Cloudflare Access verified isn't on the
site's access list.

**Fix**: the site owner grants access via the
[dashboard](https://app.butverify.dev). Make sure you're signed into the
right GitHub account in your browser.

## Remote sites unexpectedly disappear

**Cause**: free tier retention is 30 days from last remote push.

**Fix**: pin the site (`bv pin <site-id>`) or upgrade. Pinned sites never
expire automatically.

## Stripe redirect loops on `/billing`

**Cause**: stale dashboard cookie after a tenant was changed externally.

**Fix**: sign out via `app.butverify.dev` user menu, sign back in.

## Where to find my request ID

Remote API errors include a `request_id` field in their error envelope. The
CLI prints it whenever it surfaces a server error. Local-mode usage errors may
not have a request ID because they never reached the API.

## Status & incidents

Check [status.butverify.dev](https://status.butverify.dev) for current
service status. Subscribe via RSS or email from the status page.
