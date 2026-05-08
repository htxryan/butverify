---
name: review
description: Surface and acknowledge unacknowledged reviews on your most recently published butverify gallery
disable-model-invocation: false
---

# /butverify:review

Use this skill to surface human feedback (reviews) on the evidence galleries
you have published with `/butverify:prove-it`. Reviews are short typed
comments that humans leave on specific items, image regions, or text
highlights inside a published gallery.

## Workflow

1. **Identify the most recently published evidence site.** Look at the
   most recent `bv evidence --push --mode remote` URL you surfaced to the
   human. Extract the site_id from the URL host
   (`<site_id>.butverify.dev`). If you cannot find a recent site, ask the
   human which site to check.
2. **List unacknowledged reviews for that site.** Run
   `bv review list --site <site-id> --unacknowledged`. The CLI returns a
   JSON array of summaries. If the array is empty, there is no human
   feedback to act on — surface that fact and stop.
3. **Retrieve each review's full content.** For every entry returned by
   step 2, run `bv review get <review-id>`. The response includes every
   annotation the reviewer made (`type`, `comment`, `item_index`,
   `region_*` for image regions, `char_*` + `text_snippet` for text
   highlights).
4. **Summarize the feedback for the human.** Restate each annotation
   verbatim — do NOT paraphrase or omit. Preserve the reviewer's wording
   so the human sees exactly what was said.
5. **Decide what to do.** For each annotation:
   - If it points at a real issue, fix it. After fixing, re-run
     `/butverify:prove-it` so a new proof is published.
   - If it is a question or out-of-scope request, surface it back to the
     human and let them decide.
6. **Acknowledge each review only after you have processed it.** Run
   `bv review acknowledge <review-id>` once you have either fixed the
   issue or surfaced it to the human. Acknowledgment is idempotent — a
   second call returns 200 — so it is safe to retry.
7. **Surface the outcome.** Tell the human (a) which reviews you found,
   (b) what you did about each one, (c) whether each was acknowledged.

## Rules

1. NEVER acknowledge a review before you have actually processed it. The
   acknowledgment removes it from the agent's `--unacknowledged` queue
   permanently — premature acknowledgment will lose the human's feedback.
2. NEVER paraphrase the reviewer's comment when surfacing it to the
   human. The reviewer's exact wording matters; the human is reading
   their own past words.
3. NEVER attempt to delete or modify a submitted review. Reviews are
   immutable once submitted; only acknowledge them.
4. If a review points at work that requires major, epic-level changes,
   stop and ask the human for a decision before fixing anything.

## Logistics

- Reviews live behind your installation token; nothing about this skill
  is visible to the human's terminal until you summarize what you
  retrieved.
- If `bv review list` returns an authentication error, the installation
  token has expired or been revoked — surface the error and ask the
  human to re-run `bv login`.
- If a site has been deleted or its TTL has expired, `bv review list`
  may still return reviews submitted before expiry; you can still
  acknowledge them, but a fix-and-republish cycle will need a fresh
  `bv evidence --push --mode remote` (the original URL is gone).
