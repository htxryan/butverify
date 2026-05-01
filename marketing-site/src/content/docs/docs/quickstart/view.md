---
title: View it in a browser
description: How private viewing works and how to share access with collaborators.
---

When `bv push` prints a URL like
`https://kind-otter-7q.butverify.dev`, opening that URL in a browser:

1. Hits a Cloudflare Worker on `*.butverify.dev`.
2. Cloudflare Access intercepts the request and re-runs GitHub OAuth if
   your SSO has expired.
3. The Worker checks whether your verified GitHub login is on the site's
   access list and either streams the asset from R2 or returns 403.

Sites are private by default. The site owner can always view; access for
other GitHub logins is managed via the dashboard.

## Manage access

Access policies (granting other GitHub users or organizations) are managed
from the [butverify dashboard](https://app.butverify.dev) per-site. The
dashboard is the single source of truth for who can view a site.

Custom domains are a v2 feature — v1 sites live on
`<site-id>.butverify.dev` only.

## Next

- [JSON output for agent loops →](/docs/quickstart/json-output)
