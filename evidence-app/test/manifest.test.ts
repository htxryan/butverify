// Manifest reader/validator tests. The Go CLI is the canonical producer,
// but the bundle's runtime guard catches drift and bad hand-edits.

import { describe, expect, it } from "vitest";
import {
  isImageItem,
  isVideoItem,
  ManifestParseError,
  prefersReducedMotion,
  readManifest,
  validateManifest,
} from "../src/lib/manifest.js";

function makeDoc(scriptText: string | null): Document {
  // Simple stub Document that supports getElementById for the manifest
  // script tag. Avoids pulling in jsdom — tests are pure logic.
  return {
    getElementById(id: string) {
      if (id === "evidence-manifest") {
        if (scriptText === null) return null;
        return { textContent: scriptText } as unknown as HTMLElement;
      }
      return null;
    },
  } as unknown as Document;
}

describe("validateManifest", () => {
  it("accepts a minimal valid manifest", () => {
    const m = validateManifest({
      title: "Hello",
      items: [{ src: "assets/001-foo.png" }],
    });
    expect(m.title).toBe("Hello");
    expect(m.items).toHaveLength(1);
  });

  it("rejects non-objects", () => {
    expect(() => validateManifest("hi")).toThrow(ManifestParseError);
    expect(() => validateManifest(null)).toThrow(ManifestParseError);
    expect(() => validateManifest(["not", "an", "object"])).toThrow(ManifestParseError);
  });

  it("rejects missing or empty title", () => {
    expect(() => validateManifest({ items: [{ src: "x.png" }] })).toThrow(
      /title is required/,
    );
    expect(() => validateManifest({ title: "", items: [{ src: "x.png" }] })).toThrow(
      /title is required/,
    );
  });

  it("rejects missing or empty items array", () => {
    expect(() => validateManifest({ title: "x" })).toThrow(/items is required/);
    expect(() => validateManifest({ title: "x", items: [] })).toThrow(
      /at least one item/,
    );
  });

  it("rejects items with missing src", () => {
    expect(() => validateManifest({ title: "x", items: [{ title: "no src" }] })).toThrow(
      /items\[0\]\.src is required/,
    );
  });

  it("preserves optional fields", () => {
    const m = validateManifest({
      title: "x",
      subtitle: "sub",
      summary: "yo",
      metadata: { issue_id: "JIRA-1" },
      generated_at: "2026-05-05T10:00:00Z",
      enable_reviews: false,
      items: [{ src: "x.png", title: "t", description: "d", alt: "a", sequence: 1 }],
    });
    expect(m.subtitle).toBe("sub");
    expect(m.metadata?.issue_id).toBe("JIRA-1");
    expect(m.items[0]?.title).toBe("t");
  });
});

describe("readManifest", () => {
  it("parses a JSON-valid script element", () => {
    const json = JSON.stringify({ title: "x", items: [{ src: "a.png" }] });
    const m = readManifest(makeDoc(json));
    expect(m.title).toBe("x");
  });

  it("throws when the element is missing", () => {
    expect(() => readManifest(makeDoc(null))).toThrow(/element not found/);
  });

  it("throws when the payload is empty", () => {
    expect(() => readManifest(makeDoc(""))).toThrow(/payload is empty/);
    expect(() => readManifest(makeDoc("   "))).toThrow(/payload is empty/);
  });

  it("throws ManifestParseError on invalid JSON", () => {
    expect(() => readManifest(makeDoc("{not json"))).toThrow(ManifestParseError);
  });
});

describe("isImageItem / isVideoItem", () => {
  it("trusts the manifest hint when present", () => {
    expect(isImageItem({ src: "no-ext", is_image: true })).toBe(true);
    expect(isVideoItem({ src: "no-ext", is_video: true })).toBe(true);
    expect(isImageItem({ src: "x.png", is_image: false })).toBe(false);
  });

  it("falls back to extension sniffing", () => {
    expect(isImageItem({ src: "assets/001-foo.PNG" })).toBe(true);
    expect(isImageItem({ src: "assets/002-foo.jpg" })).toBe(true);
    expect(isImageItem({ src: "assets/003-foo.webp" })).toBe(true);
    expect(isImageItem({ src: "assets/004-foo.gif" })).toBe(true);
    expect(isVideoItem({ src: "assets/005-clip.mp4" })).toBe(true);
    expect(isVideoItem({ src: "assets/006-clip.webm" })).toBe(true);
    expect(isVideoItem({ src: "assets/007-clip.MOV" })).toBe(true);
    expect(isImageItem({ src: "assets/008-clip.mp4" })).toBe(false);
    expect(isVideoItem({ src: "assets/008-foo.png" })).toBe(false);
  });
});

describe("prefersReducedMotion", () => {
  it("returns false in non-browser environments", () => {
    // The vitest default env is node; window is undefined.
    expect(prefersReducedMotion()).toBe(false);
  });
});
