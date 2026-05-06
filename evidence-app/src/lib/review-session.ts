// pebble-4nwj — Review session context.
//
// The orchestrator (Gallery.svelte) sets a ReviewSession object on the
// Svelte context; per-item components (GalleryItem.svelte) read it to
// render annotation tools and report new draft annotations back up.
//
// Why context instead of props? GalleryItem is reached through two
// layout shells (StackedLayout / CarouselLayout). Threading 6 review
// callbacks through both layouts (which know nothing about reviews)
// would couple unrelated components; a context keeps the layouts
// review-agnostic.

import { getContext, setContext } from "svelte";
import type {
  AnnotationDraft,
  AnnotationInput,
  RegionShape,
} from "./annotations.js";

const KEY = Symbol("bv.review-session");

export interface ReviewSession {
  // Read this in components to know whether review affordances should
  // render at all. Driven by manifest.enable_reviews and (later) by
  // the customer-site Worker confirming the cookie is present.
  enabled: boolean;
  // The site id the reviews target. Used by the panel for the POST
  // URL and by the draft-store for namespacing.
  siteId: string;
  // Read-only view of the draft list. Components filter this for
  // their own item_index.
  drafts: () => AnnotationDraft[];
  // Mutators. Each returns the new full list of drafts after the
  // change so the caller doesn't have to call drafts() again.
  add: (input: AnnotationInput) => AnnotationDraft;
  remove: (draftId: string) => void;
}

export function setReviewSession(session: ReviewSession): void {
  setContext(KEY, session);
}

// Returns null when no session is active (manifest.enable_reviews
// false, or the gallery is rendered outside a ReviewProvider).
// Components should branch on null and render the non-review UX.
export function getReviewSession(): ReviewSession | null {
  return (getContext(KEY) as ReviewSession | undefined) ?? null;
}

// Convenience: build the AnnotationInput payload for each annotation
// type from the captured geometry. Keeps the per-component code
// shallow.

export function siteCommentInput(comment: string): AnnotationInput {
  return { type: "site_comment", comment };
}

export function itemCommentInput(itemIndex: number, comment: string): AnnotationInput {
  return { type: "item_comment", item_index: itemIndex, comment };
}

export function imageRegionInput(
  itemIndex: number,
  shape: RegionShape,
  coords: { x: number; y: number; width: number; height: number },
  comment: string,
): AnnotationInput {
  return {
    type: "image_region",
    item_index: itemIndex,
    region_shape: shape,
    region_x: coords.x,
    region_y: coords.y,
    region_width: coords.width,
    region_height: coords.height,
    comment,
  };
}

export function textHighlightInput(
  scope: "site" | "item",
  itemIndex: number | null,
  charStart: number,
  charEnd: number,
  snippet: string,
  comment: string,
): AnnotationInput {
  const out: AnnotationInput = {
    type: "text_highlight",
    text_scope: scope,
    char_start: charStart,
    char_end: charEnd,
    text_snippet: snippet,
    comment,
  };
  if (scope === "item" && itemIndex !== null) {
    out.item_index = itemIndex;
  }
  return out;
}
