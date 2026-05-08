// pebble-60yj — Viewer-side listing of own reviews on a site.
//
// Calls GET /v1/sites/:id/reviews (cookie auth). The control-plane
// returns 0 or 1 reviews today (per the (site_id, reviewer_login)
// UNIQUE) but the array contract leaves room for future revisions.
//
// Failure modes mirror the submit client: a structured `kind` field so
// the UI can branch without parsing English. `unauthenticated` means
// the viewer cookie expired or was never minted; the page should fall
// back to "no past reviews" rather than presenting an error, since the
// gallery itself is browseable without a cookie.

export type ReviewStatus = "submitted" | "acknowledged";

export interface ViewerReview {
  review_id: string;
  site_id: string;
  submitted_at: string;
  acknowledged_at: string | null;
  annotation_count: number;
  status: ReviewStatus;
}

export interface ListReviewsForViewerSuccess {
  ok: true;
  reviews: ViewerReview[];
}

export type ListReviewsForViewerFailureKind =
  | "unauthenticated" // 401 — cookie missing/invalid
  | "site_not_found" // 404
  | "rate_limited" // 429
  | "network" // fetch threw
  | "unknown";

export interface ListReviewsForViewerFailure {
  ok: false;
  kind: ListReviewsForViewerFailureKind;
  status?: number;
  message: string;
}

export type ListReviewsForViewerResult =
  | ListReviewsForViewerSuccess
  | ListReviewsForViewerFailure;

interface ListReviewsForViewerOptions {
  apiBase: string;
  siteId: string;
  fetchImpl?: typeof fetch;
  signal?: AbortSignal;
}

interface ApiErrorBody {
  error?: {
    code?: string;
    message?: string;
  };
}

interface ApiSuccessBody {
  reviews?: unknown;
}

function isViewerReview(raw: unknown): raw is ViewerReview {
  if (typeof raw !== "object" || raw === null) return false;
  const r = raw as Record<string, unknown>;
  return (
    typeof r.review_id === "string" &&
    typeof r.site_id === "string" &&
    typeof r.submitted_at === "string" &&
    (r.acknowledged_at === null || typeof r.acknowledged_at === "string") &&
    typeof r.annotation_count === "number" &&
    (r.status === "submitted" || r.status === "acknowledged")
  );
}

export async function listReviewsForViewer(
  opts: ListReviewsForViewerOptions,
): Promise<ListReviewsForViewerResult> {
  const url = `${opts.apiBase}/v1/sites/${encodeURIComponent(opts.siteId)}/reviews`;
  const fetchImpl = opts.fetchImpl ?? fetch;
  let res: Response;
  try {
    const init: RequestInit = {
      method: "GET",
      // Cookie auth — Domain=.butverify.dev so credentials:'include'
      // attaches bv_viewer on the cross-origin GET to api.butverify.dev.
      credentials: "include",
      headers: { accept: "application/json" },
    };
    if (opts.signal) init.signal = opts.signal;
    res = await fetchImpl(url, init);
  } catch (err) {
    return {
      ok: false,
      kind: "network",
      message: err instanceof Error ? err.message : "network error",
    };
  }
  if (res.status === 200) {
    let body: ApiSuccessBody = {};
    try {
      body = (await res.json()) as ApiSuccessBody;
    } catch {
      return { ok: false, kind: "unknown", message: "malformed response", status: 200 };
    }
    const reviews = Array.isArray(body.reviews) ? body.reviews.filter(isViewerReview) : [];
    return { ok: true, reviews };
  }

  let parsed: ApiErrorBody = {};
  try {
    parsed = (await res.json()) as ApiErrorBody;
  } catch {
    // ignored
  }
  const message = parsed.error?.message ?? `HTTP ${res.status}`;
  switch (res.status) {
    case 401:
      return { ok: false, kind: "unauthenticated", status: 401, message };
    case 404:
      return { ok: false, kind: "site_not_found", status: 404, message };
    case 429:
      return { ok: false, kind: "rate_limited", status: 429, message };
    default:
      return { ok: false, kind: "unknown", status: res.status, message };
  }
}
