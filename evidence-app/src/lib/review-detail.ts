// pebble-6fux — Viewer-side fetch for a single review with annotations.
//
// Calls GET /v1/sites/:id/reviews/:review_id (cookie auth). The
// control-plane verifies the review belongs to this viewer on this site
// — any mismatch (wrong site, another reviewer's review_id, unknown id)
// returns 404 with no leakage. The Review Details page uses this to
// hydrate the gallery's annotation overlays from server data instead of
// the local draft store.
//
// Failure modes mirror review-list.ts: a structured `kind` so the UI can
// branch without parsing English. `unauthenticated` typically means the
// viewer cookie expired; the page should suggest re-signing in or
// re-loading. `not_found` covers all the 404 cases above (the page
// surfaces "this review doesn't exist or isn't yours").

import type {
  AnnotationDraft,
  AnnotationType,
  RegionShape,
  TextScope,
} from "./annotations.js";

export type ReviewStatus = "submitted" | "acknowledged";

// One annotation row as the server returns it. Mirrors the
// AnnotationRow shape in apps/control-plane/src/db/reviews.ts. Optional
// fields are NULL when not relevant to the annotation type (e.g. a
// site_comment has all the region/text fields null).
export interface ReviewAnnotation {
  annotation_id: string;
  type: AnnotationType;
  item_index: number | null;
  comment: string;
  region_shape: RegionShape | null;
  region_x: number | null;
  region_y: number | null;
  region_width: number | null;
  region_height: number | null;
  text_scope: TextScope | null;
  char_start: number | null;
  char_end: number | null;
  text_snippet: string | null;
  created_at: string;
}

export interface ReviewDetail {
  review_id: string;
  site_id: string;
  submitted_at: string;
  acknowledged_at: string | null;
  annotation_count: number;
  status: ReviewStatus;
  annotations: ReviewAnnotation[];
}

export interface GetReviewDetailSuccess {
  ok: true;
  review: ReviewDetail;
}

export type GetReviewDetailFailureKind =
  | "unauthenticated" // 401 — cookie missing/invalid
  | "not_found" // 404 — review unknown, wrong site, or another reviewer's
  | "rate_limited" // 429
  | "network" // fetch threw
  | "unknown";

export interface GetReviewDetailFailure {
  ok: false;
  kind: GetReviewDetailFailureKind;
  status?: number;
  message: string;
}

export type GetReviewDetailResult = GetReviewDetailSuccess | GetReviewDetailFailure;

interface GetReviewDetailOptions {
  apiBase: string;
  siteId: string;
  reviewId: string;
  fetchImpl?: typeof fetch;
  signal?: AbortSignal;
}

// pebble-6fux — adapt server-side annotations to the AnnotationDraft
// shape used by the rest of the evidence-app components. The Review
// Details page renders these through the same overlay/comment paths
// the local-draft flow uses (Gallery → GalleryItem → AnnotationOverlay)
// so the page is a faithful playback of the submitted review without
// duplicating render logic.
//
// Mapping rules:
//   * draft_id ← annotation_id (stable server id; safe as Svelte key)
//   * NULLs collapsed to undefined to match AnnotationDraft typing

export function reviewAnnotationsAsDrafts(
  annotations: ReviewAnnotation[],
): AnnotationDraft[] {
  return annotations.map((a) => {
    const out: AnnotationDraft = {
      draft_id: a.annotation_id,
      type: a.type,
      comment: a.comment,
    };
    if (a.item_index !== null) out.item_index = a.item_index;
    if (a.region_shape !== null) out.region_shape = a.region_shape;
    if (a.region_x !== null) out.region_x = a.region_x;
    if (a.region_y !== null) out.region_y = a.region_y;
    if (a.region_width !== null) out.region_width = a.region_width;
    if (a.region_height !== null) out.region_height = a.region_height;
    if (a.text_scope !== null) out.text_scope = a.text_scope;
    if (a.char_start !== null) out.char_start = a.char_start;
    if (a.char_end !== null) out.char_end = a.char_end;
    if (a.text_snippet !== null) out.text_snippet = a.text_snippet;
    return out;
  });
}

interface ApiErrorBody {
  error?: {
    code?: string;
    message?: string;
  };
}

function isReviewAnnotation(raw: unknown): raw is ReviewAnnotation {
  if (typeof raw !== "object" || raw === null) return false;
  const a = raw as Record<string, unknown>;
  if (typeof a.annotation_id !== "string") return false;
  if (typeof a.comment !== "string") return false;
  if (typeof a.created_at !== "string") return false;
  // Type narrowing — only enumerated values are valid.
  const t = a.type;
  if (
    t !== "site_comment" &&
    t !== "item_comment" &&
    t !== "image_region" &&
    t !== "text_highlight"
  ) {
    return false;
  }
  return true;
}

function isReviewDetail(raw: unknown): raw is ReviewDetail {
  if (typeof raw !== "object" || raw === null) return false;
  const r = raw as Record<string, unknown>;
  if (typeof r.review_id !== "string") return false;
  if (typeof r.site_id !== "string") return false;
  if (typeof r.submitted_at !== "string") return false;
  if (r.acknowledged_at !== null && typeof r.acknowledged_at !== "string") return false;
  if (typeof r.annotation_count !== "number") return false;
  if (r.status !== "submitted" && r.status !== "acknowledged") return false;
  if (!Array.isArray(r.annotations)) return false;
  return r.annotations.every(isReviewAnnotation);
}

export async function getReviewDetail(
  opts: GetReviewDetailOptions,
): Promise<GetReviewDetailResult> {
  const url = `${opts.apiBase}/v1/sites/${encodeURIComponent(opts.siteId)}/reviews/${encodeURIComponent(opts.reviewId)}`;
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
    let body: unknown;
    try {
      body = await res.json();
    } catch {
      return { ok: false, kind: "unknown", message: "malformed response", status: 200 };
    }
    if (!isReviewDetail(body)) {
      return { ok: false, kind: "unknown", message: "unexpected response shape", status: 200 };
    }
    return { ok: true, review: body };
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
      return { ok: false, kind: "not_found", status: 404, message };
    case 429:
      return { ok: false, kind: "rate_limited", status: 429, message };
    default:
      return { ok: false, kind: "unknown", status: res.status, message };
  }
}
