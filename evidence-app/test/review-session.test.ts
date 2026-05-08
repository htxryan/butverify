// pebble-eaku — minimal tests for the per-item handler registry shape.
// The Svelte context plumbing in setReviewSession/getReviewSession is
// tested indirectly via component code; these tests just lock down the
// ItemHandlers contract that the toolbar relies on (set/get/clear).

import { describe, expect, it, vi } from "vitest";
import type {
  ItemHandlers,
  ReviewSession,
} from "../src/lib/review-session.js";

function makeSession(): ReviewSession {
  const registry = new Map<number, ItemHandlers>();
  return {
    enabled: true,
    siteId: "abc",
    drafts: () => [],
    add: () => {
      throw new Error("not used");
    },
    remove: () => {},
    registerItem: (idx, h) => {
      registry.set(idx, h);
    },
    unregisterItem: (idx) => {
      registry.delete(idx);
    },
    getItem: (idx) => registry.get(idx) ?? null,
  };
}

function makeHandlers(): ItemHandlers {
  return {
    openComment: vi.fn(),
    setDrawMode: vi.fn(),
    getDrawMode: vi.fn<() => "off" | "circle" | "rect">(() => "off"),
    captureHighlight: vi.fn(),
    getCanCaptureHighlight: vi.fn<() => boolean>(() => false),
  };
}

describe("review-session item registry", () => {
  it("registers and retrieves item handlers", () => {
    const session = makeSession();
    const h = makeHandlers();
    session.registerItem(2, h);
    expect(session.getItem(2)).toBe(h);
  });

  it("returns null for unregistered indices", () => {
    const session = makeSession();
    expect(session.getItem(99)).toBeNull();
  });

  it("unregisters cleanly so a stale handler is not returned", () => {
    const session = makeSession();
    const h = makeHandlers();
    session.registerItem(0, h);
    session.unregisterItem(0);
    expect(session.getItem(0)).toBeNull();
  });

  it("toolbar-style invocation flows through the registered handlers", () => {
    const session = makeSession();
    const h = makeHandlers();
    session.registerItem(1, h);

    const active = session.getItem(1);
    expect(active).not.toBeNull();
    active!.openComment();
    active!.setDrawMode("circle");
    expect(h.openComment).toHaveBeenCalledTimes(1);
    expect(h.setDrawMode).toHaveBeenCalledWith("circle");
  });

  it("re-registering the same index replaces the handlers", () => {
    const session = makeSession();
    const first = makeHandlers();
    const second = makeHandlers();
    session.registerItem(3, first);
    session.registerItem(3, second);
    expect(session.getItem(3)).toBe(second);
  });
});
