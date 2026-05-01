package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseReport_HappyPath(t *testing.T) {
	in, err := ParseReport([]byte(`{
	  "title": "Migration done",
	  "subtitle": "schema 12 -> 13",
	  "sections": [
	    {"type": "headline", "text": "All tables migrated", "tone": "success"},
	    {"type": "kv", "title": "Summary", "items": [
	      {"key": "Tables", "value": "12"},
	      {"key": "Rows", "value": "4.2M"}
	    ]},
	    {"type": "table", "title": "Per-table", "columns": ["Table","Rows"],
	      "rows": [["users","100k"],["orders","4M"]]},
	    {"type": "code", "title": "DDL", "language": "sql", "code": "ALTER TABLE x ADD COLUMN y;"},
	    {"type": "diff", "title": "config", "before": "a\nb\nc", "after": "a\nB\nc"},
	    {"type": "text", "title": "Notes", "body": "Smooth"}
	  ]
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if in.Title != "Migration done" {
		t.Errorf("title: %q", in.Title)
	}
	if len(in.Sections) != 6 {
		t.Errorf("sections: %d", len(in.Sections))
	}
}

func TestParseReport_RejectsUnknownType(t *testing.T) {
	_, err := ParseReport([]byte(`{
	  "title": "X",
	  "sections": [{"type": "interactive_chart"}]
	}`))
	if err == nil {
		t.Fatal("expected error for unknown section type")
	}
	if !strings.Contains(err.Error(), "interactive_chart") {
		t.Errorf("error should mention bad type: %v", err)
	}
}

func TestParseReport_RejectsMissingTitle(t *testing.T) {
	_, err := ParseReport([]byte(`{"sections":[{"type":"headline","text":"x"}]}`))
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Errorf("expected title error, got %v", err)
	}
}

func TestParseReport_RejectsEmptySections(t *testing.T) {
	_, err := ParseReport([]byte(`{"title":"X","sections":[]}`))
	if err == nil || !strings.Contains(err.Error(), "sections") {
		t.Errorf("expected sections error, got %v", err)
	}
}

func TestParseReport_RejectsMalformedTable(t *testing.T) {
	_, err := ParseReport([]byte(`{
	  "title": "T",
	  "sections": [
	    {"type": "table", "columns": ["a","b"], "rows": [["1","2"], ["3"]]}
	  ]
	}`))
	if err == nil || !strings.Contains(err.Error(), "row 1") {
		t.Errorf("expected row mismatch error, got %v", err)
	}
}

func TestParseReport_RejectsBadTone(t *testing.T) {
	_, err := ParseReport([]byte(`{
	  "title": "T",
	  "sections": [{"type":"headline","text":"hi","tone":"explosive"}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "tone") {
		t.Errorf("expected tone error, got %v", err)
	}
}

func TestParseReport_RejectsUnknownFields(t *testing.T) {
	// We use DisallowUnknownFields so a typo in a key fails fast at parse
	// time rather than silently dropping.
	_, err := ParseReport([]byte(`{"title":"T","sections":[{"type":"text","body":"x"}],"extra":1}`))
	if err == nil {
		t.Fatal("expected unknown-field error")
	}
}

func TestRenderReport_WritesIndexAndStyles(t *testing.T) {
	dir := t.TempDir()
	in := []byte(`{
	  "title": "T",
	  "sections": [{"type":"headline","text":"hi","tone":"success"}]
	}`)
	g := Generator{Version: "1.2.3", Now: time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)}
	if _, err := RenderReport(in, dir, g); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !strings.Contains(string(html), `<title>T</title>`) {
		t.Errorf("missing title tag: %s", html)
	}
	if !strings.Contains(string(html), `tone-success`) {
		t.Errorf("missing tone class: %s", html)
	}
	if !strings.Contains(string(html), `2026-04-27T10:00:00Z`) {
		t.Errorf("missing fixed timestamp: %s", html)
	}
	// Styles asset.
	if _, err := os.Stat(filepath.Join(dir, "assets", "styles.css")); err != nil {
		t.Errorf("styles.css not written: %v", err)
	}
}

func TestRenderReport_EscapesHTML(t *testing.T) {
	// html/template should escape — agent input is untrusted text from
	// whatever JSON they assembled, so an injected </h1><script> must NOT
	// land as live HTML.
	dir := t.TempDir()
	in := []byte(`{
	  "title": "<script>alert(1)</script>",
	  "sections": [{"type":"text","body":"<b>hi</b>"}]
	}`)
	if _, err := RenderReport(in, dir, Generator{}); err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	s := string(html)
	if strings.Contains(s, "<script>alert(1)</script>") {
		t.Errorf("title not escaped: %s", s)
	}
	if strings.Contains(s, "<b>hi</b>") {
		t.Errorf("body not escaped: %s", s)
	}
	if !strings.Contains(s, "&lt;script&gt;") {
		t.Errorf("expected escaped script: %s", s)
	}
}

func TestRenderReport_DiffBeforeAfter(t *testing.T) {
	dir := t.TempDir()
	in := []byte(`{
	  "title": "T",
	  "sections": [
	    {"type":"diff","title":"d","before":"old\nshared","after":"shared\nnew"}
	  ]
	}`)
	if _, err := RenderReport(in, dir, Generator{}); err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	s := string(html)
	if !strings.Contains(s, `class="bv-diff-line rem"`) {
		t.Errorf("missing rem class for naive diff: %s", s)
	}
	if !strings.Contains(s, `class="bv-diff-line add"`) {
		t.Errorf("missing add class for naive diff: %s", s)
	}
}

func TestRenderReport_DiffUnified(t *testing.T) {
	dir := t.TempDir()
	in := []byte(`{
	  "title": "T",
	  "sections": [
	    {"type":"diff","title":"d","unified":"--- a/x\n+++ b/x\n@@ -1,3 +1,3 @@\n line1\n-old\n+new\n line3\n"}
	  ]
	}`)
	if _, err := RenderReport(in, dir, Generator{}); err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	s := string(html)
	if !strings.Contains(s, `&#43;new`) && !strings.Contains(s, `+new`) {
		t.Errorf("unified +new not rendered: %s", s)
	}
}

func TestRenderReport_DeterministicOutput(t *testing.T) {
	// Re-rendering the same input twice must produce byte-identical output —
	// templated push relies on deterministic site_id derivation.
	in := []byte(`{
	  "title": "T",
	  "sections": [{"type":"text","body":"hello"}]
	}`)
	g := Generator{Version: "v1", Now: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)}

	dir1 := t.TempDir()
	if _, err := RenderReport(in, dir1, g); err != nil {
		t.Fatal(err)
	}
	dir2 := t.TempDir()
	if _, err := RenderReport(in, dir2, g); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(filepath.Join(dir1, "index.html"))
	b, _ := os.ReadFile(filepath.Join(dir2, "index.html"))
	if string(a) != string(b) {
		t.Errorf("non-deterministic render")
	}
}
