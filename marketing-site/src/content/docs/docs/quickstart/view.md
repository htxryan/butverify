---
title: View it in a browser
description: How local preview URLs and private remote URLs work.
---

`bv push` can print either a local preview URL or a private remote URL,
depending on the selected mode.

## Local previews

Fresh installs default to local mode. When `bv push` prints a URL like
`http://127.0.0.1:53162/`, opening that URL in a browser reads from the
filtered bundle served by the running `bv` process.

Local preview URLs are same-machine only and last only while the command is
running. Stop the process and the URL goes away; run `bv push` again to get a
new preview.

## Remote private URLs

After `bv login`, or when you pass `--mode remote`, `bv push` prints a URL like
`https://kind-otter-7q.butverify.dev`. Opening that URL in a browser:

1. Hits a Cloudflare Worker on `*.butverify.dev`.
2. Cloudflare Access intercepts the request and re-runs GitHub OAuth if
   your SSO has expired.
3. The Worker checks whether your verified GitHub login is on the site's
   access list and either streams the asset from R2 or returns 403.

Remote sites are private by default. The site owner can always view; access
for other GitHub logins is managed via the dashboard.

## Manage access

Access policies (granting other GitHub users or organizations) apply to remote
sites and are managed from the [butverify dashboard](https://app.butverify.dev)
per-site. The dashboard is the single source of truth for who can view a
remote site.

Custom domains are a v2 feature — v1 remote sites live on
`<site-id>.butverify.dev` only.

## Next

- [JSON output for agent loops →](/docs/quickstart/json-output)
