package templates

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
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
		Metadata: EvidenceMetadata{
			IssueURL:   "https://jira.example.com/browse/DELIVERY-1234",
			IssueID:    "DELIVERY-1234",
			IssueTitle: "Login page redesign",
		},
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
				Metadata: EvidenceMetadata{
					IssueURL:   "https://jira.example.com/browse/DELIVERY-1234",
					IssueID:    "DELIVERY-1234",
					IssueTitle: "Login page redesign",
				},
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
// Switchable layout golden snapshot test (EV-U-7 determinism +
// EV-U-9 lazy/decoding/controls + controlled static script).
// ---------------------------------------------------------------------------

func TestRenderEvidenceHTML_SwitchableLayoutSnapshot(t *testing.T) {
	in := fixedEvidenceInput()
	g := Generator{Version: "1.2.3", Now: time.Date(2026, 5, 4, 12, 30, 0, 0, time.UTC)}

	dir1 := t.TempDir()
	if err := renderEvidenceHTML(in, dir1, g); err != nil {
		t.Fatalf("render: %v", err)
	}
	dir2 := t.TempDir()
	if err := renderEvidenceHTML(in, dir2, g); err != nil {
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
		t.Errorf("non-deterministic render — bundles differ across runs")
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
	js1, err := os.ReadFile(filepath.Join(dir1, "evidence.js"))
	if err != nil {
		t.Fatalf("read evidence.js: %v", err)
	}
	js2, err := os.ReadFile(filepath.Join(dir2, "evidence.js"))
	if err != nil {
		t.Fatalf("read evidence.js 2: %v", err)
	}
	if string(js1) != string(js2) {
		t.Errorf("non-deterministic evidence.js — bundles differ across runs")
	}
	for _, want := range []string{
		`--ev-carousel-caption-height`,
		`body:has(#ev-layout-carousel:checked) .ev-caption`,
		`max-height: var(--ev-carousel-caption-height)`,
		`grid-template-columns: auto minmax(0, 1fr)`,
		`@media (max-width: 36rem)`,
	} {
		if !strings.Contains(string(css1), want) {
			t.Errorf("styles.css missing responsive/carousel marker %q", want)
		}
	}
	for _, want := range []string{
		`[data-ev-published-at]`,
		`toLocaleString`,
		`(max-width: 48rem)`,
		`setOutlineCollapsed(true)`,
		`mobileQuery.addEventListener("change"`,
		`function stepBy(delta)`,
		`prev.toggleAttribute("disabled"`,
		`next.toggleAttribute("disabled"`,
	} {
		if !strings.Contains(string(js1), want) {
			t.Errorf("evidence.js missing behavior marker %q", want)
		}
	}

	s := string(html1)

	if strings.Contains(s, `data-layout=`) {
		t.Errorf("render should not hard-code layout in the HTML; got:\n%s", firstFewLines(s, 16))
	}

	if strings.Count(s, `<script src="evidence.js" defer></script>`) != 1 {
		t.Errorf("rendered HTML must include exactly one static evidence.js script; got:\n%s", firstFewLines(s, 20))
	}

	for _, want := range []string{
		`localStorage.getItem("butverify:theme")`,
		`class="ev-topbar"`,
		`class="ev-brand"`,
		`Evidence</span>`,
		`ButVerify</span>`,
		`class="ev-meta-strip"`,
		`data-ev-published-at`,
		`2026-05-04T12:30:00Z`,
		`id="ev-meta-panel"`,
		`hidden`,
		`data-ev-theme-toggle`,
		`data-ev-theme-label`,
		`id="ev-layout-stacked"`,
		`id="ev-layout-carousel"`,
		`for="ev-layout-stacked"`,
		`for="ev-layout-carousel"`,
		`class="ev-content-panel"`,
		`class="ev-outline `,
		`Outline panel`,
		`Metadata panel`,
		`data-ev-outline-toggle`,
		`data-ev-prev`,
		`data-ev-next`,
		`disabled`,
		`class="ev-track"`,
		`class="ev-slide"`,
		`class="ev-pager"`,
		`data-ev-page`,
		`data-ev-lightbox`,
		`data-ev-lightbox-trigger`,
		`data-ev-lightbox-zoom-in`,
		`data-ev-lightbox-fullscreen`,
		`data-ev-lightbox-close`,
		`href="#item-1"`,
		`href="#item-3"`,
		`href="https://jira.example.com/browse/DELIVERY-1234"`,
		`Issue ID`,
		`Issue Title`,
		`DELIVERY-1234`,
		`More Details`,
		`class="ev-button-icon"`,
		`class="ev-page-icon"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing switchable layout marker %q", want)
		}
	}
	buttonRe := regexp.MustCompile(`(?s)<button\b[^>]*>.*?</button>`)
	for _, button := range buttonRe.FindAllString(s, -1) {
		if !strings.Contains(button, `class="ev-button-icon"`) && !strings.Contains(button, `class="ev-page-icon"`) {
			t.Errorf("button missing icon: %s", button)
		}
	}

	// EV-U-9: every evidence asset <img> has loading=lazy +
	// decoding=async + non-empty alt. The lightbox shell image is
	// populated by evidence.js at click time and has no static src.
	imgRe := regexp.MustCompile(`<img\s+([^>]*src="assets/[^"]+"[^>]*)>`)
	imgs := imgRe.FindAllStringSubmatch(s, -1)
	if len(imgs) != 2 {
		t.Errorf("expected 2 evidence asset <img> tags, got %d (input has 2 images)", len(imgs))
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

	// Generator and publication metadata are rendered for auditability.
	if !strings.Contains(s, `bv 1.2.3`) {
		t.Errorf("expected generator version \"bv 1.2.3\" in output")
	}
	if !strings.Contains(s, `Published`) || !strings.Contains(s, `data-ev-published-at`) {
		t.Errorf("expected published timestamp metadata in output")
	}
}

func TestRenderEvidenceHTML_ItemPropertiesDetails(t *testing.T) {
	seq := 1
	in := EvidenceInput{
		Title: "Properties proof",
		Items: []EvidenceItem{{
			Src:         "./screenshots/01-empty.png",
			Title:       "Empty state",
			Description: "Page loads with no validation errors.",
			Sequence:    &seq,
			Properties: map[string]any{
				"commit":       "abc123",
				"build_number": json.Number("42"),
				"details": map[string]any{
					"branch": "main",
					"status": "passed",
				},
			},
		}},
	}
	dir := t.TempDir()
	if err := renderEvidenceHTML(in, dir, Generator{Version: "test", Now: time.Date(2026, 5, 4, 12, 30, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("render: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`More Details`,
		`build_number`,
		`42`,
		`commit`,
		`abc123`,
		`details`,
		`ev-json-panel`,
		`ev-json-key`,
		`branch`,
		`main`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("properties render missing %q", want)
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

	dir := t.TempDir()
	if err := renderEvidenceHTML(in, dir, Generator{}); err != nil {
		t.Fatalf("render: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	s := string(b)

	// The static bundle script is allowed, but attacker-controlled script
	// text must not become executable markup.
	if strings.Contains(s, "<script>alert") || strings.Contains(s, `</title><script`) {
		t.Errorf("unescaped attacker script in output (EV-U-4 violation)")
	}
	// Escaped form must appear.
	if !strings.Contains(s, "&lt;script&gt;") {
		t.Errorf("expected escaped &lt;script&gt; in output; got:\n%s",
			firstFewLines(s, 30))
	}
	// `<b>bold</b>` description must be escaped too.
	if strings.Contains(s, "<b>bold</b>") {
		t.Errorf("description HTML not escaped")
	}
}

// ---------------------------------------------------------------------------
// Bundle size assertion (fitness function).
// ---------------------------------------------------------------------------

func TestRenderEvidenceHTML_BundleUnder100KB(t *testing.T) {
	// A typical small gallery's index.html + styles.css + evidence.js must
	// stay under 100 KB combined. This keeps the static shell small while
	// leaving room for readable source assets and richer viewer behavior.
	dir := t.TempDir()
	if err := renderEvidenceHTML(fixedEvidenceInput(), dir, Generator{Version: "1"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	htmlInfo, err := os.Stat(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("stat html: %v", err)
	}
	cssInfo, err := os.Stat(filepath.Join(dir, "styles.css"))
	if err != nil {
		t.Fatalf("stat css: %v", err)
	}
	jsInfo, err := os.Stat(filepath.Join(dir, "evidence.js"))
	if err != nil {
		t.Fatalf("stat evidence.js: %v", err)
	}
	total := htmlInfo.Size() + cssInfo.Size() + jsInfo.Size()
	const limit = 100 * 1024
	if total > limit {
		t.Errorf("bundle (index.html + styles.css + evidence.js) is %d bytes; limit %d", total, limit)
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
	srcRe := regexp.MustCompile(`<(?:img|video)\b[^>]*\ssrc="assets/([^"]+)"`)
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
