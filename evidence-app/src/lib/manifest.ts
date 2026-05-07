// Manifest = the JSON the Go CLI inlines into <script
// type="application/json" id="evidence-manifest"> on every published site.
// The bundle reads it at startup and renders the gallery. Schema mirrors
// Go's pkg/templates EvidenceInput plus a few render-time hints.
//
// Authoritative source: pkg/templates/evidence.go in the public repo.
// When that file's struct tags change, this type must change too — the
// bundle accepts unknown fields (forward compat) but typed access here
// is restricted to known fields so a typo'd field name fails fast in
// tests rather than silently rendering nothing.

export interface EvidenceManifest {
  title: string;
  subtitle?: string;
  summary?: string;
  metadata?: EvidenceMetadata;
  items: EvidenceItem[];
  // Render-time hints injected by the CLI (not part of EvidenceInput
  // proper). Optional on every field — older CLIs may omit them.
  generated_at?: string; // ISO 8601 UTC
  generator_version?: string;
  bundle_version?: string;
  enable_reviews?: boolean; // future: annotation UI gate (E3); off here
}

export interface EvidenceMetadata {
  issue_url?: string;
  issue_id?: string;
  issue_title?: string;
}

export interface EvidenceItem {
  src: string; // relative URL: assets/<NNN-name.ext>
  title?: string;
  description?: string;
  alt?: string;
  sequence?: number;
  metadata?: EvidenceMetadata;
  properties?: Record<string, string | number | Record<string, unknown>>;
  // Pre-computed by the CLI:
  is_image?: boolean;
  is_video?: boolean;
}

export class ManifestParseError extends Error {
  constructor(
    message: string,
    public readonly cause?: unknown,
  ) {
    super(message);
    this.name = "ManifestParseError";
  }
}

// readManifest extracts the JSON payload from the inlined <script> tag
// and validates the shape. The element id is the stable contract; missing
// or empty payload throws ManifestParseError so the boot path can render
// a "missing manifest" fallback instead of a blank page.
export function readManifest(doc: Document): EvidenceManifest {
  const node = doc.getElementById("evidence-manifest");
  if (!node) {
    throw new ManifestParseError(
      "evidence-manifest <script> element not found in document",
    );
  }
  const raw = node.textContent ?? "";
  if (raw.trim().length === 0) {
    throw new ManifestParseError("evidence-manifest payload is empty");
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch (err) {
    throw new ManifestParseError("evidence-manifest payload is not valid JSON", err);
  }
  return validateManifest(parsed);
}

// validateManifest guards the runtime shape. The Go CLI is the
// canonical producer; this guard catches manifest drift and bad
// hand-edits during dev. We only check required fields here — optional
// fields fall through with `as` casts because deep-validating user data
// against TS types is the wrong layer (the CLI already validated).
export function validateManifest(value: unknown): EvidenceManifest {
  if (typeof value !== "object" || value === null) {
    throw new ManifestParseError("manifest must be an object");
  }
  const obj = value as Record<string, unknown>;
  if (typeof obj.title !== "string" || obj.title.length === 0) {
    throw new ManifestParseError("manifest.title is required and must be a non-empty string");
  }
  if (!Array.isArray(obj.items)) {
    throw new ManifestParseError("manifest.items is required and must be an array");
  }
  if (obj.items.length === 0) {
    throw new ManifestParseError("manifest.items must contain at least one item");
  }
  for (let i = 0; i < obj.items.length; i++) {
    const it = obj.items[i];
    if (typeof it !== "object" || it === null) {
      throw new ManifestParseError(`manifest.items[${i}] must be an object`);
    }
    const itObj = it as Record<string, unknown>;
    if (typeof itObj.src !== "string" || itObj.src.length === 0) {
      throw new ManifestParseError(`manifest.items[${i}].src is required`);
    }
  }
  return obj as unknown as EvidenceManifest;
}

// isImageItem / isVideoItem — convenience predicates. Prefer the manifest
// hint (Go CLI sets these) but fall back to extension sniffing for older
// CLIs that don't emit the hints.
const IMAGE_EXT = new Set([".png", ".jpg", ".jpeg", ".webp", ".gif", ".avif"]);
const VIDEO_EXT = new Set([".mp4", ".webm", ".mov"]);

export function isImageItem(item: EvidenceItem): boolean {
  if (typeof item.is_image === "boolean") return item.is_image;
  return IMAGE_EXT.has(extname(item.src));
}

export function isVideoItem(item: EvidenceItem): boolean {
  if (typeof item.is_video === "boolean") return item.is_video;
  return VIDEO_EXT.has(extname(item.src));
}

function extname(p: string): string {
  // Strip query string and fragment before sniffing extension so that
  // URLs like "https://placehold.co/img.png?w=800" resolve to ".png".
  const noQuery = (p.split("?")[0] ?? p).split("#")[0] ?? p;
  const dot = noQuery.lastIndexOf(".");
  if (dot < 0) return "";
  return noQuery.slice(dot).toLowerCase();
}

// Honor `prefers-reduced-motion: reduce` from JS-driven animation
// branches. Components that animate via Web Animations API or
// requestAnimationFrame should call this and skip transitions.
//
// Per docs/2026-04-30-reduced-motion-policy.md the CSS branch is
// already handled by base.css; this helper is for JS-only animations
// (carousel scroll-snap, lightbox open/close).
export function prefersReducedMotion(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return false;
  }
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}
