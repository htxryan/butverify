---
title: JSON output for agents
description: Make every CLI command machine-readable for agent loops.
---

Every `bv` subcommand accepts `--json`. With it, stdout is a single JSON
document and stderr stays human-readable for debugging.

## Push

```bash
bv push --json .
```

```json
{
  "site_id": "kind-otter-7q",
  "url": "https://kind-otter-7q.butverify.dev",
  "status": "live",
  "manifest_sha": "sha256:…",
  "expires_at": "2026-05-27T00:00:00Z",
  "idempotent": false,
  "file_count": 142,
  "total_bytes": 3568912
}
```

On failure, stdout emits an error envelope:

```json
{
  "error": {
    "code": "quota_exceeded",
    "message": "site count limit reached for free tenant",
    "limit": 5,
    "current": 5,
    "upgrade_url": "https://app.butverify.dev/billing"
  }
}
```

The exit code is non-zero on any error so agent harnesses can branch on
`$?` without parsing JSON.

## List sites

```bash
bv ls --json
```

```json
{
  "sites": [
    {
      "site_id": "kind-otter-7q",
      "tenant_id": "t_abc",
      "status": "live",
      "url": "https://kind-otter-7q.butverify.dev",
      "expires_at": "2026-05-27T00:00:00Z",
      "pinned_at": "",
      "bytes_used": 3568912
    }
  ]
}
```

## Get / cat / manifest

`bv get`, `bv cat`, and `bv manifest` all support `--json` and return the
asset metadata you'd otherwise see in the human-readable output.

## Common error codes

| code                | meaning                 | suggested agent action                                             |
| ------------------- | ----------------------- | ------------------------------------------------------------------ |
| `auth_required`     | No / expired token      | Surface to human; re-run `bv init` with a fresh installation token |
| `quota_exceeded`    | Hit a tier limit        | Surface `upgrade_url` to human                                     |
| `payload_too_large` | Tarball >1 GB           | Trim assets; surface to human                                      |
| `site_not_found`    | Wrong site_id           | Check `bv ls --json`                                               |
| `rate_limited`      | Too many requests       | Sleep `retry_after` seconds                                        |
| `network`           | Transport-level failure | Retry up to 3× with backoff                                        |
| `internal`          | Server-side failure     | Retry once; surface to human                                       |

The full list lives in the [error code reference](/docs/reference/error-codes).
