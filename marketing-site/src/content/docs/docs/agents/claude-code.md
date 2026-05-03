---
title: Claude Code recipe
description: Push the project's preview directory after every successful build.
---

This recipe wires `bv push` into a Claude Code session so the agent can
hand the human a viewable URL after every meaningful change.

## Prerequisites

- The CLI is installed: `bv --version` works.
- For remote/private URLs, you've run `bv login --token <token>` once (see
  [Authenticate](/docs/quickstart/install/#authenticate-for-remote-publishing)).
- The project produces output to a known directory (e.g. `dist/`,
  `out/`, `public/`, etc.).

## Recipe: PostToolUse hook

Save this as `.claude/hooks/post-tool-use.sh` in your project (or your
home directory for a global hook):

```bash
#!/usr/bin/env bash
set -euo pipefail

# Only run after Bash tool calls that look like a build.
if [[ "${CLAUDE_TOOL_NAME:-}" != "Bash" ]]; then exit 0; fi
if ! grep -qE "^(npm run build|pnpm build|yarn build|astro build|vite build)" \
       <<< "${CLAUDE_TOOL_INPUT:-}"; then
  exit 0
fi

# Pick the dist directory by convention.
for dir in dist out build .next/static; do
  if [[ -d "$dir" ]]; then
    BV_DIR="$dir"
    break
  fi
done
: "${BV_DIR:=}"
if [[ -z "$BV_DIR" ]]; then exit 0; fi

# Push and emit a one-line preview URL Claude will see.
RESULT="$(bv --json push --mode remote "$BV_DIR")"
URL="$(jq -r '.url' <<< "$RESULT")"
echo "[butverify] Preview live: $URL"
```

Then register the hook in `.claude/settings.json`:

```jsonc
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash",
        "hooks": [{ "type": "command", "command": ".claude/hooks/post-tool-use.sh" }],
      },
    ],
  },
}
```

After this, anytime Claude runs a build the URL appears in the transcript.
The human can click it to verify the change visually.

## Recipe: Slash command

A simpler alternative — give the human a `/preview` command that publishes
on demand:

```jsonc
// .claude/commands/preview.json
{
  "name": "preview",
  "description": "Publish dist/ to butverify and print the URL",
  "prompt": "Run `bv --json push --mode remote dist` and report the URL field.",
}
```

## Tips

- Each `bv push` provisions a new `site_id`. To re-publish to the same URL
  across runs, capture the `site_id` from the first push and reuse the
  associated `upload_id` via `bv push --upload-id <id>` for an idempotent
  retry of that specific upload.
- Use `bv pin <site-id>` once you've found a stable preview to keep it
  beyond your tier's retention window.
- For private collaboration, manage per-site access policies via the
  [dashboard](https://app.butverify.dev).
