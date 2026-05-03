---
title: Error codes
description: Every structured error the CLI / API can return, what triggered it, and how to recover.
---

Every error from `bv --json ...` and from the HTTP API is shaped:

```json
{
  "ok": false,
  "error": {
    "code": "string",
    "message": "human-readable message",
    "...": "code-specific fields"
  }
}
```

Codes are stable across versions; messages may be tweaked.

## Auth

### `auth_required`

The CLI has no token, or the token has expired.
**Recover**: grab a fresh installation token from
[app.butverify.dev](https://app.butverify.dev) and re-run `bv login`.

### `auth_refused`

The token is valid but the principal is not allowed to perform the
action — typically a billing-portal request from a non-owner.
**Recover**: surface to a human; the resource is access-controlled.

### `account_suspended`

The tenant is suspended for non-payment or admin action.
**Recover**: surface `upgrade_url` from the response.

## Quotas

### `quota_exceeded`

Tier limit hit. Body includes `limit`, `current`, and `upgrade_url`.
**Recover**: surface to the human, or delete sites with `bv rm`.

### `payload_too_large`

The tarball exceeds the 1 GB hard cap.
**Recover**: trim the directory; we don't paginate uploads in v1.

## Inputs

### `invalid_argument`

A flag value or path failed validation. The `field` key in the body says
which.
**Recover**: re-issue the request with corrected input.

### `site_not_found`

The `site_id` doesn't resolve to a site in this tenant.
**Recover**: `bv --json ls` to see the real `site_id`s.

## Transport

### `rate_limited`

You're hitting the API too fast. Body includes `retry_after` (seconds).
**Recover**: sleep `retry_after`, retry.

### `network`

TLS / DNS / connection-level failure.
**Recover**: retry up to 3 times with exponential backoff.

### `internal`

Server-side bug. Includes a `request_id` that you can include in support
emails.
**Recover**: retry once; if it persists, surface to the human with the
`request_id`.

## Webhook-only

These appear in audit logs / Stripe webhook deliveries and aren't
returned to the CLI.

### `webhook_signature_mismatch`

The Stripe webhook signature failed verification. The event was
discarded.

### `webhook_replay`

Duplicate `event.id` within the 7-day dedup window.

## Migration & lifecycle

### `tenant_grace`

Action attempted on a tenant in `payment_failed` grace period — read-only
operations succeed, writes fail.
**Recover**: pay the open invoice via the dashboard.

### `site_expired`

Read of a non-pinned site whose retention window has elapsed.
**Recover**: re-push, or upgrade and pin.
