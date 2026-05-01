---
title: Troubleshooting
description: Common failure modes and how to recover.
---

If you hit something that isn't here, email
[support@butverify.dev](mailto:support@butverify.dev) with the request ID
the CLI printed (the API echoes a `request_id` in error envelopes).

## `bv init` fails with `auth_required` or 401

**Cause**: the installation token you supplied is invalid, expired, or
belongs to a GitHub App installation that no longer exists.

**Fix**: open [app.butverify.dev](https://app.butverify.dev), copy a fresh
installation token from the dashboard, and re-run `bv init --token <token>`.

## `bv push` fails with `auth_required`

**Cause**: the installation token in `~/.config/butverify/config.json`
expired between runs (installation tokens are short-lived).

**Fix**: grab a fresh token from the dashboard and re-run `bv init`.

## `bv push` fails with `quota_exceeded`

**Cause**: you've hit a tier limit (sites, storage, or rate).

**Fix**: the response includes `current`, `limit`, and `upgrade_url`.
Either delete sites (`bv rm <site-id>`) or
[upgrade](https://app.butverify.dev/billing).

## `bv push` returns success but the URL 404s

**Cause**: the edge KV mirror is still propagating (typically <2 seconds).

**Fix**: wait a beat; if it persists past 30 seconds, you've hit a bug —
file an issue with the request ID.

## "TLS handshake failed" / `network` errors in CI

**Cause**: corporate proxies or VPNs that intercept TLS.

**Fix**: set `HTTPS_PROXY` if your environment uses one. The CLI honors
the standard `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY` environment variables
that Go's `net/http` reads.

## `Permission denied` opening a site URL

**Cause**: the GitHub identity Cloudflare Access verified isn't on the
site's access list.

**Fix**: the site owner grants access via the
[dashboard](https://app.butverify.dev). Make sure you're signed into the
right GitHub account in your browser.

## Sites unexpectedly disappear

**Cause**: free tier retention is 30 days from last push.

**Fix**: pin the site (`bv pin <site-id>`) or upgrade. Pinned sites never
expire automatically.

## Stripe redirect loops on `/billing`

**Cause**: stale dashboard cookie after a tenant was changed externally.

**Fix**: sign out via `app.butverify.dev` user menu, sign back in.

## Where to find my request ID

The API includes a `request_id` field in every error envelope. The CLI
prints it whenever it surfaces a server error.

## Status & incidents

Check [status.butverify.dev](https://status.butverify.dev) for current
service status. Subscribe via RSS or email from the status page.
