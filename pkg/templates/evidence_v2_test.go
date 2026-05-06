// Tests for the v2 (CDN-bundle) evidence render path.
// Spec: docs/specs/evidence-v2.md §3.1 (EV2-U-2).
// ADR: docs/2026-05-05-evidence-cdn-pipeline.md.

package templates

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helper: build a minimal valid input + matching safe-name slice.
func v2Fixture(t *testing.T) (EvidenceInput, []string) {
	t.Helper()
	in := EvidenceInput{
		Title:    "Hello",
		Subtitle: "Subtitle",
		Summary:  "Short summary",
		Metadata: EvidenceMetadata{IssueID: "ENG-1"},
		Items: []EvidenceItem{
			{Src: "screenshot.png", Title: "Item 1", Description: "first", Alt: "alt 1"},
			{Src: "demo.mp4", Title: "Item 2", Description: "second"},
		},
	}
	if err := in.Validate(); err != nil {
		t.Fatalf("fixture invalid: %v", err)
	}
	dstNames := []string{
		SafeAssetName(in.Items[0], 0),
		SafeAssetName(in.Items[1], 1),
	}
	return in, dstNames
}

func TestEvidenceBundleVersion_NotEmpty(t *testing.T) {
	if EvidenceBundleVersion == "" {
		t.Fatal("EvidenceBundleVersion must not be empty")
	}
	// Must look like a semver-ish string (no leading 'v', dots present).
	if strings.HasPrefix(EvidenceBundleVersion, "v") {
		t.Errorf("EvidenceBundleVersion must not have leading 'v' (got %q); the template adds the prefix", EvidenceBundleVersion)
	}
	if !strings.Contains(EvidenceBundleVersion, ".") {
		t.Errorf("EvidenceBundleVersion %q does not look like a semver", EvidenceBundleVersion)
	}
}

func TestRenderEvidenceHTMLV2_WritesIndexHtmlOnly(t *testing.T) {
	in, dstNames := v2Fixture(t)
	dir := t.TempDir()

	if err := renderEvidenceHTMLV2(in, dir, dstNames, Generator{Version: "0.1", Now: time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("renderEvidenceHTMLV2: %v", err)
	}

	// Only index.html should be written. NO styles.css, NO evidence.js
	// (those live on the CDN now).
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read outDir: %v", err)
	}
	gotFiles := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		gotFiles = append(gotFiles, e.Name())
	}
	if len(gotFiles) != 1 || gotFiles[0] != "index.html" {
		t.Errorf("v2 must write only index.html; got %v", gotFiles)
	}
}

func TestRenderEvidenceHTMLV2_TemplateContent(t *testing.T) {
	in, dstNames := v2Fixture(t)
	dir := t.TempDir()
	if err := renderEvidenceHTMLV2(in, dir, dstNames, Generator{Version: "0.1", Now: time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("renderEvidenceHTMLV2: %v", err)
	}
	bytes, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(bytes)

	// Must reference the CDN-versioned bundle assets.
	wantRefs := []string{
		`<link rel="stylesheet" href="/assets/evidence/v` + EvidenceBundleVersion + `/_astro/main.css">`,
		`<script type="module" src="/assets/evidence/v` + EvidenceBundleVersion + `/_astro/main.js"></script>`,
	}
	for _, want := range wantRefs {
		if !strings.Contains(html, want) {
			t.Errorf("index.html missing reference %q", want)
		}
	}

	// Must include the manifest mount point.
	if !strings.Contains(html, `<div id="bv-gallery-root"></div>`) {
		t.Error("index.html missing #bv-gallery-root mount point")
	}

	// Must inline a valid manifest JSON in the script tag.
	if !strings.Contains(html, `<script type="application/json" id="evidence-manifest">`) {
		t.Error("index.html missing <script id=\"evidence-manifest\">")
	}

	// Title appears in <title> AND noscript fallback.
	titleCount := strings.Count(html, "Hello")
	if titleCount < 2 {
		t.Errorf("title 'Hello' should appear in <title> and noscript fallback; got %d occurrences", titleCount)
	}

	// Must NOT reference the legacy bundle paths.
	for _, bad := range []string{`href="styles.css"`, `src="evidence.js"`} {
		if strings.Contains(html, bad) {
			t.Errorf("v2 must not reference legacy asset %q; rendered HTML contains it", bad)
		}
	}
}

func TestRenderEvidenceHTMLV2_NoscriptFallback(t *testing.T) {
	in, dstNames := v2Fixture(t)
	dir := t.TempDir()
	if err := renderEvidenceHTMLV2(in, dir, dstNames, Generator{}); err != nil {
		t.Fatalf("renderEvidenceHTMLV2: %v", err)
	}
	bytes, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(bytes)

	// Each item should appear in the noscript fallback with its safe-name.
	for i, name := range dstNames {
		want := `assets/` + name
		if !strings.Contains(html, want) {
			t.Errorf("item[%d] safe-name %q missing from noscript fallback", i, want)
		}
	}
}

func TestMarshalEvidenceManifest_ShapeMatchesBundle(t *testing.T) {
	in, dstNames := v2Fixture(t)
	g := Generator{Version: "0.1", Now: time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)}
	raw, err := MarshalEvidenceManifest(in, dstNames, g)
	if err != nil {
		t.Fatalf("MarshalEvidenceManifest: %v", err)
	}

	// Round-trip parse — the bundle's TS code does the same with
	// JSON.parse, so a JSON-invalid payload would be a hard failure for
	// every published site.
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("manifest must be valid JSON: %v", err)
	}

	// Required fields present and well-typed.
	if got["title"] != "Hello" {
		t.Errorf("title: got %v", got["title"])
	}
	if got["bundle_version"] != EvidenceBundleVersion {
		t.Errorf("bundle_version: got %v", got["bundle_version"])
	}
	items, ok := got["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("items must be an array of 2; got %T %v", got["items"], got["items"])
	}
	first := items[0].(map[string]any)
	if first["src"] != "assets/"+dstNames[0] {
		t.Errorf("items[0].src: got %v, want %v", first["src"], "assets/"+dstNames[0])
	}
	if first["is_image"] != true {
		t.Errorf("items[0].is_image must be true for .png; got %v", first["is_image"])
	}
	second := items[1].(map[string]any)
	if second["is_video"] != true {
		t.Errorf("items[1].is_video must be true for .mp4; got %v", second["is_video"])
	}
}

func TestMarshalEvidenceManifest_NoEnableReviewsAtV1(t *testing.T) {
	in, dstNames := v2Fixture(t)
	raw, err := MarshalEvidenceManifest(in, dstNames, Generator{})
	if err != nil {
		t.Fatalf("MarshalEvidenceManifest: %v", err)
	}
	// enable_reviews is omitempty + zero-value, so it should NOT appear
	// in the v1 bundle's wire output. E3 will set this.
	if strings.Contains(string(raw), `"enable_reviews"`) {
		t.Errorf("enable_reviews should be omitted in v1; payload: %s", raw)
	}
}

func TestMarshalEvidenceManifest_PreservesPropertiesAndMetadata(t *testing.T) {
	seq := 5
	in := EvidenceInput{
		Title: "x",
		Items: []EvidenceItem{
			{
				Src:      "a.png",
				Title:    "T",
				Sequence: &seq,
				Metadata: EvidenceMetadata{IssueURL: "https://example.com/issue/1", IssueID: "X-1"},
				Properties: map[string]any{
					"viewport": "1280x720",
					"frame":    map[string]any{"step": "ok"},
				},
			},
		},
	}
	if err := in.Validate(); err != nil {
		t.Fatalf("fixture invalid: %v", err)
	}
	dstNames := []string{SafeAssetName(in.Items[0], 0)}
	raw, err := MarshalEvidenceManifest(in, dstNames, Generator{})
	if err != nil {
		t.Fatalf("MarshalEvidenceManifest: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	items := got["items"].([]any)
	first := items[0].(map[string]any)
	if first["sequence"].(float64) != 5 {
		t.Errorf("sequence not preserved: %v", first["sequence"])
	}
	meta := first["metadata"].(map[string]any)
	if meta["issue_id"] != "X-1" {
		t.Errorf("metadata.issue_id not preserved: %v", meta["issue_id"])
	}
	props := first["properties"].(map[string]any)
	if props["viewport"] != "1280x720" {
		t.Errorf("property not preserved: %v", props)
	}
	frame := props["frame"].(map[string]any)
	if frame["step"] != "ok" {
		t.Errorf("nested property not preserved: %v", frame)
	}
}

func TestRenderEvidence_V2_ViaPublicAPI(t *testing.T) {
	// Integration check: RenderEvidence with UseBundleV2=true must
	// produce only index.html + the assets/ subdir; no styles.css or
	// evidence.js at the top level.
	dir := t.TempDir()
	containment := filepath.Join(dir, "src")
	if err := os.MkdirAll(containment, 0o755); err != nil {
		t.Fatalf("mkdir containment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(containment, "screenshot.png"), pngFixture(), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	manifest := []byte(`{
		"title": "v2 integration",
		"items": [{"src": "screenshot.png", "title": "shot"}]
	}`)
	outDir := filepath.Join(dir, "out")
	_, _, err := RenderEvidence(manifest, RenderOptions{
		OutDir:          outDir,
		ContainmentRoot: containment,
		UseBundleV2:     true,
	}, Generator{Version: "test"})
	if err != nil {
		t.Fatalf("RenderEvidence v2: %v", err)
	}

	// outDir contents: index.html + assets/
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read outDir: %v", err)
	}
	got := make(map[string]bool, len(entries))
	for _, e := range entries {
		got[e.Name()] = e.IsDir()
	}
	if isDir, ok := got["index.html"]; !ok || isDir {
		t.Errorf("expected index.html file; got entries: %v", got)
	}
	if isDir, ok := got["assets"]; !ok || !isDir {
		t.Errorf("expected assets/ dir; got entries: %v", got)
	}
	if _, has := got["styles.css"]; has {
		t.Errorf("v2 must NOT emit styles.css")
	}
	if _, has := got["evidence.js"]; has {
		t.Errorf("v2 must NOT emit evidence.js")
	}
}

// pngFixture returns the smallest valid PNG (1x1 transparent pixel) so the
// MIME-sniff gate in copyAsset accepts it.
func pngFixture() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x62, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
		0x42, 0x60, 0x82,
	}
}
