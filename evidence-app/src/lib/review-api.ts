// pebble-4nwj — POST /v1/sites/:id/reviews client.
//
// Maps the control-plane response shape to a small set of result
// outcomes the UI can render directly. The control-plane returns the
// standard `{error:{code,message,…}}` envelope on failure, except for
// EV2-N-1 (`empty_review`) and EV2-S-3 (`site_not_accepting_reviews`)
// which carry domain codes in `error.code` so we don't have to
// string-match on `message`.

import {
  toWireAnnotation,
  trimAnnotationForType,
  type AnnotationDraft,
  type AnnotationInput,
} from "./annotations.js";

export interface SubmitReviewSuccess {
  ok: true;
  review_id: string;
  expires_at: string | null;
  annotation_count: number;
}

export type SubmitReviewFailureKind =
  | "site_not_accepting_reviews" // 409 — expired/draining/deleted
  | "review_already_submitted" // 409 — viewer already reviewed this site
  | "empty_review" // 400 — should be caught client-side first
  | "unauthenticated" // 401 — cookie missing/invalid
  | "rate_limited" // 429 — per-viewer limiter
  | "validation_failed" // 422 — per-field annotation rule failure
  | "site_not_found" // 404 — including enable_reviews=false
  | "network" // fetch threw / no response
  | "unknown"; // anything else

export interface SubmitReviewFailure {
  ok: false;
  kind: SubmitReviewFailureKind;
  status?: number;
  message: string;
  retry_after_seconds?: number;
}

export type SubmitReviewResult = SubmitReviewSuccess | SubmitReviewFailure;

interface SubmitReviewOptions {
  apiBase: string;
  siteId: string;
  drafts: AnnotationDraft[];
  // Test seam: defaults to globalThis.fetch.
  fetchImpl?: typeof fetch;
  // AbortSignal to cancel the request (e.g. component unmount).
  signal?: AbortSignal;
}

interface ApiErrorBody {
  error?: {
    code?: string;
    message?: string;
    details?: Record<string, unknown>;
  };
}

export async function submitReview(opts: SubmitReviewOptions): Promise<SubmitReviewResult> {
  const wire: AnnotationInput[] = opts.drafts.map((d) =>
    trimAnnotationForType(toWireAnnotation(d)),
  );
  const url = `${opts.apiBase}/v1/sites/${encodeURIComponent(opts.siteId)}/reviews`;
  const fetchImpl = opts.fetchImpl ?? fetch;
  let res: Response;
  try {
    const init: RequestInit = {
      method: "POST",
      // Cookie auth — bv_viewer is on Domain=.butverify.dev so the
      // browser will attach it to the cross-origin POST when
      // credentials:'include'.
      credentials: "include",
      headers: {
        "content-type": "application/json",
        accept: "application/json",
      },
      body: JSON.stringify({ annotations: wire }),
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

  if (res.status === 201) {
    let body: { review_id?: unknown; expires_at?: unknown; annotation_count?: unknown } = {};
    try {
      body = (await res.json()) as typeof body;
    } catch {
      // 201 with non-JSON body shouldn't happen, but treat as success
      // anyway — the row landed.
    }
    return {
      ok: true,
      review_id: typeof body.review_id === "string" ? body.review_id : "",
      expires_at: typeof body.expires_at === "string" ? body.expires_at : null,
      annotation_count: typeof body.annotation_count === "number" ? body.annotation_count : 0,
    };
  }

  // Failure paths.
  let parsed: ApiErrorBody = {};
  try {
    parsed = (await res.json()) as ApiErrorBody;
  } catch {
    // Server returned non-JSON; carry the status only.
  }
  const code = parsed.error?.code ?? "";
  const message = parsed.error?.message ?? `HTTP ${res.status}`;

  // Map to a kind. Domain codes (set by the control-plane in
  // `error.code`) take precedence over the HTTP status because the
  // server emits them precisely so clients can branch without parsing
  // English messages.
  const failure: SubmitReviewFailure = (() => {
    if (code === "site_not_accepting_reviews") {
      return { ok: false as const, kind: "site_not_accepting_reviews" as const, status: res.status, message };
    }
    if (code === "review_already_submitted") {
      return { ok: false as const, kind: "review_already_submitted" as const, status: res.status, message };
    }
    if (code === "empty_review") {
      return { ok: false as const, kind: "empty_review" as const, status: res.status, message };
    }
    switch (res.status) {
      case 401:
        return { ok: false as const, kind: "unauthenticated" as const, status: 401, message };
      case 404:
        return { ok: false as const, kind: "site_not_found" as const, status: 404, message };
      case 422:
        return { ok: false as const, kind: "validation_failed" as const, status: 422, message };
      case 429: {
        const retry =
          typeof parsed.error?.details?.retry_after_seconds === "number"
            ? (parsed.error.details.retry_after_seconds as number)
            : undefined;
        const f: SubmitReviewFailure = {
          ok: false,
          kind: "rate_limited",
          status: 429,
          message,
        };
        if (retry !== undefined) f.retry_after_seconds = retry;
        return f;
      }
      default:
        return { ok: false as const, kind: "unknown" as const, status: res.status, message };
    }
  })();
  return failure;
}
