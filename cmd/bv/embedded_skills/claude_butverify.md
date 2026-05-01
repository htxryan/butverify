---
name: butverify
description: Publish proof of recent work as a private gallery on butverify.dev
disable-model-invocation: false
bv-skill-version: 0000000000ab
---

# /butverify

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
   for each entry; use `sequence` to order them deliberately.
6. **Publish.** Run `bv evidence --from evidence.json --push`. Capture the
   returned `{url, expires_at}` JSON.
7. **Surface the URL to the human.** Tell them what you delivered and link
   the gallery.

## Rules

1. NEVER skirt around UI complexity to force the app into a particular
   state. Use the app as it is. If you hit blocking issues, resolve them
   to achieve your goal.
2. NEVER publish proof of unfinished work. If `bv evidence --push` would
   represent the work as done when it is not, fix the work first.
3. NEVER claim the work is delivered to the human until `bv evidence
--push` has returned a `{url, expires_at}` payload AND you have
   surfaced that URL to the human. If `--push` returns an error
   (HTTP 4xx, network failure, anything else), surface the error
   verbatim and the work is NOT delivered.
4. If you run into major, epic-level changes that are needed, stop and
   ask the user for a decision before proceeding.

## Logistics

- Working directory: put captures in `./.butverify/<short-task-id>/`.
- After publish, the URL is private — recipients sign in with GitHub.
- The free tier allows 30 evidence sites per month; if `--push` returns
  HTTP 402, the human's tenant is over cap.
