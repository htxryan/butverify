// pebble-4nwj — annotation client validation tests. Mirrors the
// server's per-type rules so a 422 from the control-plane should
// always be preceded by a client-side rejection here.

import { describe, expect, it } from "vitest";
import {
  toWireAnnotation,
  trimAnnotationForType,
  validateAnnotation,
  type AnnotationDraft,
} from "../src/lib/annotations.js";

describe("validateAnnotation", () => {
  it("requires a non-empty comment for every type", () => {
    expect(validateAnnotation({ type: "site_comment", comment: "" })).toMatch(
      /Comment is required/,
    );
    expect(validateAnnotation({ type: "site_comment", comment: "ok" })).toBeNull();
  });

  it("caps comment length at 4096", () => {
    const long = "x".repeat(4097);
    expect(validateAnnotation({ type: "site_comment", comment: long })).toMatch(/4096/);
  });

  it("item_comment requires item_index", () => {
    expect(validateAnnotation({ type: "item_comment", comment: "x" })).toMatch(
      /Item index/,
    );
    expect(
      validateAnnotation({ type: "item_comment", comment: "x", item_index: 0 }),
    ).toBeNull();
  });

  it("image_region validates shape and unit-range coords", () => {
    const base = { type: "image_region" as const, comment: "x", item_index: 0 };
    expect(
      validateAnnotation({
        ...base,
        region_shape: "circle",
        region_x: 0.1,
        region_y: 0.2,
        region_width: 0.3,
        region_height: 0.4,
      }),
    ).toBeNull();
    expect(validateAnnotation({ ...base, region_shape: "rect" } as never)).toMatch(
      /region_x must/,
    );
    expect(
      validateAnnotation({
        ...base,
        region_shape: "circle",
        region_x: 1.5,
        region_y: 0.2,
        region_width: 0.3,
        region_height: 0.4,
      }),
    ).toMatch(/region_x must/);
  });

  it("text_highlight requires scope, integer offsets, snippet", () => {
    const base = {
      type: "text_highlight" as const,
      comment: "x",
      text_scope: "site" as const,
      char_start: 0,
      char_end: 5,
      text_snippet: "hello",
    };
    expect(validateAnnotation(base)).toBeNull();
    expect(validateAnnotation({ ...base, char_end: 0 })).toMatch(/greater than/);
    expect(validateAnnotation({ ...base, text_snippet: "" })).toMatch(/snippet/);
  });

  it("text_highlight scope=item requires item_index", () => {
    const base = {
      type: "text_highlight" as const,
      comment: "x",
      text_scope: "item" as const,
      char_start: 0,
      char_end: 5,
      text_snippet: "h",
    };
    expect(validateAnnotation(base)).toMatch(/item_index/);
    expect(validateAnnotation({ ...base, item_index: 2 })).toBeNull();
  });
});

describe("toWireAnnotation / trimAnnotationForType", () => {
  it("strips draft_id", () => {
    const d: AnnotationDraft = {
      type: "site_comment",
      comment: "ok",
      draft_id: "abc",
    };
    expect(toWireAnnotation(d)).toEqual({ type: "site_comment", comment: "ok" });
  });

  it("trims surplus fields per type", () => {
    const messy: AnnotationDraft = {
      type: "site_comment",
      comment: "ok",
      draft_id: "abc",
      // these should be dropped — not relevant for site_comment
      item_index: 5,
      region_shape: "circle",
      region_x: 0.5,
      char_start: 0,
    };
    expect(trimAnnotationForType(toWireAnnotation(messy))).toEqual({
      type: "site_comment",
      comment: "ok",
    });
  });

  it("preserves item_index only on text_highlight scope=item", () => {
    const siteScoped = trimAnnotationForType({
      type: "text_highlight",
      comment: "c",
      text_scope: "site",
      char_start: 0,
      char_end: 3,
      text_snippet: "abc",
      item_index: 99, // should be dropped
    });
    expect(siteScoped.item_index).toBeUndefined();
    const itemScoped = trimAnnotationForType({
      type: "text_highlight",
      comment: "c",
      text_scope: "item",
      char_start: 0,
      char_end: 3,
      text_snippet: "abc",
      item_index: 2,
    });
    expect(itemScoped.item_index).toBe(2);
  });
});
