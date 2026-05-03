---
title: Site lifecycle
description: How remote butverify.dev sites are created, expire, and get reaped.
---

This page describes **remote** sites: artifacts published with `--mode remote`
or with a configured remote default after `bv login`. Local previews do not
enter this state machine. In local mode, `bv` serves the filtered bundle on
`127.0.0.1` until the process is interrupted; no control-plane `site_id`, R2
object, KV mapping, retention window, or janitor sweep is created.

## Remote states

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

Pinned remote sites skip the `expired` transition and stay `live` until you
explicitly delete them.

## Retention windows

| tier | retention                               |
| ---- | --------------------------------------- |
| free | 30 days from last remote push, per site |
| pro  | indefinite (until `bv rm`)              |
| team | indefinite                              |

Snapshot retention (older versions of the same remote site):

| tier | snapshot retention                  |
| ---- | ----------------------------------- |
| free | last snapshot only                  |
| pro  | last 5 snapshots; older ones reaped |
| team | last 5 snapshots; older ones reaped |

## What "live" means

A remote site is `live` when the upload tarball has been expanded into R2
_and_ the host→site map has been published to the edge KV. Both happen
synchronously inside the `POST /v1/sites/{id}/finalize` call that remote
`bv push` issues — when the CLI returns with `status: live`, the URL is
ready. Edge KV propagation typically completes within ~2 seconds; if a
freshly-pushed remote URL 404s, wait a beat before failing the run.

Local mode reports `status: local` and a `127.0.0.1` URL instead. That status
means the CLI is serving the staged bundle from the current process, not that a
remote site has entered the lifecycle above.

## Janitor sweeps

The janitor Worker runs every five minutes. It:

1. Reaps `expired` and `deleted` remote sites' R2 assets.
2. Bumps `tenant.status` based on Stripe payment state.
3. Reconciles the host→site KV against D1 (drift detector).

Read the [E5 janitor doc](https://github.com/butverify/butverify/blob/main/docs/2026-04-27-e5-janitor.md)
for the gory operational details.
