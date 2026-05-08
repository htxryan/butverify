// pebble-4nwj — submitReview() failure-mapping tests.

import { describe, expect, it, vi } from "vitest";
import { submitReview } from "../src/lib/review-api.js";
import type { AnnotationDraft } from "../src/lib/annotations.js";

const drafts: AnnotationDraft[] = [
  { type: "site_comment", comment: "looks good", draft_id: "d1" },
];

function fetchReturning(status: number, body?: unknown): typeof fetch {
  return vi.fn(async () => {
    const init: ResponseInit = { status };
    if (body !== undefined) {
      init.headers = { "content-type": "application/json" };
    }
    return new Response(body !== undefined ? JSON.stringify(body) : null, init);
  }) as unknown as typeof fetch;
}

describe("submitReview", () => {
  it("returns ok=true on 201", async () => {
    const fetchImpl = fetchReturning(201, {
      review_id: "rev_x",
      expires_at: "2026-05-15T00:00:00Z",
      annotation_count: 1,
    });
    const res = await submitReview({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      drafts,
      fetchImpl,
    });
    expect(res.ok).toBe(true);
    if (res.ok) {
      expect(res.review_id).toBe("rev_x");
      expect(res.annotation_count).toBe(1);
    }
  });

  it("maps 409 site_not_accepting_reviews via error.code", async () => {
    const fetchImpl = fetchReturning(409, {
      error: {
        code: "site_not_accepting_reviews",
        message: "site_not_accepting_reviews",
      },
    });
    const res = await submitReview({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      drafts,
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "site_not_accepting_reviews", status: 409 });
  });

  it("maps 409 review_already_submitted via error.code", async () => {
    const fetchImpl = fetchReturning(409, {
      error: {
        code: "review_already_submitted",
        message: "review_already_submitted",
      },
    });
    const res = await submitReview({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      drafts,
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "review_already_submitted" });
  });

  it("maps 422 to validation_failed", async () => {
    const fetchImpl = fetchReturning(422, {
      error: { code: "UNPROCESSABLE", message: "annotations[0].region_x must be in [0,1]" },
    });
    const res = await submitReview({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      drafts,
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "validation_failed", status: 422 });
  });

  it("maps 401 to unauthenticated", async () => {
    const fetchImpl = fetchReturning(401, {
      error: { code: "UNAUTHENTICATED", message: "viewer cookie required" },
    });
    const res = await submitReview({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      drafts,
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "unauthenticated" });
  });

  it("maps 429 to rate_limited and surfaces retry_after_seconds", async () => {
    const fetchImpl = fetchReturning(429, {
      error: {
        code: "RATE_LIMITED",
        message: "review submit rate limit 30/min exceeded; retry in 12s",
        details: { retry_after_seconds: 12, limit: 30 },
      },
    });
    const res = await submitReview({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      drafts,
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "rate_limited", retry_after_seconds: 12 });
  });

  it("maps 404 to site_not_found", async () => {
    const fetchImpl = fetchReturning(404, {
      error: { code: "NOT_FOUND", message: "site not found" },
    });
    const res = await submitReview({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      drafts,
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "site_not_found" });
  });

  it("returns network kind when fetch throws", async () => {
    const fetchImpl = vi.fn(async () => {
      throw new TypeError("Failed to fetch");
    }) as unknown as typeof fetch;
    const res = await submitReview({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      drafts,
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "network" });
  });

  it("strips draft_id and surplus shape fields from the wire body", async () => {
    let captured: BodyInit | null | undefined;
    const fetchImpl = vi.fn(async (_url, init?: RequestInit) => {
      captured = init?.body;
      return new Response(
        JSON.stringify({ review_id: "rev_x", expires_at: null, annotation_count: 1 }),
        { status: 201 },
      );
    }) as unknown as typeof fetch;
    await submitReview({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      drafts: [
        {
          type: "site_comment",
          comment: "ok",
          draft_id: "d1",
          // surplus fields that should NOT make it to the wire:
          item_index: 99,
          region_shape: "rect",
          region_x: 0.1,
        },
      ],
      fetchImpl,
    });
    const parsed = JSON.parse(captured as string) as {
      annotations: Array<Record<string, unknown>>;
    };
    expect(parsed.annotations).toHaveLength(1);
    expect(parsed.annotations[0]).toEqual({ type: "site_comment", comment: "ok" });
  });

  it("posts to the correct site URL with credentials:include", async () => {
    let capturedUrl: RequestInfo | URL | undefined;
    let capturedInit: RequestInit | undefined;
    const fetchImpl = vi.fn(async (url, init?: RequestInit) => {
      capturedUrl = url;
      capturedInit = init;
      return new Response(
        JSON.stringify({ review_id: "r", expires_at: null, annotation_count: 1 }),
        { status: 201 },
      );
    }) as unknown as typeof fetch;
    await submitReview({
      apiBase: "https://api.butverify.dev",
      siteId: "abc 123 / weird",
      drafts,
      fetchImpl,
    });
    expect(capturedUrl).toBe(
      "https://api.butverify.dev/v1/sites/abc%20123%20%2F%20weird/reviews",
    );
    expect(capturedInit?.credentials).toBe("include");
  });
});
