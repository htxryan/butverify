package templates

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fixedEvidenceInput is the canonical input used by render tests. It
// covers: subtitle, summary, image item with alt fallback, image item
// with explicit alt, and a video item — exercising every code path in
// the template in a single render.
func fixedEvidenceInput() EvidenceInput {
	seq1 := 1
	seq2 := 2
	seq3 := 3
	return EvidenceInput{
		Title:    "Login redesign",
		Subtitle: "Ticket DELIVERY-1234",
		Summary:  "Updated the login form.\nAll states pass automated tests.",
		Items: []EvidenceItem{
			{
				Src:         "./screenshots/01-empty.png",
				Title:       "Empty state",
				Description: "Page loads with no validation errors.",
				Sequence:    &seq1,
				// Alt unset → falls back to Title.
			},
			{
				Src:         "./screenshots/02-error.png",
				Title:       "Inline validation",
				Description: "Empty-email submit shows the helper inline.",
				Sequence:    &seq2,
				Alt:         "Form with red error text under the email field",
			},
			{
				Src:         "./videos/03-success.webm",
				Title:       "Successful sign-in",
				Description: "End-to-end sign-in flow.",
				Sequence:    &seq3,
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Stacked layout golden snapshot test (EV-U-7 determinism + EV-U-10
// data-layout + EV-U-9 lazy/decoding/controls + EV-U-2 no script).
// ---------------------------------------------------------------------------

func TestRenderEvidenceHTML_StackedSnapshot(t *testing.T) {
	in := fixedEvidenceInput()
	g := Generator{Version: "1.2.3"}

	dir1 := t.TempDir()
	if err := renderEvidenceHTML(in, "stacked", dir1, g); err != nil {
		t.Fatalf("render: %v", err)
	}
	dir2 := t.TempDir()
	if err := renderEvidenceHTML(in, "stacked", dir2, g); err != nil {
		t.Fatalf("render: %v", err)
	}

	html1, err := os.ReadFile(filepath.Join(dir1, "index.html"))
	if err != nil {
		t.Fatalf("read index1: %v", err)
	}
	html2, err := os.ReadFile(filepath.Join(dir2, "index.html"))
	if err != nil {
		t.Fatalf("read index2: %v", err)
	}
	if string(html1) != string(html2) {
		t.Errorf("non-deterministic stacked render — bundles differ across runs")
	}

	css1, err := os.ReadFile(filepath.Join(dir1, "styles.css"))
	if err != nil {
		t.Fatalf("read styles1: %v", err)
	}
	css2, err := os.ReadFile(filepath.Join(dir2, "styles.css"))
	if err != nil {
		t.Fatalf("read styles2: %v", err)
	}
	if string(css1) != string(css2) {
		t.Errorf("non-deterministic styles.css — bundles differ across runs")
	}

	s := string(html1)

	// EV-U-10: exact body data-layout match.
	if !strings.Contains(s, `<body data-layout="stacked">`) {
		t.Errorf("missing/wrong <body data-layout=\"stacked\">; got:\n%s", firstFewLines(s, 12))
	}

	// EV-U-2: no <script> ANYWHERE.
	if strings.Contains(s, "<script") {
		t.Errorf("rendered HTML contains <script> tag (EV-U-2 violation)")
	}

	// EV-U-9: every <img> has loading=lazy + decoding=async + non-empty alt.
	imgRe := regexp.MustCompile(`<img\s+([^>]+)>`)
	imgs := imgRe.FindAllStringSubmatch(s, -1)
	if len(imgs) != 2 {
		t.Errorf("expected 2 <img> tags, got %d (input has 2 images)", len(imgs))
	}
	for _, m := range imgs {
		attrs := m[1]
		if !strings.Contains(attrs, `loading="lazy"`) {
			t.Errorf("img missing loading=\"lazy\": %q", attrs)
		}
		if !strings.Contains(attrs, `decoding="async"`) {
			t.Errorf("img missing decoding=\"async\": %q", attrs)
		}
		altRe := regexp.MustCompile(`alt="([^"]+)"`)
		am := altRe.FindStringSubmatch(attrs)
		if am == nil || strings.TrimSpace(am[1]) == "" {
			t.Errorf("img missing non-empty alt: %q", attrs)
		}
	}

	// EV-U-9: every <video> has preload=metadata + controls.
	videoRe := regexp.MustCompile(`<video\s+([^>]*)>`)
	videos := videoRe.FindAllStringSubmatch(s, -1)
	if len(videos) != 1 {
		t.Errorf("expected 1 <video> tag, got %d", len(videos))
	}
	for _, m := range videos {
		attrs := m[1]
		if !strings.Contains(attrs, `preload="metadata"`) {
			t.Errorf("video missing preload=\"metadata\": %q", attrs)
		}
		if !regexp.MustCompile(`\bcontrols\b`).MatchString(attrs) {
			t.Errorf("video missing controls: %q", attrs)
		}
	}

	// Alt fallback: first item has no Alt; should fall back to Title.
	if !strings.Contains(s, `alt="Empty state"`) {
		t.Errorf("expected alt fallback to title (\"Empty state\"); not found in HTML")
	}
	// Explicit alt on second item is preserved.
	if !strings.Contains(s, `alt="Form with red error text under the email field"`) {
		t.Errorf("expected explicit alt to be preserved; not found in HTML")
	}

	// Generator metadata is rendered (Version) but no timestamp (EV-U-7).
	if !strings.Contains(s, `bv 1.2.3`) {
		t.Errorf("expected generator version \"bv 1.2.3\" in output")
	}
}

// ---------------------------------------------------------------------------
// Carousel layout golden snapshot test.
// ---------------------------------------------------------------------------

func TestRenderEvidenceHTML_CarouselSnapshot(t *testing.T) {
	in := fixedEvidenceInput()
	g := Generator{Version: "1.2.3"}

	dir1 := t.TempDir()
	if err := renderEvidenceHTML(in, "carousel", dir1, g); err != nil {
		t.Fatalf("render: %v", err)
	}
	dir2 := t.TempDir()
	if err := renderEvidenceHTML(in, "carousel", dir2, g); err != nil {
		t.Fatalf("render: %v", err)
	}

	html1, err := os.ReadFile(filepath.Join(dir1, "index.html"))
	if err != nil {
		t.Fatalf("read index1: %v", err)
	}
	html2, err := os.ReadFile(filepath.Join(dir2, "index.html"))
	if err != nil {
		t.Fatalf("read index2: %v", err)
	}
	if string(html1) != string(html2) {
		t.Errorf("non-deterministic carousel render")
	}

	s := string(html1)

	// EV-U-10: carousel data-layout exact match.
	if !strings.Contains(s, `<body data-layout="carousel">`) {
		t.Errorf("missing/wrong <body data-layout=\"carousel\">; got:\n%s", firstFewLines(s, 12))
	}

	// EV-U-2: no <script> ANYWHERE.
	if strings.Contains(s, "<script") {
		t.Errorf("carousel HTML contains <script> tag (EV-U-2 violation)")
	}

	// Carousel structure: track + items + pager.
	if !strings.Contains(s, `class="carousel-track"`) {
		t.Errorf("missing .carousel-track")
	}
	if !strings.Contains(s, `class="carousel-item"`) {
		t.Errorf("missing .carousel-item")
	}
	if !strings.Contains(s, `id="item-1"`) {
		t.Errorf("missing id=\"item-1\" on first carousel item")
	}
	if !strings.Contains(s, `id="item-3"`) {
		t.Errorf("missing id=\"item-3\" on third carousel item")
	}
	if !strings.Contains(s, `href="#item-1"`) || !strings.Contains(s, `href="#item-3"`) {
		t.Errorf("missing pager anchor links")
	}

	// Same EV-U-9 attribute requirements as stacked.
	imgRe := regexp.MustCompile(`<img\s+([^>]+)>`)
	for _, m := range imgRe.FindAllStringSubmatch(s, -1) {
		attrs := m[1]
		if !strings.Contains(attrs, `loading="lazy"`) ||
			!strings.Contains(attrs, `decoding="async"`) {
			t.Errorf("carousel img missing lazy/async: %q", attrs)
		}
	}
	videoRe := regexp.MustCompile(`<video\s+([^>]*)>`)
	for _, m := range videoRe.FindAllStringSubmatch(s, -1) {
		attrs := m[1]
		if !strings.Contains(attrs, `preload="metadata"`) ||
			!regexp.MustCompile(`\bcontrols\b`).MatchString(attrs) {
			t.Errorf("carousel video missing preload/controls: %q", attrs)
		}
	}
}

// ---------------------------------------------------------------------------
// HTML-escape test (EV-U-4).
// ---------------------------------------------------------------------------

func TestRenderEvidenceHTML_EscapesScriptTagInputs(t *testing.T) {
	// html/template should escape — agent input is untrusted text. A title
	// containing literal `<script>...</script>` MUST land as escaped text,
	// never as live HTML.
	seq := 1
	in := EvidenceInput{
		Title:    `<script>alert(1)</script>`,
		Subtitle: `</title><script>alert(2)</script>`,
		Summary:  `before <script>alert(3)</script> after`,
		Items: []EvidenceItem{{
			Src:         "./img.png",
			Title:       `<script>alert("item")</script>`,
			Description: `<b>bold</b>`,
			Sequence:    &seq,
		}},
	}

	for _, layout := range []string{"stacked", "carousel"} {
		dir := t.TempDir()
		if err := renderEvidenceHTML(in, layout, dir, Generator{}); err != nil {
			t.Fatalf("%s render: %v", layout, err)
		}
		b, err := os.ReadFile(filepath.Join(dir, "index.html"))
		if err != nil {
			t.Fatalf("%s read index: %v", layout, err)
		}
		s := string(b)

		// Case-sensitive: an attacker-controlled `<script>` substring must
		// NOT appear unescaped anywhere.
		if strings.Contains(s, "<script>") {
			t.Errorf("[%s] unescaped <script> in output (EV-U-4 violation)", layout)
		}
		// Escaped form must appear.
		if !strings.Contains(s, "&lt;script&gt;") {
			t.Errorf("[%s] expected escaped &lt;script&gt; in output; got:\n%s",
				layout, firstFewLines(s, 30))
		}
		// `<b>bold</b>` description must be escaped too.
		if strings.Contains(s, "<b>bold</b>") {
			t.Errorf("[%s] description HTML not escaped", layout)
		}
	}
}

// ---------------------------------------------------------------------------
// Bundle size assertion (fitness function).
// ---------------------------------------------------------------------------

func TestRenderEvidenceHTML_BundleUnder50KB(t *testing.T) {
	// A typical small gallery's index.html + styles.css must stay well
	// under 50 KB combined. The minimal-shell CSS + flat HTML easily fits;
	// this test exists so a future polish pass (T7) doesn't accidentally
	// blow the bundle out by inlining hero images or fonts in CSS.
	for _, layout := range []string{"stacked", "carousel"} {
		dir := t.TempDir()
		if err := renderEvidenceHTML(fixedEvidenceInput(), layout, dir, Generator{Version: "1"}); err != nil {
			t.Fatalf("%s render: %v", layout, err)
		}
		htmlInfo, err := os.Stat(filepath.Join(dir, "index.html"))
		if err != nil {
			t.Fatalf("%s stat html: %v", layout, err)
		}
		cssInfo, err := os.Stat(filepath.Join(dir, "styles.css"))
		if err != nil {
			t.Fatalf("%s stat css: %v", layout, err)
		}
		total := htmlInfo.Size() + cssInfo.Size()
		const limit = 50 * 1024
		if total > limit {
			t.Errorf("[%s] bundle (index.html + styles.css) is %d bytes; limit %d",
				layout, total, limit)
		}
	}
}

// ---------------------------------------------------------------------------
// Defensive layout validation (EV-E-7 belt + braces).
// ---------------------------------------------------------------------------

func TestRenderEvidenceHTML_RejectsUnknownLayout(t *testing.T) {
	dir := t.TempDir()
	err := renderEvidenceHTML(fixedEvidenceInput(), "bogus", dir, Generator{})
	if err == nil {
		t.Fatal("expected error for unknown layout")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("expected error to mention bad layout, got: %v", err)
	}
	// On error we must NOT have written index.html or styles.css.
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
		t.Errorf("index.html was written despite layout error")
	}
}

func TestRenderEvidenceHTML_RejectsEmptyLayout(t *testing.T) {
	dir := t.TempDir()
	err := renderEvidenceHTML(fixedEvidenceInput(), "", dir, Generator{})
	if err == nil {
		t.Fatal("expected error for empty layout")
	}
}

// ---------------------------------------------------------------------------
// End-to-end consistency: HTML hrefs MUST match files placed on disk.
//
// The /compound:build-great-things "evidence" wave originally had two
// independent safe-name implementations — one in T3 (asset placement)
// and one in T4 (HTML hrefs). They diverged (1-based vs 0-based, length
// caps, etc.) and the rendered bundle's HTML pointed at files that did
// not exist on disk. This test pins that the SafeAssetName helper used
// by the HTML template is the SAME function that places bytes on disk,
// by going through the public RenderEvidence entry point and asserting
// every `src="assets/<name>"` href resolves to an actual file in
// outDir/assets/ with the expected bytes.
// ---------------------------------------------------------------------------

func TestRenderEvidence_HTMLAndDiskAreConsistent(t *testing.T) {
	tmp := t.TempDir()
	contRoot := filepath.Join(tmp, "evidence-root")
	if err := os.MkdirAll(filepath.Join(contRoot, "screenshots"), 0o755); err != nil {
		t.Fatalf("mkdir screenshots: %v", err)
	}

	// Fixture inputs: two real PNGs with distinct bytes so the test
	// catches both "wrong filename" and "right filename, wrong bytes"
	// regressions.
	pngA := makePNG(t)
	pngB := append(append([]byte{}, makePNG(t)...), 0x00, 0x01, 0x02) // suffix-tagged variant
	srcA := filepath.Join(contRoot, "screenshots", "01-Empty State.png")
	srcB := filepath.Join(contRoot, "screenshots", "02-error.png")
	writeFileT(t, srcA, pngA)
	writeFileT(t, srcB, pngB)

	seq1, seq2 := 1, 2
	in := EvidenceInput{
		Title: "End-to-end consistency",
		Items: []EvidenceItem{
			{Src: "./screenshots/01-Empty State.png", Title: "Empty", Sequence: &seq1},
			{Src: "./screenshots/02-error.png", Title: "Error", Sequence: &seq2},
		},
	}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	outDir := filepath.Join(tmp, "out")
	_, finalDir, err := RenderEvidence(body, RenderOptions{
		OutDir:          outDir,
		ContainmentRoot: contRoot,
	}, Generator{Version: "test"})
	if err != nil {
		t.Fatalf("RenderEvidence: %v", err)
	}
	if finalDir != outDir {
		t.Fatalf("finalDir %q != outDir %q", finalDir, outDir)
	}

	htmlBytes, err := os.ReadFile(filepath.Join(finalDir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}

	// Pull every `src="assets/<name>"` reference out of the rendered
	// HTML — both <img> and <video> use the same prefix.
	srcRe := regexp.MustCompile(`src="assets/([^"]+)"`)
	matches := srcRe.FindAllStringSubmatch(string(htmlBytes), -1)
	if len(matches) != len(in.Items) {
		t.Fatalf(
			"expected %d assets/<name> hrefs in HTML, got %d. HTML head:\n%s",
			len(in.Items), len(matches), firstFewLines(string(htmlBytes), 40),
		)
	}

	// For each href, the on-disk file must exist with the same bytes
	// as the source it came from. Sources are in-order (the inputs are
	// already sorted by sequence), so href[i] must match item[i]'s src.
	wantBytes := [][]byte{pngA, pngB}
	seenNames := make(map[string]bool, len(matches))
	for i, m := range matches {
		name := m[1]
		if seenNames[name] {
			t.Errorf("duplicate href assets/%q in HTML", name)
		}
		seenNames[name] = true

		got, rerr := os.ReadFile(filepath.Join(finalDir, "assets", name))
		if rerr != nil {
			t.Errorf("HTML references assets/%q but file is missing: %v", name, rerr)
			continue
		}
		if !bytes.Equal(got, wantBytes[i]) {
			t.Errorf(
				"assets/%q has %d bytes; want %d (HTML and disk diverged)",
				name, len(got), len(wantBytes[i]),
			)
		}

		// Belt + braces: the href MUST be exactly what SafeAssetName
		// returns for the same item+index. If a future change adds a
		// second naming pathway (the bug this test exists to prevent),
		// this assertion catches it even if the bytes happen to match.
		expectedName := SafeAssetName(in.Items[i], i)
		if name != expectedName {
			t.Errorf(
				"href[%d] = %q; want %q (HTML must use SafeAssetName)",
				i, name, expectedName,
			)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

func firstFewLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
