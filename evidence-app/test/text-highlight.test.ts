// pebble-4nwj — text-highlight offset math tests.
//
// We use happy-dom (via vitest's experimental DOM support) — no, we
// don't have that configured. Instead, build minimal DOM stubs and
// test the offset math directly. The textOffsetWithin function only
// needs Node.TEXT_NODE and basic tree walking, both of which we can
// fake.

import { describe, expect, it } from "vitest";
import { textOffsetWithin } from "../src/lib/text-highlight.js";

// Minimal stub of Node enough for textOffsetWithin to traverse.
type StubNode = {
  nodeType: 1 | 3;
  textContent: string;
  childNodes: StubNode[];
  contains?: (n: StubNode) => boolean;
};

function elem(children: StubNode[]): StubNode {
  const e: StubNode = {
    nodeType: 1,
    childNodes: children,
    get textContent() {
      return children.map((c) => c.textContent).join("");
    },
  } as unknown as StubNode;
  e.contains = (n: StubNode) => containsNode(e, n);
  return e;
}

function text(s: string): StubNode {
  return { nodeType: 3, textContent: s, childNodes: [] };
}

function containsNode(host: StubNode, target: StubNode): boolean {
  if (host === target) return true;
  for (const c of host.childNodes) {
    if (containsNode(c, target)) return true;
  }
  return false;
}

describe("textOffsetWithin", () => {
  // The function uses Node.TEXT_NODE which is 3. Verify our stub.
  it("respects Node.TEXT_NODE constant via stub", () => {
    expect(text("abc").nodeType).toBe(3);
  });

  it("returns offset for a text-node target", () => {
    const t = text("hello world");
    const root = elem([t]);
    const off = textOffsetWithin(
      root as unknown as HTMLElement,
      t as unknown as Node,
      6,
    );
    expect(off).toBe(6);
  });

  it("sums prior siblings before reaching the target text node", () => {
    const t1 = text("hello ");
    const t2 = text("world");
    const root = elem([t1, t2]);
    const off = textOffsetWithin(
      root as unknown as HTMLElement,
      t2 as unknown as Node,
      3,
    );
    expect(off).toBe(9);
  });

  it("walks into nested elements", () => {
    const inner = text("italic");
    const outer = text("plain ");
    const root = elem([outer, elem([inner])]);
    const off = textOffsetWithin(
      root as unknown as HTMLElement,
      inner as unknown as Node,
      6,
    );
    expect(off).toBe(12);
  });

  it("returns null if target is not in container", () => {
    const orphan = text("orphan");
    const root = elem([text("not orphan")]);
    expect(
      textOffsetWithin(
        root as unknown as HTMLElement,
        orphan as unknown as Node,
        0,
      ),
    ).toBeNull();
  });
});
