---
title: Cursor recipe
description: Wire bv push into a Cursor command and surface URLs in the transcript.
---

Cursor doesn't have an agent-side hook system, but its **Composer** can
shell out and read JSON. This recipe wires a project-level command that
publishes the current build and prints the URL.

## Prerequisites

- The CLI is installed: `bv --version` works.
- For remote/private URLs, you've run `bv login --token <token>` once (see
  [Authenticate](/docs/quickstart/install/#authenticate-for-remote-publishing)).
- Your project has a build script that emits to a known directory.

## Recipe: Cursor rule + script

Drop this script in your repo as `scripts/preview.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

DIR="${1:-dist}"

if [[ ! -d "$DIR" ]]; then
  echo "{\"error\":{\"code\":\"invalid_argument\",\"message\":\"directory $DIR does not exist; run your build first\"}}" >&2
  exit 1
fi

bv --json push --mode remote "$DIR"
```

Make it executable:

```bash
chmod +x scripts/preview.sh
```

Then add a Cursor rule (`.cursor/rules/preview.mdc`):

```markdown
---
description: Publish the current build to butverify
---

Whenever the user types "preview" or "publish" or "deploy preview", run
`scripts/preview.sh`. Read the JSON result and tell the user the value of
the `url` field, formatted as a markdown link. If the response includes an
`error` object, surface its `code` and any `upgrade_url`.
```

## Recipe: Cursor Composer task template

If you'd prefer not to add a rule, ask Composer:

> Run `./scripts/preview.sh dist` and reply with the URL from the JSON
> output as a markdown link. If the command fails, show me the error code.

Composer reads the JSON output the same way an agent harness would.

## Tips

- Each `bv push` mints a fresh `site_id`. Capture it from the JSON
  response and pin it (`bv pin <site-id>`) if you want it to outlive
  the retention window.
- The CLI exits non-zero on quota / auth errors so Cursor's "command
  failed" surfacing kicks in correctly.
