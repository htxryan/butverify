// pebble-4nwj — Annotation type model. Mirrors the control-plane's
// `AnnotationInput` (apps/control-plane/src/db/reviews.ts) and the spec
// at docs/specs/evidence-v2.md §4.1. Defining the type here (rather than
// importing from a workspace package) keeps the public evidence-app
// independent of the private service repo's TS packages — the wire
// format is the contract.

export type AnnotationType =
  | "site_comment"
  | "item_comment"
  | "image_region"
  | "text_highlight";

export type RegionShape = "circle" | "rect";
export type TextScope = "site" | "item";

// One submitted annotation. Field requirements per spec §4.3:
//   site_comment   → comment
//   item_comment   → comment + item_index
//   image_region   → comment + item_index + region_shape + region_x + region_y + region_width + region_height
//   text_highlight → comment + text_scope + char_start + char_end + text_snippet (+ item_index when text_scope='item')
export interface AnnotationInput {
  type: AnnotationType;
  comment: string;
  item_index?: number;
  region_shape?: RegionShape;
  region_x?: number;
  region_y?: number;
  region_width?: number;
  region_height?: number;
  text_scope?: TextScope;
  char_start?: number;
  char_end?: number;
  text_snippet?: string;
}

// Draft annotation — same wire shape plus a transient client-side id so
// the UI can address individual drafts (delete, focus, edit) before
// they're submitted. The id is NEVER sent to the server; the server
// generates `annotation_id` on submit.
export interface AnnotationDraft extends AnnotationInput {
  draft_id: string;
}

// Generate a draft id. Stable enough for client use (UUID v4 from
// crypto.randomUUID, falling back to a timestamp+random for ancient
// browsers that somehow shipped without subtle crypto). Not security-
// critical — just a Svelte keyed-each key.
export function newDraftId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

const MAX_COMMENT_LENGTH = 4096;
const MAX_TEXT_SNIPPET_LENGTH = 1024;

// Client-side validation. Mirrors the server's parseAnnotation rules so
// users get fast feedback BEFORE a 422 round-trip; any mismatch must be
// fixed on both sides. Returns null on success or a human-readable
// error string. We stop at the first failure to keep the UX simple.
export function validateAnnotation(a: AnnotationInput): string | null {
  if (typeof a.comment !== "string" || a.comment.length === 0) {
    return "Comment is required";
  }
  if (a.comment.length > MAX_COMMENT_LENGTH) {
    return `Comment must be at most ${MAX_COMMENT_LENGTH} characters`;
  }
  switch (a.type) {
    case "site_comment":
      return null;
    case "item_comment":
      if (typeof a.item_index !== "number" || a.item_index < 0) {
        return "Item index is required for item comments";
      }
      return null;
    case "image_region":
      if (typeof a.item_index !== "number" || a.item_index < 0) {
        return "Item index is required for image regions";
      }
      if (a.region_shape !== "circle" && a.region_shape !== "rect") {
        return "Region shape must be circle or rect";
      }
      for (const f of [
        "region_x",
        "region_y",
        "region_width",
        "region_height",
      ] as const) {
        const v = a[f];
        if (typeof v !== "number" || !Number.isFinite(v) || v < 0 || v > 1) {
          return `${f} must be a number in [0, 1]`;
        }
      }
      return null;
    case "text_highlight":
      if (a.text_scope !== "site" && a.text_scope !== "item") {
        return "Text scope must be site or item";
      }
      if (typeof a.char_start !== "number" || !Number.isInteger(a.char_start) || a.char_start < 0) {
        return "char_start must be a non-negative integer";
      }
      if (typeof a.char_end !== "number" || !Number.isInteger(a.char_end)) {
        return "char_end must be an integer";
      }
      if (a.char_end <= a.char_start) {
        return "char_end must be greater than char_start";
      }
      if (typeof a.text_snippet !== "string" || a.text_snippet.length === 0) {
        return "text_snippet is required";
      }
      if (a.text_snippet.length > MAX_TEXT_SNIPPET_LENGTH) {
        return `text_snippet must be at most ${MAX_TEXT_SNIPPET_LENGTH} characters`;
      }
      if (a.text_scope === "item" && (typeof a.item_index !== "number" || a.item_index < 0)) {
        return "item_index is required when text_scope is 'item'";
      }
      return null;
  }
}

// Strip the draft_id before posting. The server rejects unknown fields
// implicitly (it reads only the typed columns) but stripping keeps the
// wire payload predictable.
export function toWireAnnotation(d: AnnotationDraft): AnnotationInput {
  const { draft_id: _draft, ...rest } = d;
  return rest;
}

// Drop fields that are NULL/undefined for this annotation type, to
// match the server's typed-column expectations. The server's parser
// ignores extra fields gracefully but persists only the typed columns
// for the matched type — sending stale shape fields would be wasted
// bytes, not a bug.
export function trimAnnotationForType(a: AnnotationInput): AnnotationInput {
  switch (a.type) {
    case "site_comment":
      return { type: a.type, comment: a.comment };
    case "item_comment": {
      const out: AnnotationInput = { type: a.type, comment: a.comment };
      if (a.item_index !== undefined) out.item_index = a.item_index;
      return out;
    }
    case "image_region": {
      const out: AnnotationInput = {
        type: a.type,
        comment: a.comment,
      };
      if (a.item_index !== undefined) out.item_index = a.item_index;
      if (a.region_shape !== undefined) out.region_shape = a.region_shape;
      if (a.region_x !== undefined) out.region_x = a.region_x;
      if (a.region_y !== undefined) out.region_y = a.region_y;
      if (a.region_width !== undefined) out.region_width = a.region_width;
      if (a.region_height !== undefined) out.region_height = a.region_height;
      return out;
    }
    case "text_highlight": {
      const out: AnnotationInput = {
        type: a.type,
        comment: a.comment,
      };
      if (a.text_scope !== undefined) out.text_scope = a.text_scope;
      if (a.char_start !== undefined) out.char_start = a.char_start;
      if (a.char_end !== undefined) out.char_end = a.char_end;
      if (a.text_snippet !== undefined) out.text_snippet = a.text_snippet;
      // item_index only when scope='item'.
      if (a.text_scope === "item" && a.item_index !== undefined) {
        out.item_index = a.item_index;
      }
      return out;
    }
  }
}
