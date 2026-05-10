// Regression tests for the desktop ReviewPanel sticky-positioning rule
// vs. the gallery topbar. The bug: the desktop panel stuck at
// top: 1.5rem with z-index: 30 (inherited) while the topbar stuck at
// top: 0 with z-index: 20, so the panel painted over the topbar between
// y=24px and y=52px while scrolling. The fix anchors the panel below the
// topbar via --bv-topbar-height and lowers its z-index below the topbar.
//
// We assert against the file source rather than a rendered DOM because
// jsdom does not lay out sticky positioning during paint, so a regex
// check on the CSS rule verifies the same invariant at zero dependency
// cost.

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const here = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(here, "..");

function read(rel: string): string {
  return readFileSync(path.join(repoRoot, rel), "utf-8");
}

describe("ReviewPanel desktop sticky vs. topbar", () => {
  it("defines --bv-topbar-height: 3.25rem in tokens.css", () => {
    const tokens = read("src/styles/tokens.css");
    expect(tokens).toMatch(/--bv-topbar-height:\s*3\.25rem\s*;/);
  });

  it("topbar inner consumes --bv-topbar-height", () => {
    const header = read("src/components/GalleryHeader.svelte");
    expect(header).toMatch(/\.bv-topbar-inner\s*{[^}]*height:\s*var\(--bv-topbar-height\)/);
  });

  it("desktop @media block anchors top below the topbar", () => {
    const panel = read("src/components/ReviewPanel.svelte");
    const desktopMatch = panel.match(/@media\s*\(min-width:\s*1024px\)\s*{[\s\S]*?}\s*}/);
    expect(desktopMatch, "expected a desktop @media (min-width: 1024px) block").toBeTruthy();
    const desktop = desktopMatch![0];
    expect(desktop).toMatch(/top:\s*calc\(\s*var\(--bv-topbar-height\)/);
  });

  it("desktop @media block lowers z-index below the topbar (which is 20)", () => {
    const panel = read("src/components/ReviewPanel.svelte");
    const desktopMatch = panel.match(/@media\s*\(min-width:\s*1024px\)\s*{[\s\S]*?}\s*}/);
    expect(desktopMatch).toBeTruthy();
    const desktop = desktopMatch![0];
    const zMatch = desktop.match(/z-index:\s*(\d+)/);
    expect(zMatch, "expected an explicit z-index on the desktop rule").toBeTruthy();
    const z = Number(zMatch![1]);
    expect(z).toBeLessThan(20);
  });
});
