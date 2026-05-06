---
name: butverify
description: "[DEPRECATED — use /butverify:prove-it] Publish proof of recent work as a private gallery on butverify.dev"
disable-model-invocation: false
---

# /butverify (deprecated alias for /butverify:prove-it)

> **Deprecated**: This skill is preserved for one release cycle so existing
> users keep working. Prefer `/butverify:prove-it` going forward; that
> skill has the same behavior. The `/butverify:review` skill is also
> available for surfacing human feedback on already-published galleries.

If you have just finished a piece of functional work, you must PROVE IT —
and publish that proof so the human can review it without scrolling through
chat.

## Workflow

1. **Run the app end-to-end.** No mocks, no faking, no forcing the app into
   a particular state. Use it as a human would.
2. **Capture proof as you go.** Use whatever capture tooling you have:
   browser MCP, Playwright, an OS screenshot tool, terminal recording. Aim
   for: empty state, primary success path, at least one edge case, and any
   visual states the change touches.
3. **Inspect each capture as you take it.** Look for functional bugs and
   visual fidelity issues. The feature should be 100% functional and free
   of visual fidelity bugs.
4. **If you find issues:**
   1. List them.
   2. Fix them all.
   3. Resume at step 1.
5. **Write `evidence.json`** with one entry per capture (see the schema
   shipped with `bv evidence --schema`). Set a `title` and `description`
   for each entry; use `sequence` to order them deliberately. If the work
   maps to a Jira/Linear/GitHub/Todoist item, set top-level
   `metadata.issue_url`, `metadata.issue_id`, and `metadata.issue_title`;
   use per-entry `metadata` only when a capture maps to a different or
   more specific work item.
6. **Publish.** Run `bv evidence --from evidence.json --push --mode remote`.
   Capture the returned `{url, expires_at}` JSON. If the CLI reports that
   login is required, tell the human to run `bv login` or use local mode
   manually for a same-machine preview.
7. **Surface the URL to the human.** Tell them what you delivered and link
   the gallery.

## Rules

1. NEVER skirt around UI complexity to force the app into a particular
   state. Use the app as it is. If you hit blocking issues, resolve them
   to achieve your goal.
2. NEVER publish proof of unfinished work. If `bv evidence --push --mode remote` would
   represent the work as done when it is not, fix the work first.
3. NEVER claim the work is delivered to the human until `bv evidence
--push --mode remote` has returned a `{url, expires_at}` payload AND you have
   surfaced that URL to the human. If `--push` returns an error
   (HTTP 4xx, network failure, anything else), surface the error
   verbatim and the work is NOT delivered.
4. If you run into major, epic-level changes that are needed, stop and
   ask the user for a decision before proceeding.

## Logistics

- Working directory: put captures in `./.butverify/<short-task-id>/`.
- Remote publish URLs are private — recipients sign in with GitHub.
- Local mode is available for same-machine previews, but it serves only
  while the CLI process is running and does not produce an expiring
  butverify.dev URL.
- The free tier allows 30 remote evidence sites per month; if `--push` returns
  HTTP 402, the human's tenant is over cap.
