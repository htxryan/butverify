// pebble-4nwj — Text highlight capture helpers.
//
// The text_highlight annotation stores `char_start`/`char_end` as
// character offsets WITHIN A SPECIFIC ELEMENT'S text content. Browsers'
// Selection API gives us positions inside a Range against arbitrary
// nodes; we project those to a single offset by counting characters
// up to the selection in the host element's textContent.
//
// "Container" semantics — the spec's text_scope:
//   - 'site'  → offset within the site-summary text (manifest.summary)
//   - 'item'  → offset within a specific item's description text
//
// Both anchors are rendered as plain <p> nodes, so the offset math is
// straightforward: walk the text nodes inside the container in
// document order and sum their lengths up to the selection's start /
// end node.

// Inline well-known DOM nodeType constants so the file is testable in
// pure-node vitest (where the global `Node` is undefined). The
// numeric values are stable per W3C DOM Level 1.
const TEXT_NODE = 3;

export interface CapturedHighlight {
  char_start: number;
  char_end: number;
  text_snippet: string;
}

// Capture the current selection within `container`. Returns null if
// the selection is empty, collapsed, or doesn't lie inside the
// container.
export function captureSelection(container: HTMLElement): CapturedHighlight | null {
  if (typeof window === "undefined") return null;
  const sel = window.getSelection();
  if (!sel || sel.rangeCount === 0) return null;
  const range = sel.getRangeAt(0);
  if (range.collapsed) return null;

  // Both range endpoints must be inside the container (the user might
  // have dragged from outside a comment paragraph).
  if (
    !container.contains(range.startContainer) ||
    !container.contains(range.endContainer)
  ) {
    return null;
  }

  const start = textOffsetWithin(container, range.startContainer, range.startOffset);
  const end = textOffsetWithin(container, range.endContainer, range.endOffset);
  if (start === null || end === null || end <= start) return null;

  const snippet = (container.textContent ?? "").slice(start, end);
  if (snippet.length === 0) return null;

  return {
    char_start: start,
    char_end: end,
    text_snippet: snippet,
  };
}

// Walk the container's descendants in document order, counting
// character offsets in text nodes until we hit `targetNode`. Returns
// the absolute offset within the container's textContent. Internal
// whitespace text nodes are counted as-is (so the offset matches what
// `container.textContent.slice(...)` would yield).
export function textOffsetWithin(
  container: HTMLElement,
  targetNode: Node,
  targetOffset: number,
): number | null {
  if (!container.contains(targetNode) && targetNode !== container) return null;

  // If the target IS a text node we add its targetOffset directly.
  // If the target is an element node, we add the lengths of all text
  // nodes appearing before child #targetOffset in document order.
  let total = 0;
  let found = false;

  function walk(node: Node): void {
    if (found) return;
    if (node === targetNode) {
      if (node.nodeType === TEXT_NODE) {
        total += Math.min(targetOffset, node.textContent?.length ?? 0);
      } else {
        // Element / document node: count text in children up to
        // index `targetOffset`.
        const childNodes = node.childNodes;
        for (let i = 0; i < Math.min(targetOffset, childNodes.length); i++) {
          const child = childNodes[i];
          if (child) sumDescendantText(child);
        }
      }
      found = true;
      return;
    }
    if (node.nodeType === TEXT_NODE) {
      total += node.textContent?.length ?? 0;
      return;
    }
    for (const child of Array.from(node.childNodes)) {
      if (found) return;
      walk(child);
    }
  }

  function sumDescendantText(node: Node): void {
    if (node.nodeType === TEXT_NODE) {
      total += node.textContent?.length ?? 0;
      return;
    }
    for (const child of Array.from(node.childNodes)) {
      sumDescendantText(child);
    }
  }

  walk(container);
  return found ? total : null;
}
