// pebble-6fux — getReviewDetail() tests.

import { describe, expect, it, vi } from "vitest";
import { getReviewDetail } from "../src/lib/review-detail.js";

function fetchReturning(status: number, body?: unknown): typeof fetch {
  return vi.fn(async () => {
    const init: ResponseInit = { status };
    if (body !== undefined) {
      init.headers = { "content-type": "application/json" };
    }
    return new Response(body !== undefined ? JSON.stringify(body) : null, init);
  }) as unknown as typeof fetch;
}

const SAMPLE_REVIEW = {
  review_id: "rev_abc",
  site_id: "abc",
  submitted_at: "2026-05-07T12:00:00Z",
  acknowledged_at: null,
  annotation_count: 2,
  status: "submitted" as const,
  annotations: [
    {
      annotation_id: "ann_1",
      type: "site_comment",
      item_index: null,
      comment: "overall good",
      region_shape: null,
      region_x: null,
      region_y: null,
      region_width: null,
      region_height: null,
      text_scope: null,
      char_start: null,
      char_end: null,
      text_snippet: null,
      created_at: "2026-05-07T12:00:00Z",
    },
    {
      annotation_id: "ann_2",
      type: "image_region",
      item_index: 0,
      comment: "this part",
      region_shape: "circle",
      region_x: 0.1,
      region_y: 0.2,
      region_width: 0.3,
      region_height: 0.3,
      text_scope: null,
      char_start: null,
      char_end: null,
      text_snippet: null,
      created_at: "2026-05-07T12:00:00.001Z",
    },
  ],
};

describe("getReviewDetail", () => {
  it("returns the parsed review with annotations on 200", async () => {
    const fetchImpl = fetchReturning(200, SAMPLE_REVIEW);
    const res = await getReviewDetail({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      reviewId: "rev_abc",
      fetchImpl,
    });
    expect(res.ok).toBe(true);
    if (res.ok) {
      expect(res.review.review_id).toBe("rev_abc");
      expect(res.review.annotations).toHaveLength(2);
      expect(res.review.annotations[0]?.type).toBe("site_comment");
      expect(res.review.annotations[1]?.region_shape).toBe("circle");
    }
  });

  it("rejects malformed top-level response shape", async () => {
    const fetchImpl = fetchReturning(200, { not: "a review" });
    const res = await getReviewDetail({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      reviewId: "rev_abc",
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "unknown", status: 200 });
  });

  it("rejects when annotations contains a malformed entry", async () => {
    const fetchImpl = fetchReturning(200, {
      ...SAMPLE_REVIEW,
      annotations: [
        SAMPLE_REVIEW.annotations[0],
        { annotation_id: "bad", type: "wat" },
      ],
    });
    const res = await getReviewDetail({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      reviewId: "rev_abc",
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "unknown" });
  });

  it("maps 401 to unauthenticated", async () => {
    const fetchImpl = fetchReturning(401, {
      error: { code: "UNAUTHENTICATED", message: "viewer cookie required" },
    });
    const res = await getReviewDetail({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      reviewId: "rev_abc",
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "unauthenticated", status: 401 });
  });

  it("maps 404 to not_found", async () => {
    const fetchImpl = fetchReturning(404, {
      error: { code: "NOT_FOUND", message: "review not found" },
    });
    const res = await getReviewDetail({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      reviewId: "rev_abc",
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "not_found", status: 404 });
  });

  it("maps 429 to rate_limited", async () => {
    const fetchImpl = fetchReturning(429, {
      error: { code: "RATE_LIMITED", message: "slow down" },
    });
    const res = await getReviewDetail({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      reviewId: "rev_abc",
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "rate_limited", status: 429 });
  });

  it("returns network kind when fetch throws", async () => {
    const fetchImpl = vi.fn(async () => {
      throw new TypeError("Failed to fetch");
    }) as unknown as typeof fetch;
    const res = await getReviewDetail({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      reviewId: "rev_abc",
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "network" });
  });

  it("calls GET on the correct URL with credentials:include", async () => {
    let capturedUrl: RequestInfo | URL | undefined;
    let capturedInit: RequestInit | undefined;
    const fetchImpl = vi.fn(async (url, init?: RequestInit) => {
      capturedUrl = url;
      capturedInit = init;
      return new Response(JSON.stringify(SAMPLE_REVIEW), { status: 200 });
    }) as unknown as typeof fetch;
    await getReviewDetail({
      apiBase: "https://api.butverify.dev",
      siteId: "my-site",
      reviewId: "rev_abc",
      fetchImpl,
    });
    expect(capturedUrl).toBe("https://api.butverify.dev/v1/sites/my-site/reviews/rev_abc");
    expect(capturedInit?.method).toBe("GET");
    expect(capturedInit?.credentials).toBe("include");
  });

  it("URL-encodes the site_id and review_id", async () => {
    let capturedUrl: RequestInfo | URL | undefined;
    const fetchImpl = vi.fn(async (url) => {
      capturedUrl = url;
      return new Response(JSON.stringify(SAMPLE_REVIEW), { status: 200 });
    }) as unknown as typeof fetch;
    await getReviewDetail({
      apiBase: "https://api.butverify.dev",
      siteId: "weird id?",
      reviewId: "rev/with/slash",
      fetchImpl,
    });
    expect(capturedUrl).toBe(
      "https://api.butverify.dev/v1/sites/weird%20id%3F/reviews/rev%2Fwith%2Fslash",
    );
  });
});
