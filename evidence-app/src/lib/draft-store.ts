// pebble-4nwj — localStorage-backed draft annotation store.
//
// Per spec EV2-S-1: while a reviewer is building a review, annotation
// state SHALL be held client-side (memory / localStorage) as a draft;
// the draft SHALL NOT be visible to the agent or other reviewers until
// submitted.
//
// The store is keyed by `bv-draft-v1:<site_id>` so drafts on different
// sites do not bleed into each other. The version prefix lets us evolve
// the on-disk schema without orphaning old drafts; a stored payload at
// `bv-draft-v0` would simply be ignored.

import {
  newDraftId,
  type AnnotationDraft,
  type AnnotationInput,
  type AnnotationType,
} from "./annotations.js";

const STORAGE_VERSION = "v1";
const STORAGE_PREFIX = `bv-draft-${STORAGE_VERSION}:`;

export function storageKeyFor(siteId: string): string {
  return `${STORAGE_PREFIX}${siteId}`;
}

// Serialized payload shape. We round-trip through this so a future
// schema change can be detected via `version`.
interface DraftPayload {
  version: typeof STORAGE_VERSION;
  site_id: string;
  saved_at: string; // ISO 8601
  drafts: AnnotationDraft[];
}

function isAnnotationType(s: unknown): s is AnnotationType {
  return (
    s === "site_comment" ||
    s === "item_comment" ||
    s === "image_region" ||
    s === "text_highlight"
  );
}

// Defensive parse — if localStorage was tampered with or written by a
// different version of the bundle, we want to silently discard it
// rather than crash boot. Returns [] for any unparseable / mismatched
// payload.
function parsePayload(raw: string | null): AnnotationDraft[] {
  if (!raw) return [];
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  if (typeof parsed !== "object" || parsed === null) return [];
  const obj = parsed as Record<string, unknown>;
  if (obj.version !== STORAGE_VERSION) return [];
  if (!Array.isArray(obj.drafts)) return [];
  // Per-draft shape filter. We accept anything that has a recognized
  // type + non-empty comment; all other fields pass through. The
  // submission code re-validates before posting.
  const out: AnnotationDraft[] = [];
  for (const d of obj.drafts) {
    if (typeof d !== "object" || d === null) continue;
    const r = d as Record<string, unknown>;
    if (!isAnnotationType(r.type)) continue;
    if (typeof r.comment !== "string") continue;
    const draft: AnnotationDraft = {
      type: r.type,
      comment: r.comment,
      draft_id: typeof r.draft_id === "string" && r.draft_id.length > 0 ? r.draft_id : newDraftId(),
    };
    if (typeof r.item_index === "number") draft.item_index = r.item_index;
    if (r.region_shape === "circle" || r.region_shape === "rect") {
      draft.region_shape = r.region_shape;
    }
    if (typeof r.region_x === "number") draft.region_x = r.region_x;
    if (typeof r.region_y === "number") draft.region_y = r.region_y;
    if (typeof r.region_width === "number") draft.region_width = r.region_width;
    if (typeof r.region_height === "number") draft.region_height = r.region_height;
    if (r.text_scope === "site" || r.text_scope === "item") {
      draft.text_scope = r.text_scope;
    }
    if (typeof r.char_start === "number") draft.char_start = r.char_start;
    if (typeof r.char_end === "number") draft.char_end = r.char_end;
    if (typeof r.text_snippet === "string") draft.text_snippet = r.text_snippet;
    out.push(draft);
  }
  return out;
}

// Storage handle abstraction so tests can pass an in-memory shim without
// touching globals. Defaults to window.localStorage.
export interface DraftStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

function defaultStorage(): DraftStorage | null {
  // localStorage may be unavailable: SSR (server-render of the dev
  // shell), private-browsing safari with quota=0, or a sandboxed iframe
  // that blocks storage. In all cases we fall back to in-memory only.
  if (typeof window === "undefined") return null;
  try {
    const probe = "__bv_probe__";
    window.localStorage.setItem(probe, probe);
    window.localStorage.removeItem(probe);
    return window.localStorage;
  } catch {
    return null;
  }
}

export interface DraftStore {
  load(): AnnotationDraft[];
  save(drafts: AnnotationDraft[]): void;
  clear(): void;
  add(input: AnnotationInput): AnnotationDraft;
  remove(draftId: string): AnnotationDraft[];
}

// Create a draft store for one site. The store reads + writes
// synchronously on each call — there is no in-memory cache here because
// the Svelte components own the reactive copy; this module is just
// persistence.
export function createDraftStore(
  siteId: string,
  storage: DraftStorage | null = defaultStorage(),
): DraftStore {
  const key = storageKeyFor(siteId);

  // Memory fallback when storage is unavailable. Drafts are still
  // accumulated and submitted within a single page visit; they just
  // don't survive a refresh.
  let memoryDrafts: AnnotationDraft[] | null = storage === null ? [] : null;

  function load(): AnnotationDraft[] {
    if (memoryDrafts !== null) return [...memoryDrafts];
    const raw = storage!.getItem(key);
    return parsePayload(raw);
  }

  function save(drafts: AnnotationDraft[]): void {
    if (memoryDrafts !== null) {
      memoryDrafts = [...drafts];
      return;
    }
    const payload: DraftPayload = {
      version: STORAGE_VERSION,
      site_id: siteId,
      saved_at: new Date().toISOString(),
      drafts,
    };
    try {
      storage!.setItem(key, JSON.stringify(payload));
    } catch {
      // Quota / serialization failure — fall back to memory only so the
      // running session keeps working. We accept the silent loss here
      // rather than blocking the user mid-review.
      memoryDrafts = [...drafts];
    }
  }

  function clear(): void {
    if (memoryDrafts !== null) {
      memoryDrafts = [];
      return;
    }
    try {
      storage!.removeItem(key);
    } catch {
      // ignore
    }
  }

  function add(input: AnnotationInput): AnnotationDraft {
    const draft: AnnotationDraft = { ...input, draft_id: newDraftId() };
    const next = [...load(), draft];
    save(next);
    return draft;
  }

  function remove(draftId: string): AnnotationDraft[] {
    const next = load().filter((d) => d.draft_id !== draftId);
    save(next);
    return next;
  }

  return { load, save, clear, add, remove };
}
