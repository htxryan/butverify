---
title: OpenAI Codex CLI recipe
description: Drop a post-task hook that publishes the workspace and prints JSON.
---

The OpenAI Codex CLI executes tasks inside a sandboxed workspace. This
recipe ships a `~/.codex/post-task.sh` that publishes whatever the agent
left in `./out/` (or another agreed-upon directory) and prints the URL
back to the agent transcript.

## Prerequisites

- The CLI is installed: `bv --version` works.
- For remote/private URLs, you've run `bv login --token <token>` once (see
  [Authenticate](/docs/quickstart/install/#authenticate-for-remote-publishing)).
- Your Codex tasks emit a directory (`out/`, `dist/`, `public/`).

## Recipe: post-task script

```bash
#!/usr/bin/env bash
# ~/.codex/post-task.sh — runs after every Codex task.
set -euo pipefail

WS="${CODEX_WORKSPACE:-.}"

# Try a few conventional output directories, fall back to skipping.
for dir in "$WS/out" "$WS/dist" "$WS/build" "$WS/public"; do
  if [[ -d "$dir" ]]; then
    TARGET="$dir"
    break
  fi
done
: "${TARGET:=}"
if [[ -z "$TARGET" ]]; then
  echo '{"ok":true,"published":false,"reason":"no output directory found"}'
  exit 0
fi

bv --json push --mode remote "$TARGET"
```

Make it executable: `chmod +x ~/.codex/post-task.sh`.

Then point Codex at it via your `~/.codex/config.toml`:

```toml
[hooks]
post_task = "~/.codex/post-task.sh"
```

(Adjust the key name to match the Codex CLI version you have installed —
the hook name has shifted across releases.)

## Recipe: inline tool

If you prefer not to ship a hook script, declare a Codex tool the agent
can invoke directly:

```toml
[[tools]]
name = "publish_preview"
description = "Publish ./out as a private butverify site and return its URL."
command = ["bv", "--json", "push", "--mode", "remote", "out"]
```

The agent can call it explicitly and use the URL from the JSON response.

## Tips

- Each `bv push` provisions a new `site_id`. Pin the resulting site
  (`bv pin <site-id>`) once you've found one you want to keep across
  retention windows.
- The `--json` output includes `site_id`, `url`, and `expires_at` — the
  agent can echo any of those back to the human in subsequent turns.
