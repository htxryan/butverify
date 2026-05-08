// pebble-60yj — listReviewsForViewer() tests.

import { describe, expect, it, vi } from "vitest";
import { listReviewsForViewer } from "../src/lib/review-list.js";

function fetchReturning(status: number, body?: unknown): typeof fetch {
  return vi.fn(async () => {
    const init: ResponseInit = { status };
    if (body !== undefined) {
      init.headers = { "content-type": "application/json" };
    }
    return new Response(body !== undefined ? JSON.stringify(body) : null, init);
  }) as unknown as typeof fetch;
}

describe("listReviewsForViewer", () => {
  it("returns the parsed reviews array on 200", async () => {
    const fetchImpl = fetchReturning(200, {
      reviews: [
        {
          review_id: "rev_a",
          site_id: "abc",
          submitted_at: "2026-05-07T12:00:00Z",
          acknowledged_at: null,
          annotation_count: 3,
          status: "submitted",
        },
        {
          review_id: "rev_b",
          site_id: "abc",
          submitted_at: "2026-05-06T12:00:00Z",
          acknowledged_at: "2026-05-07T01:00:00Z",
          annotation_count: 1,
          status: "acknowledged",
        },
      ],
    });
    const res = await listReviewsForViewer({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      fetchImpl,
    });
    expect(res.ok).toBe(true);
    if (res.ok) {
      expect(res.reviews).toHaveLength(2);
      expect(res.reviews[0]?.review_id).toBe("rev_a");
      expect(res.reviews[1]?.status).toBe("acknowledged");
    }
  });

  it("returns an empty array when the server returns no reviews", async () => {
    const fetchImpl = fetchReturning(200, { reviews: [] });
    const res = await listReviewsForViewer({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      fetchImpl,
    });
    expect(res).toEqual({ ok: true, reviews: [] });
  });

  it("filters out malformed review objects from the response", async () => {
    const fetchImpl = fetchReturning(200, {
      reviews: [
        {
          review_id: "rev_ok",
          site_id: "abc",
          submitted_at: "2026-05-07T12:00:00Z",
          acknowledged_at: null,
          annotation_count: 1,
          status: "submitted",
        },
        // missing required fields
        { review_id: "rev_bad", status: "??" },
        // wrong types
        {
          review_id: 42,
          site_id: "abc",
          submitted_at: "x",
          acknowledged_at: null,
          annotation_count: 1,
          status: "submitted",
        },
      ],
    });
    const res = await listReviewsForViewer({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      fetchImpl,
    });
    expect(res.ok).toBe(true);
    if (res.ok) {
      expect(res.reviews).toHaveLength(1);
      expect(res.reviews[0]?.review_id).toBe("rev_ok");
    }
  });

  it("maps 401 to unauthenticated", async () => {
    const fetchImpl = fetchReturning(401, {
      error: { code: "UNAUTHENTICATED", message: "viewer cookie required" },
    });
    const res = await listReviewsForViewer({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "unauthenticated", status: 401 });
  });

  it("maps 404 to site_not_found", async () => {
    const fetchImpl = fetchReturning(404, {
      error: { code: "NOT_FOUND", message: "site not found" },
    });
    const res = await listReviewsForViewer({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "site_not_found", status: 404 });
  });

  it("returns network kind when fetch throws", async () => {
    const fetchImpl = vi.fn(async () => {
      throw new TypeError("Failed to fetch");
    }) as unknown as typeof fetch;
    const res = await listReviewsForViewer({
      apiBase: "https://api.butverify.dev",
      siteId: "abc",
      fetchImpl,
    });
    expect(res).toMatchObject({ ok: false, kind: "network" });
  });

  it("calls GET on the correct site URL with credentials:include", async () => {
    let capturedUrl: RequestInfo | URL | undefined;
    let capturedInit: RequestInit | undefined;
    const fetchImpl = vi.fn(async (url, init?: RequestInit) => {
      capturedUrl = url;
      capturedInit = init;
      return new Response(JSON.stringify({ reviews: [] }), { status: 200 });
    }) as unknown as typeof fetch;
    await listReviewsForViewer({
      apiBase: "https://api.butverify.dev",
      siteId: "my-site",
      fetchImpl,
    });
    expect(capturedUrl).toBe("https://api.butverify.dev/v1/sites/my-site/reviews");
    expect(capturedInit?.method).toBe("GET");
    expect(capturedInit?.credentials).toBe("include");
  });

  it("URL-encodes the site id", async () => {
    let capturedUrl: RequestInfo | URL | undefined;
    const fetchImpl = vi.fn(async (url) => {
      capturedUrl = url;
      return new Response(JSON.stringify({ reviews: [] }), { status: 200 });
    }) as unknown as typeof fetch;
    await listReviewsForViewer({
      apiBase: "https://api.butverify.dev",
      siteId: "weird id?",
      fetchImpl,
    });
    expect(capturedUrl).toBe("https://api.butverify.dev/v1/sites/weird%20id%3F/reviews");
  });
});
