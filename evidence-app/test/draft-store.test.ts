// pebble-4nwj — draft-store tests. We use an in-memory storage shim so
// the tests stay deterministic and don't depend on jsdom.

import { describe, expect, it } from "vitest";
import { createDraftStore, storageKeyFor, type DraftStorage } from "../src/lib/draft-store.js";

function makeStorage(): DraftStorage & { _data: Map<string, string> } {
  const data = new Map<string, string>();
  return {
    _data: data,
    getItem: (k) => data.get(k) ?? null,
    setItem: (k, v) => {
      data.set(k, v);
    },
    removeItem: (k) => {
      data.delete(k);
    },
  };
}

describe("draft-store", () => {
  it("namespaces storage by site_id", () => {
    expect(storageKeyFor("abc123")).toBe("bv-draft-v1:abc123");
    expect(storageKeyFor("xyz789")).toBe("bv-draft-v1:xyz789");
  });

  it("starts empty when storage is empty", () => {
    const s = makeStorage();
    expect(createDraftStore("a", s).load()).toEqual([]);
  });

  it("round-trips a draft through storage", () => {
    const s = makeStorage();
    const store = createDraftStore("a", s);
    const d = store.add({ type: "site_comment", comment: "looks ok" });
    expect(d.draft_id).toBeTruthy();
    expect(d.type).toBe("site_comment");
    // A fresh store reads the same drafts back from storage.
    const reloaded = createDraftStore("a", s).load();
    expect(reloaded).toHaveLength(1);
    expect(reloaded[0]?.comment).toBe("looks ok");
  });

  it("removes by draft_id", () => {
    const s = makeStorage();
    const store = createDraftStore("a", s);
    const d1 = store.add({ type: "site_comment", comment: "1" });
    const d2 = store.add({ type: "site_comment", comment: "2" });
    expect(store.load()).toHaveLength(2);
    store.remove(d1.draft_id);
    const remaining = store.load();
    expect(remaining).toHaveLength(1);
    expect(remaining[0]?.draft_id).toBe(d2.draft_id);
  });

  it("clear() empties the store", () => {
    const s = makeStorage();
    const store = createDraftStore("a", s);
    store.add({ type: "site_comment", comment: "1" });
    store.clear();
    expect(store.load()).toEqual([]);
  });

  it("does not bleed across site_ids", () => {
    const s = makeStorage();
    const a = createDraftStore("a", s);
    const b = createDraftStore("b", s);
    a.add({ type: "site_comment", comment: "site a" });
    expect(a.load()).toHaveLength(1);
    expect(b.load()).toHaveLength(0);
  });

  it("ignores payloads with the wrong version", () => {
    const s = makeStorage();
    s.setItem(
      storageKeyFor("a"),
      JSON.stringify({
        version: "v0",
        site_id: "a",
        saved_at: "2026-05-06T00:00:00Z",
        drafts: [{ type: "site_comment", comment: "old", draft_id: "x" }],
      }),
    );
    expect(createDraftStore("a", s).load()).toEqual([]);
  });

  it("ignores tampered drafts (wrong type)", () => {
    const s = makeStorage();
    s.setItem(
      storageKeyFor("a"),
      JSON.stringify({
        version: "v1",
        site_id: "a",
        saved_at: "2026-05-06T00:00:00Z",
        drafts: [
          { type: "bogus", comment: "x", draft_id: "1" },
          { type: "site_comment", comment: "ok", draft_id: "2" },
        ],
      }),
    );
    const drafts = createDraftStore("a", s).load();
    expect(drafts).toHaveLength(1);
    expect(drafts[0]?.draft_id).toBe("2");
  });

  it("falls back to memory when storage is null", () => {
    const store = createDraftStore("a", null);
    store.add({ type: "site_comment", comment: "ok" });
    expect(store.load()).toHaveLength(1);
  });

  it("preserves region + text_highlight fields", () => {
    const s = makeStorage();
    const store = createDraftStore("a", s);
    store.add({
      type: "image_region",
      comment: "off-center",
      item_index: 1,
      region_shape: "circle",
      region_x: 0.5,
      region_y: 0.5,
      region_width: 0.1,
      region_height: 0.1,
    });
    store.add({
      type: "text_highlight",
      comment: "misleading",
      text_scope: "item",
      item_index: 0,
      char_start: 4,
      char_end: 12,
      text_snippet: "abcdefgh",
    });
    const reloaded = createDraftStore("a", s).load();
    expect(reloaded).toHaveLength(2);
    expect(reloaded[0]?.region_shape).toBe("circle");
    expect(reloaded[1]?.text_snippet).toBe("abcdefgh");
  });
});
