// Evidence v2: CDN-bundle render path.
//
// Spec: docs/specs/evidence-v2.md §3.1 (EV2-U-2). The Astro/Svelte
// gallery bundle lives at the customer-site Worker's CDN path
// `/assets/evidence/v{semver}/_astro/main.{js,css}` (R2 prefix
// `evidence-bundles/v{semver}/_astro/*` per the
// docs/2026-05-05-evidence-cdn-pipeline.md ADR). The CLI here:
//
//   - Embeds only the bundle version string (EvidenceBundleVersion
//     below); the bundle JS/CSS bytes are NOT in the CLI binary.
//   - Generates a per-publish `index.html` that:
//       1. Inlines the manifest as <script type="application/json"
//          id="evidence-manifest">…</script> for the bundle to read.
//       2. References the CDN-versioned bundle assets.
//       3. Renders a noscript fallback gallery so screen readers and
//          JavaScript-disabled environments still see the proof.
//
// The legacy in-package template (evidence_render.go) remains for
// backward compatibility but new publishes use this path.

package templates

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"path/filepath"
	"strings"
)

// EvidenceBundleVersion is defined in bundle_version_gen.go.
// It is updated automatically by CI whenever evidence-app/ changes land on main.

// evidenceV2Tmpl is the v2 minimal index.html template. Parsed once at
// init; parse failure is a programmer error.
var evidenceV2Tmpl = mustParseEvidenceV2()

func mustParseEvidenceV2() *template.Template {
	b, err := evidenceFS.ReadFile("assets/evidence.v2.html.tmpl")
	if err != nil {
		panic(fmt.Sprintf("templates: read embedded evidence.v2.html.tmpl: %v", err))
	}
	t, err := template.New("evidence.v2.html.tmpl").Parse(string(b))
	if err != nil {
		panic(fmt.Sprintf("templates: parse evidence.v2.html.tmpl: %v", err))
	}
	return t
}

// evidenceV2Model is the data passed to the v2 template. Distinct from
// the v1 evidenceRenderModel because the v2 surface is much smaller:
// most rendering happens client-side from the inlined manifest, and the
// noscript fallback shows the bare minimum (asset + caption).
type evidenceV2Model struct {
	Title            string
	Subtitle         string
	Summary          string
	BundleVersion    string
	GeneratorVersion string
	// ManifestJSON is the inlined manifest payload. Pre-marshaled so the
	// template can drop it into <script type="application/json"> as-is.
	// We use template.JS here NOT to bypass escaping (this isn't JS, it's
	// JSON) but to opt out of HTML-context escaping inside <script> tags
	// — the Go html/template engine treats <script> as JS context and
	// would mangle the JSON unless we tell it the bytes are already safe.
	// The JSON marshaler guarantees no `</script>` sequences; if a string
	// value contained one, Go's json.Marshal escapes the `<` as `<`.
	ManifestJSON template.JS
	Items        []evidenceV2Item
}

// evidenceV2Item is the noscript-only fallback shape. Image/video flag
// is pre-computed so the template stays a flat range. The interactive
// rendering reads from ManifestJSON; this slice is only for assistive
// tech / JS-disabled.
type evidenceV2Item struct {
	SafeName    string
	Alt         string
	Title       string
	Description string
	IsImage     bool
	IsVideo     bool
}

// EvidenceManifestPayload is the JSON shape inlined into <script
// type="application/json" id="evidence-manifest">. It mirrors
// EvidenceInput plus the render-time hints the bundle expects. The
// bundle's TypeScript counterpart is `EvidenceManifest` in
// `evidence-app/src/lib/manifest.ts` — keep both in sync.
type EvidenceManifestPayload struct {
	Title            string                        `json:"title"`
	Subtitle         string                        `json:"subtitle,omitempty"`
	Summary          string                        `json:"summary,omitempty"`
	Metadata         *EvidenceManifestMetadata     `json:"metadata,omitempty"`
	GeneratedAt      string                        `json:"generated_at,omitempty"`
	GeneratorVersion string                        `json:"generator_version,omitempty"`
	BundleVersion    string                        `json:"bundle_version,omitempty"`
	EnableReviews    bool                          `json:"enable_reviews,omitempty"`
	Items            []EvidenceManifestPayloadItem `json:"items"`
}

type EvidenceManifestMetadata struct {
	IssueURL   string `json:"issue_url,omitempty"`
	IssueID    string `json:"issue_id,omitempty"`
	IssueTitle string `json:"issue_title,omitempty"`
}

type EvidenceManifestPayloadItem struct {
	Src         string                    `json:"src"`
	Title       string                    `json:"title,omitempty"`
	Description string                    `json:"description,omitempty"`
	Alt         string                    `json:"alt,omitempty"`
	Sequence    *int                      `json:"sequence,omitempty"`
	Metadata    *EvidenceManifestMetadata `json:"metadata,omitempty"`
	Properties  map[string]any            `json:"properties,omitempty"`
	IsImage     bool                      `json:"is_image,omitempty"`
	IsVideo     bool                      `json:"is_video,omitempty"`
}

// buildManifestPayload converts a validated EvidenceInput plus the
// per-item safe names into the JSON payload the bundle reads at boot.
// The `src` in the payload is rewritten to the on-disk safe-name path
// (e.g. `assets/001-foo.png`) so the bundle's <img src> resolves
// against the per-site origin without needing path-rewriting at runtime.
func buildManifestPayload(in EvidenceInput, safeNames []string, g Generator, enableReviews bool) EvidenceManifestPayload {
	p := EvidenceManifestPayload{
		Title:            in.Title,
		Subtitle:         in.Subtitle,
		Summary:          in.Summary,
		GeneratedAt:      g.generatedAt(),
		GeneratorVersion: g.version(),
		BundleVersion:    EvidenceBundleVersion,
		EnableReviews:    enableReviews,
		Items:            make([]EvidenceManifestPayloadItem, 0, len(in.Items)),
	}
	if hasMetadata(in.Metadata) {
		p.Metadata = toManifestMetadata(in.Metadata)
	}
	for i, it := range in.Items {
		ext := strings.ToLower(filepath.Ext(it.Src))
		alt := it.Alt
		if alt == "" {
			alt = it.Title
		}
		safe := safeNames[i]
		item := EvidenceManifestPayloadItem{
			Src:         "assets/" + safe,
			Title:       it.Title,
			Description: it.Description,
			Alt:         alt,
			IsImage:     imageExts[ext],
			IsVideo:     videoExts[ext],
		}
		if it.Sequence != nil {
			s := *it.Sequence
			item.Sequence = &s
		}
		if hasMetadata(it.Metadata) {
			item.Metadata = toManifestMetadata(it.Metadata)
		}
		if len(it.Properties) > 0 {
			item.Properties = it.Properties
		}
		p.Items = append(p.Items, item)
	}
	return p
}

func hasMetadata(m EvidenceMetadata) bool {
	return m.IssueURL != "" || m.IssueID != "" || m.IssueTitle != ""
}

func toManifestMetadata(m EvidenceMetadata) *EvidenceManifestMetadata {
	return &EvidenceManifestMetadata{
		IssueURL:   m.IssueURL,
		IssueID:    m.IssueID,
		IssueTitle: m.IssueTitle,
	}
}

// MarshalEvidenceManifest produces the JSON payload that
// renderEvidenceHTMLV2 inlines. Exposed as a pure function so the
// integration-verification harness can assert against the wire shape
// without round-tripping through HTML rendering.
func MarshalEvidenceManifest(in EvidenceInput, safeNames []string, g Generator) ([]byte, error) {
	p := buildManifestPayload(in, safeNames, g, false)
	// Compact JSON, no indent — the v2 bundle is the canonical reader,
	// and human inspection is via the dashboard / API rather than View
	// Source. Keeping the payload compact matters for sites with many
	// items: a 100-item gallery's manifest can be ~50 KB.
	return json.Marshal(p)
}

// renderEvidenceHTMLV2 writes index.html only — no styles.css or
// evidence.js. The bundle is served from the CDN (see ADR
// docs/2026-05-05-evidence-cdn-pipeline.md). Asset files
// (assets/<safe-name>) are placed by the caller (RenderEvidence in
// evidence.go), same as v1.
//
// The dstNames argument is the per-item asset basename (already in
// post-sort order, matching the canonical SafeAssetName output). The
// caller passes it so we don't recompute SafeAssetName here.
func renderEvidenceHTMLV2(in EvidenceInput, outDir string, dstNames []string, g Generator, enableReviews bool) error {
	// Build the noscript fallback model. Items are already sorted via
	// the caller; we mirror evidence_render.go's image/video extension
	// branching so the noscript markup is consistent with what the
	// bundle would render for the same item.
	items := make([]evidenceV2Item, len(in.Items))
	for i, it := range in.Items {
		ext := strings.ToLower(filepath.Ext(it.Src))
		alt := it.Alt
		if alt == "" {
			alt = it.Title
		}
		items[i] = evidenceV2Item{
			SafeName:    dstNames[i],
			Alt:         alt,
			Title:       it.Title,
			Description: it.Description,
			IsImage:     imageExts[ext],
			IsVideo:     videoExts[ext],
		}
	}

	// Build the inlined manifest payload.
	manifestPayload := buildManifestPayload(in, dstNames, g, enableReviews)
	manifestBytes, err := json.Marshal(manifestPayload)
	if err != nil {
		return fmt.Errorf("templates: marshal evidence manifest: %w", err)
	}

	model := evidenceV2Model{
		Title:            in.Title,
		Subtitle:         in.Subtitle,
		Summary:          in.Summary,
		BundleVersion:    EvidenceBundleVersion,
		GeneratorVersion: g.version(),
		ManifestJSON:     template.JS(manifestBytes),
		Items:            items,
	}

	var html bytes.Buffer
	if err := evidenceV2Tmpl.Execute(&html, model); err != nil {
		return fmt.Errorf("templates: render evidence v2: %w", err)
	}
	if err := writeFile(outDir, "index.html", html.Bytes()); err != nil {
		return err
	}
	// v2 deliberately writes only index.html. styles.css and evidence.js
	// from v1 are NOT emitted; they live on the CDN now.
	return nil
}
