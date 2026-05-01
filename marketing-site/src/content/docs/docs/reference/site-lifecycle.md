---
title: Site lifecycle
description: How butverify creates, updates, expires, and reaps sites.
---

Every site moves through a small state machine. Understanding it makes
errors easier to interpret and helps you know when content is safe to
rely on.

## States

| state          | meaning                                                                  |
| -------------- | ------------------------------------------------------------------------ |
| `provisioning` | API accepted the push; tarball not yet expanded into R2                  |
| `live`         | viewable at `https://<site-id>.butverify.dev/`                           |
| `expired`      | retention window elapsed; read returns 410                               |
| `deleted`      | janitor has reaped the assets; the `site_id` remains reserved for 7 days |
| `suspended`    | tenant suspended (non-payment / admin); reads return 503                 |

## Transitions

```
provisioning ──(upload ok)──▶ live
        │
        └─(upload failed)──▶ deleted

live ──(retention elapsed)──▶ expired ──(janitor sweep)──▶ deleted
live ──(bv rm)──▶ deleted
live ──(tenant suspended)──▶ suspended ──(payment ok)──▶ live
```

Pinned sites skip the `expired` transition and stay `live` until you
explicitly delete them.

## Retention windows

| tier | retention                        |
| ---- | -------------------------------- |
| free | 30 days from last push, per site |
| pro  | indefinite (until `bv rm`)       |
| team | indefinite                       |

Snapshot retention (older versions of the same site):

| tier | snapshot retention                  |
| ---- | ----------------------------------- |
| free | last snapshot only                  |
| pro  | last 5 snapshots; older ones reaped |
| team | last 5 snapshots; older ones reaped |

## What "live" means

A site is `live` when the upload tarball has been expanded into R2 _and_
the host→site map has been published to the edge KV. Both happen
synchronously inside the `POST /v1/sites/{id}/finalize` call that
`bv push` issues — when the CLI returns with `status: live`, the URL is
ready. Edge KV propagation typically completes within ~2 seconds; if a
freshly-pushed URL 404s, wait a beat before failing the run.

## Janitor sweeps

The janitor Worker runs every five minutes. It:

1. Reaps `expired` and `deleted` sites' R2 assets.
2. Bumps `tenant.status` based on Stripe payment state.
3. Reconciles the host→site KV against D1 (drift detector).

Read the [E5 janitor doc](https://github.com/butverify/butverify/blob/main/docs/2026-04-27-e5-janitor.md)
for the gory operational details.
