// Regression tests for the gallery topbar full-bleed + flush-edges fix.
// The bug: on a reviews-enabled site at >= 1024px the topbar's right edge
// sat ~1.5rem inside the viewport (because .bv-shell--with-review carried
// padding-right: var(--bv-space-5)) and on every site the title and right
// controls were inset ~12px from the header's edges (because
// .bv-topbar-inner carried padding: 0 var(--bv-space-3)). The fix: zero
// the inner padding and add margin-right: calc(-1 * var(--bv-space-5))
// to the desktop topbar rule so the topbar breaks out of the shell's
// right padding and spans the full viewport.
//
// We assert against the file source rather than a rendered DOM because
// jsdom does not lay out sticky/negative-margin positioning during paint.

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const here = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(here, "..");

function read(rel: string): string {
  return readFileSync(path.join(repoRoot, rel), "utf-8");
}

describe("Gallery topbar full-bleed + flush edges", () => {
  it("defines --bv-topbar-height: 3.25rem in tokens.css (defensive)", () => {
    const tokens = read("src/styles/tokens.css");
    expect(tokens).toMatch(/--bv-topbar-height:\s*3\.25rem\s*;/);
  });

  it(".bv-topbar still spans 100% width (defensive)", () => {
    const header = read("src/components/GalleryHeader.svelte");
    expect(header).toMatch(/\.bv-topbar\s*{[^}]*width:\s*100%/);
  });

  it(".bv-topbar-inner has zero horizontal padding so title/controls sit flush to the header edges", () => {
    const header = read("src/components/GalleryHeader.svelte");
    const innerMatch = header.match(/\.bv-topbar-inner\s*{[^}]*}/);
    expect(innerMatch, "expected to find .bv-topbar-inner rule").toBeTruthy();
    expect(innerMatch![0]).toMatch(/padding:\s*0\s*;/);
    // Must not still carry the old horizontal padding that caused the inset.
    expect(innerMatch![0]).not.toMatch(/padding:\s*0\s+var\(--bv-space-3\)/);
  });

  it("desktop topbar rule breaks the topbar out of the shell's right padding via negative margin", () => {
    const gallery = read("src/components/Gallery.svelte");
    // Capture the .bv-shell--with-review > :global(.bv-topbar) rule body.
    const topbarRuleMatch = gallery.match(
      /\.bv-shell--with-review\s*>\s*:global\(\.bv-topbar\)\s*{([^}]*)}/,
    );
    expect(
      topbarRuleMatch,
      "expected a .bv-shell--with-review > :global(.bv-topbar) rule",
    ).toBeTruthy();
    const body = topbarRuleMatch![1];
    expect(body).toMatch(/grid-column:\s*1\s*\/\s*-1/);
    expect(body).toMatch(/margin-right:\s*calc\(\s*-1\s*\*\s*var\(--bv-space-5\)\s*\)/);

    // And the surrounding @media (min-width: 1024px) block must exist.
    expect(gallery).toMatch(/@media\s*\(min-width:\s*1024px\)/);
  });
});
