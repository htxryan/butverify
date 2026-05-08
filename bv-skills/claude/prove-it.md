---
name: prove-it
description: Publish proof of recent work as a private gallery on butverify.dev
disable-model-invocation: false
---

# /butverify:prove-it

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
6. **Start the validation workflow.** Run `bv evidence --from evidence.json`
   (no `--push`). The CLI renders the bundle and prints a prompt for the
   first item.
7. **Validate each item.** For each item prompt:
   1. Open the screenshot, recording, or output file at the path shown.
   2. Confirm it accurately shows what is described.
   3. Check for errors, unexpected behavior, or incomplete states.
   4. If anything is wrong, fix the application and restart with
      `bv evidence --from evidence.json`.
   5. When satisfied, run `bv evidence validate --item N` (where N is the
      item number shown in the prompt).

   **For web apps:** also open browser DevTools → Console before
   validating each item and confirm zero JS errors or exceptions.
   A screenshot of a crashed Svelte/React/Vue app can look visually
   normal — the console is the ground truth.

   **For CLI apps:** verify exit codes, stderr content, and that stdout
   matches the expected format described. Capture error-path output or
   `--help` as separate items when those are part of the feature.

   **For APIs / services:** include a real request/response pair as an
   evidence item — not just server logs. Show the public surface a caller
   would see.
8. **Publish automatically.** After all items are validated the CLI
   publishes via the standard push flow and returns `{url, expires_at}`.
   Capture that JSON. If the CLI reports that login is required, tell the
   human to run `bv login` or use local mode manually for a same-machine
   preview.
9. **Surface the URL to the human.** Tell them what you delivered and link
   the gallery.

### Skip validation (escape hatch)

If you need to publish immediately without the per-item review loop, use:

`bv evidence --from evidence.json --push --mode remote`

This bypasses validation and pushes directly. Only use this escape hatch
when the validation workflow is genuinely inappropriate (e.g. automated CI
pipelines).

## Rules

1. NEVER skirt around UI complexity to force the app into a particular
   state. Use the app as it is. If you hit blocking issues, resolve them
   to achieve your goal.
2. NEVER publish proof of unfinished work. If publishing would represent
   the work as done when it is not, fix the work first.
3. NEVER claim the work is delivered to the human until the push step has
   returned a `{url, expires_at}` payload AND you have surfaced that URL
   to the human. If `--push` returns an error (HTTP 4xx, network failure,
   anything else), surface the error verbatim and the work is NOT delivered.
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
