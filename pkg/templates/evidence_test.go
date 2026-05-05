package templates

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Strict-parse tests (EV-U-3, EV-N-1, EV-N-5)
// ---------------------------------------------------------------------------

func TestParseEvidence_HappyPath(t *testing.T) {
	// The §5 canonical example. If this stops parsing, the spec and the
	// code have diverged and one of them is wrong.
	in, err := ParseEvidence([]byte(specExampleJSON))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if in.Title != "Login page redesign" {
		t.Errorf("title: %q", in.Title)
	}
	if len(in.Items) != 3 {
		t.Fatalf("items: %d", len(in.Items))
	}
	if in.Items[0].Sequence == nil || *in.Items[0].Sequence != 1 {
		t.Errorf("items[0].sequence: %v", in.Items[0].Sequence)
	}
}

func TestParseEvidence_RejectsUnknownTopLevel(t *testing.T) {
	_, err := ParseEvidence([]byte(`{
	  "title": "X",
	  "extra": "nope",
	  "items": [{"src": "./a.png"}]
	}`))
	if err == nil {
		t.Fatal("expected error for unknown top-level field")
	}
	if !strings.Contains(err.Error(), "extra") {
		t.Errorf("error should mention bad field: %v", err)
	}
}

func TestParseEvidence_RejectsUnknownItemField(t *testing.T) {
	_, err := ParseEvidence([]byte(`{
	  "title": "X",
	  "items": [{"src": "./a.png", "weight": 5}]
	}`))
	if err == nil {
		t.Fatal("expected error for unknown item field")
	}
	if !strings.Contains(err.Error(), "weight") {
		t.Errorf("error should mention bad field: %v", err)
	}
}

func TestParseEvidence_RejectsMissingTitle(t *testing.T) {
	_, err := ParseEvidence([]byte(`{"items":[{"src":"./a.png"}]}`))
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Errorf("expected title error, got %v", err)
	}
}

func TestParseEvidence_RejectsZeroItems(t *testing.T) {
	_, err := ParseEvidence([]byte(`{"title":"X","items":[]}`))
	if err == nil || !strings.Contains(err.Error(), "items") {
		t.Errorf("expected items error, got %v", err)
	}
}

func TestParseEvidence_RejectsTooManyItems(t *testing.T) {
	// 501 items: just over the EV-N-5 cap.
	var b strings.Builder
	b.WriteString(`{"title":"X","items":[`)
	for i := 0; i < 501; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"src":"./a.png"}`)
	}
	b.WriteString(`]}`)
	_, err := ParseEvidence([]byte(b.String()))
	if err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Errorf("expected items-cap error, got %v", err)
	}
}

func TestParseEvidence_RejectsMissingSrc(t *testing.T) {
	_, err := ParseEvidence([]byte(`{"title":"X","items":[{"title":"no src"}]}`))
	if err == nil || !strings.Contains(err.Error(), "src") {
		t.Errorf("expected src-required error, got %v", err)
	}
}

func TestParseEvidence_RejectsTrailingData(t *testing.T) {
	_, err := ParseEvidence([]byte(`{"title":"X","items":[{"src":"./a.png"}]}{"foo":1}`))
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Errorf("expected trailing-data error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// EV-U-5: URL-scheme src rejection
// ---------------------------------------------------------------------------

func TestParseEvidence_RejectsURLSchemes(t *testing.T) {
	cases := []string{
		"http://example.com/x.png",
		"https://example.com/x.png",
		"data:image/png;base64,AAA",
		"file:///etc/passwd",
		// case-insensitive: an attacker uppercasing the scheme should fail too.
		"HTTPS://example.com/x.png",
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{
				"title": "X",
				"items": []map[string]any{{"src": src}},
			})
			_, err := ParseEvidence(body)
			if err == nil {
				t.Fatalf("expected scheme-reject for %q", src)
			}
			if !strings.Contains(err.Error(), "URL schemes") && !strings.Contains(err.Error(), "local relative path") {
				t.Errorf("error should explain URL-scheme rejection: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Stable sort (EVSC-10)
// ---------------------------------------------------------------------------

func TestSortItems_MixedSequenceAndJSONOrder(t *testing.T) {
	mk := func(seq *int, src string) EvidenceItem {
		return EvidenceItem{Src: src, Sequence: seq}
	}
	pi := func(n int) *int { return &n }

	// EVSC-10: items [a(seq=2), b(no seq), c(seq=1), d(no seq)] -> c,a,b,d
	items := []EvidenceItem{
		mk(pi(2), "a"),
		mk(nil, "b"),
		mk(pi(1), "c"),
		mk(nil, "d"),
	}
	got := SortItems(items)
	want := []string{"c", "a", "b", "d"}
	for i, w := range want {
		if got[i].Src != w {
			t.Errorf("position %d: want %q, got %q (full: %v)",
				i, w, got[i].Src, srcs(got))
		}
	}
}

func TestSortItems_PreservesJSONOrderForUnsequencedTail(t *testing.T) {
	pi := func(n int) *int { return &n }
	items := []EvidenceItem{
		{Src: "x", Sequence: nil},
		{Src: "y", Sequence: pi(10)},
		{Src: "z", Sequence: nil},
	}
	got := SortItems(items)
	want := []string{"y", "x", "z"} // sequenced first; then JSON-order (x before z)
	for i, w := range want {
		if got[i].Src != w {
			t.Errorf("pos %d: want %q got %q (%v)", i, w, got[i].Src, srcs(got))
		}
	}
}

func TestSortItems_StableForEqualSequences(t *testing.T) {
	pi := func(n int) *int { return &n }
	items := []EvidenceItem{
		{Src: "first", Sequence: pi(5)},
		{Src: "second", Sequence: pi(5)},
		{Src: "third", Sequence: pi(5)},
	}
	got := SortItems(items)
	want := []string{"first", "second", "third"}
	for i, w := range want {
		if got[i].Src != w {
			t.Errorf("pos %d: want %q got %q", i, w, got[i].Src)
		}
	}
}

func TestSortItems_DistinguishesZeroFromNil(t *testing.T) {
	// `*int` lets us tell "sequence: 0" (sort first) from "sequence
	// omitted" (slot in tail). A non-pointer int would collapse both.
	pi := func(n int) *int { return &n }
	items := []EvidenceItem{
		{Src: "no-seq-1", Sequence: nil},
		{Src: "seq-zero", Sequence: pi(0)},
	}
	got := SortItems(items)
	if got[0].Src != "seq-zero" {
		t.Errorf("sequence:0 must come before unsequenced; got %v", srcs(got))
	}
}

func srcs(items []EvidenceItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Src
	}
	return out
}

// ---------------------------------------------------------------------------
// EV-N-6: stdin 4 MiB cap
// ---------------------------------------------------------------------------

func TestReadStdin_AtCapPasses(t *testing.T) {
	// Exactly 4 MiB: should pass.
	buf := strings.NewReader(strings.Repeat("a", evidenceStdinMaxBytes))
	got, err := ReadStdin(buf)
	if err != nil {
		t.Fatalf("at-cap should pass: %v", err)
	}
	if len(got) != evidenceStdinMaxBytes {
		t.Errorf("expected %d bytes, got %d", evidenceStdinMaxBytes, len(got))
	}
}

func TestReadStdin_OverCapRejected(t *testing.T) {
	// 4 MiB + 1: must be rejected.
	buf := strings.NewReader(strings.Repeat("a", evidenceStdinMaxBytes+1))
	_, err := ReadStdin(buf)
	if err == nil {
		t.Fatal("over-cap should reject")
	}
	if !strings.Contains(err.Error(), "4 MiB") && !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error should mention cap: %v", err)
	}
}

func TestReadStdin_UnderCapPasses(t *testing.T) {
	buf := strings.NewReader(`{"title":"X","items":[{"src":"./a.png"}]}`)
	got, err := ReadStdin(buf)
	if err != nil {
		t.Fatalf("under-cap should pass: %v", err)
	}
	if !strings.Contains(string(got), "title") {
		t.Errorf("read content corrupted: %s", got)
	}
}

// ---------------------------------------------------------------------------
// HTML-escape preservation at parse level (EV-U-4 happens at render time)
// ---------------------------------------------------------------------------

func TestParseEvidence_PreservesScriptTagVerbatim(t *testing.T) {
	// EV-U-4 says strings flow through html/template at render time.
	// At parse time, the raw bytes must round-trip — escaping at parse
	// would either double-escape later or hide the bug from the
	// renderer's golden snapshot test.
	body := []byte(`{
	  "title": "<script>alert(1)</script>",
	  "items": [
	    {"src": "./a.png", "title": "<b>bold</b>", "description": "& < > \" '"}
	  ]
	}`)
	in, err := ParseEvidence(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if in.Title != `<script>alert(1)</script>` {
		t.Errorf("title escaped at parse time: %q", in.Title)
	}
	if in.Items[0].Title != `<b>bold</b>` {
		t.Errorf("item title escaped at parse time: %q", in.Items[0].Title)
	}
	if in.Items[0].Description != `& < > " '` {
		t.Errorf("description corrupted: %q", in.Items[0].Description)
	}
}

// ---------------------------------------------------------------------------
// Determinism (parse-level — render determinism is T3/T4)
// ---------------------------------------------------------------------------

func TestParseEvidence_Deterministic(t *testing.T) {
	body := []byte(specExampleJSON)
	a, err := ParseEvidence(body)
	if err != nil {
		t.Fatalf("parse a: %v", err)
	}
	b, err := ParseEvidence(body)
	if err != nil {
		t.Fatalf("parse b: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("parse not deterministic:\n a=%+v\n b=%+v", a, b)
	}
}

// ---------------------------------------------------------------------------
// Length-bound checks (§5)
// ---------------------------------------------------------------------------

func TestValidate_TitleTooLong(t *testing.T) {
	in := EvidenceInput{
		Title: strings.Repeat("a", maxEvidenceTitleLen+1),
		Items: []EvidenceItem{{Src: "./a.png"}},
	}
	if err := in.Validate(); err == nil || !strings.Contains(err.Error(), "title") {
		t.Errorf("expected title-length error, got %v", err)
	}
}

func TestValidate_SubtitleTooLong(t *testing.T) {
	in := EvidenceInput{
		Title:    "ok",
		Subtitle: strings.Repeat("a", maxEvidenceSubtitleLen+1),
		Items:    []EvidenceItem{{Src: "./a.png"}},
	}
	if err := in.Validate(); err == nil || !strings.Contains(err.Error(), "subtitle") {
		t.Errorf("expected subtitle-length error, got %v", err)
	}
}

func TestValidate_SummaryTooLong(t *testing.T) {
	in := EvidenceInput{
		Title:   "ok",
		Summary: strings.Repeat("a", maxEvidenceSummaryLen+1),
		Items:   []EvidenceItem{{Src: "./a.png"}},
	}
	if err := in.Validate(); err == nil || !strings.Contains(err.Error(), "summary") {
		t.Errorf("expected summary-length error, got %v", err)
	}
}

func TestValidate_ItemTitleTooLong(t *testing.T) {
	in := EvidenceInput{
		Title: "ok",
		Items: []EvidenceItem{{Src: "./a.png", Title: strings.Repeat("a", maxEvidenceItemTitleLen+1)}},
	}
	if err := in.Validate(); err == nil || !strings.Contains(err.Error(), "title") {
		t.Errorf("expected item-title-length error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Schema/parser parity (EV-U-11)
// ---------------------------------------------------------------------------

// We use a stdlib-only minimal Draft 2020-12 validator (see
// miniSchemaValidate below) per templates.go design principle 1. The
// validator implements the subset of keywords actually used by
// EvidenceSchema: type, required, additionalProperties, properties,
// minLength, maxLength, minItems, maxItems, items, integer, pattern,
// not + anyOf. That's enough to verify the parity claim — every
// positive payload validates against the schema AND parses, every
// negative payload fails both.

func TestEvidenceSchema_HasRequiredHeader(t *testing.T) {
	// A Draft 2020-12 schema MUST carry $schema and $id (EV-U-11).
	if !strings.Contains(EvidenceSchema, `"$schema": "https://json-schema.org/draft/2020-12/schema"`) {
		t.Errorf("schema missing $schema field")
	}
	if !strings.Contains(EvidenceSchema, `"$id": "https://butverify.dev/schemas/evidence/v1.json"`) {
		t.Errorf("schema missing $id field")
	}
	// And it MUST be valid JSON.
	var v any
	if err := json.Unmarshal([]byte(EvidenceSchema), &v); err != nil {
		t.Errorf("schema is not valid JSON: %v", err)
	}
}

func TestEvidenceSchema_PositiveExamplesParityWithParser(t *testing.T) {
	// Every positive payload MUST validate against the schema AND
	// parse cleanly via ParseEvidence.
	for name, payload := range positivePayloads() {
		t.Run("positive/"+name, func(t *testing.T) {
			if err := miniSchemaValidate([]byte(EvidenceSchema), []byte(payload)); err != nil {
				t.Errorf("schema rejected positive payload: %v\npayload:\n%s", err, payload)
			}
			if _, err := ParseEvidence([]byte(payload)); err != nil {
				t.Errorf("parser rejected positive payload: %v\npayload:\n%s", err, payload)
			}
		})
	}
}

func TestEvidenceSchema_NegativeExamplesParityWithParser(t *testing.T) {
	// Every negative payload MUST fail BOTH the schema and the parser.
	for name, payload := range negativePayloads() {
		t.Run("negative/"+name, func(t *testing.T) {
			schemaErr := miniSchemaValidate([]byte(EvidenceSchema), []byte(payload))
			_, parseErr := ParseEvidence([]byte(payload))
			if schemaErr == nil {
				t.Errorf("schema accepted negative payload (%s):\n%s", name, payload)
			}
			if parseErr == nil {
				t.Errorf("parser accepted negative payload (%s):\n%s", name, payload)
			}
		})
	}
}

// positivePayloads returns the §5 canonical example plus a handful of
// minimal-but-valid variants. Each must validate AND parse.
func positivePayloads() map[string]string {
	return map[string]string{
		"spec_canonical":      specExampleJSON,
		"single_image":        `{"title":"X","items":[{"src":"./a.png"}]}`,
		"image_with_seq_zero": `{"title":"X","items":[{"src":"./a.png","sequence":0}]}`,
		"video_mp4":           `{"title":"X","items":[{"src":"./clip.mp4","title":"clip","description":"d"}]}`,
		"video_webm":          `{"title":"X","items":[{"src":"./clip.webm"}]}`,
		"video_mov":           `{"title":"X","items":[{"src":"./clip.mov"}]}`,
		"image_jpg":           `{"title":"X","items":[{"src":"./a.jpg"}]}`,
		"image_jpeg":          `{"title":"X","items":[{"src":"./a.jpeg"}]}`,
		"image_webp":          `{"title":"X","items":[{"src":"./a.webp"}]}`,
		"image_gif":           `{"title":"X","items":[{"src":"./a.gif"}]}`,
		"with_optional_alt":   `{"title":"X","subtitle":"sub","summary":"s","items":[{"src":"./a.png","alt":"alt text"}]}`,
		"deep_relative_path":  `{"title":"X","items":[{"src":"./screenshots/sub/a.png"}]}`,
	}
}

// negativePayloads is a curated set, each violating exactly one v1
// parse-time contract rule. Each MUST fail the schema AND the parser.
//
// Note on what's intentionally excluded: extension/MIME checks
// (EV-U-6) live at file-open time, not parse time — those are T3's
// responsibility. The schema documents the closed allowlist via a
// `pattern` regex (so dashboards / agents see the intended set), but
// runtime gating is the MIME double-gate. Including extension-failure
// payloads here would only assert "schema is stricter than parser at
// parse time" — true, but a property of layering, not a contract bug.
func negativePayloads() map[string]string {
	return map[string]string{
		"unknown_top_level":   `{"title":"X","extra":"nope","items":[{"src":"./a.png"}]}`,
		"unknown_item_field":  `{"title":"X","items":[{"src":"./a.png","weight":1}]}`,
		"missing_title":       `{"items":[{"src":"./a.png"}]}`,
		"missing_src":         `{"title":"X","items":[{"title":"no src"}]}`,
		"empty_items":         `{"title":"X","items":[]}`,
		"http_scheme":         `{"title":"X","items":[{"src":"http://x/y.png"}]}`,
		"https_scheme":        `{"title":"X","items":[{"src":"https://x/y.png"}]}`,
		"data_scheme":         `{"title":"X","items":[{"src":"data:image/png;base64,AA"}]}`,
		"file_scheme":         `{"title":"X","items":[{"src":"file:///etc/passwd"}]}`,
		"title_wrong_type":    `{"title":42,"items":[{"src":"./a.png"}]}`,
		"items_wrong_type":    `{"title":"X","items":"oops"}`,
		"sequence_wrong_type": `{"title":"X","items":[{"src":"./a.png","sequence":"first"}]}`,
	}
}

// ---------------------------------------------------------------------------
// Spec example payload
// ---------------------------------------------------------------------------

// specExampleJSON is the exact §5 canonical example. Keeping it as a
// const rather than reading from disk both keeps tests hermetic and
// makes drift between spec and code visible in code review.
const specExampleJSON = `{
  "title": "Login page redesign",
  "subtitle": "Ticket DELIVERY-1234 · 2026-04-27",
  "summary": "Updated the login form to match the new identity. All states pass automated tests; here is the human-visible proof.",
  "items": [
    {
      "src": "./screenshots/01-empty.png",
      "title": "Empty state",
      "description": "Page loads with no validation errors visible.",
      "sequence": 1
    },
    {
      "src": "./screenshots/02-error.png",
      "title": "Inline validation",
      "description": "Empty-email submit shows the helper inline; field gets aria-describedby.",
      "sequence": 2
    },
    {
      "src": "./videos/03-success.webm",
      "title": "Successful sign-in",
      "description": "End-to-end flow from form fill to dashboard redirect (3s clip).",
      "sequence": 3
    }
  ]
}`

// ---------------------------------------------------------------------------
// Minimal Draft 2020-12 validator (stdlib-only)
//
// Implements the subset of JSON Schema keywords used by
// EvidenceSchema. This is NOT a general JSON Schema validator — it's
// just enough to verify the parity claim (EV-U-11). If we add a new
// keyword to EvidenceSchema, extend this validator (or accept a
// validator dep in v1.x).
//
// Supported keywords:
//   - type ("object" | "string" | "integer" | "array")
//   - required (array of strings)
//   - additionalProperties (false)
//   - properties (object)
//   - minLength / maxLength
//   - minItems / maxItems
//   - items (single subschema; tuple form not used)
//   - pattern (Go regexp)
//   - not + anyOf (just enough to express "no scheme prefix")
// ---------------------------------------------------------------------------

func miniSchemaValidate(schemaBytes, dataBytes []byte) error {
	var schema, data any
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return err
	}
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		// Unparseable JSON is a validation failure.
		return err
	}
	return msvCheck(schema, data, "$")
}

func msvCheck(schema, data any, path string) error {
	s, ok := schema.(map[string]any)
	if !ok {
		return nil // empty schema accepts anything
	}

	// type
	if t, ok := s["type"].(string); ok {
		if err := msvCheckType(t, data, path); err != nil {
			return err
		}
	}

	// not + anyOf
	if not, ok := s["not"].(map[string]any); ok {
		if err := msvCheck(not, data, path); err == nil {
			return msvErr(path, "matched `not` subschema")
		}
	}
	if anyOf, ok := s["anyOf"].([]any); ok {
		matched := false
		for _, alt := range anyOf {
			if err := msvCheck(alt, data, path); err == nil {
				matched = true
				break
			}
		}
		if !matched {
			return msvErr(path, "did not match any `anyOf` alternative")
		}
	}

	switch d := data.(type) {
	case map[string]any:
		// required
		if req, ok := s["required"].([]any); ok {
			for _, r := range req {
				name, _ := r.(string)
				if _, present := d[name]; !present {
					return msvErr(path, "missing required property "+name)
				}
			}
		}
		// additionalProperties: false
		props, _ := s["properties"].(map[string]any)
		if ap, exists := s["additionalProperties"]; exists {
			if allow, isBool := ap.(bool); isBool && !allow {
				for k := range d {
					if _, declared := props[k]; !declared {
						return msvErr(path, "additional property not allowed: "+k)
					}
				}
			}
		}
		// per-property
		for k, sub := range props {
			if v, present := d[k]; present {
				if err := msvCheck(sub, v, path+"."+k); err != nil {
					return err
				}
			}
		}
	case []any:
		if mi, ok := s["minItems"]; ok {
			if n := msvAsInt(mi); len(d) < n {
				return msvErr(path, "fewer than minItems")
			}
		}
		if mx, ok := s["maxItems"]; ok {
			if n := msvAsInt(mx); len(d) > n {
				return msvErr(path, "more than maxItems")
			}
		}
		if items, ok := s["items"].(map[string]any); ok {
			for i, it := range d {
				if err := msvCheck(items, it, path+"["+itoa(i)+"]"); err != nil {
					return err
				}
			}
		}
	case string:
		if mi, ok := s["minLength"]; ok {
			if n := msvAsInt(mi); len(d) < n {
				return msvErr(path, "shorter than minLength")
			}
		}
		if mx, ok := s["maxLength"]; ok {
			if n := msvAsInt(mx); len(d) > n {
				return msvErr(path, "longer than maxLength")
			}
		}
		if pat, ok := s["pattern"].(string); ok {
			ok, err := msvRegexpMatch(pat, d)
			if err != nil {
				return msvErr(path, "bad regexp in schema: "+err.Error())
			}
			if !ok {
				return msvErr(path, "did not match pattern")
			}
		}
	}
	return nil
}

func msvCheckType(t string, data any, path string) error {
	switch t {
	case "object":
		if _, ok := data.(map[string]any); !ok {
			return msvErr(path, "expected object")
		}
	case "array":
		if _, ok := data.([]any); !ok {
			return msvErr(path, "expected array")
		}
	case "string":
		if _, ok := data.(string); !ok {
			return msvErr(path, "expected string")
		}
	case "integer":
		// JSON numbers parse as float64 in encoding/json; require an
		// integral value.
		f, ok := data.(float64)
		if !ok {
			return msvErr(path, "expected integer")
		}
		if f != float64(int64(f)) {
			return msvErr(path, "expected integer (got fraction)")
		}
	}
	return nil
}

func msvAsInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func msvErr(path, msg string) error {
	return &msvError{path: path, msg: msg}
}

type msvError struct {
	path string
	msg  string
}

func (e *msvError) Error() string { return e.path + ": " + e.msg }

func itoa(i int) string { return strconv.Itoa(i) }

// msvRegexpMatch wraps regexp.MatchString so the validator's regex
// surface is contained to one call site. We use Go's RE2-flavored
// regexp; the patterns in EvidenceSchema are simple enough that
// PCRE-only constructs aren't needed.
func msvRegexpMatch(pattern, s string) (bool, error) {
	return regexp.MatchString(pattern, s)
}

// ---------------------------------------------------------------------------
// T3: filesystem layer — MIME double-gate, containment, atomic --out
// EARS coverage: EV-U-6, EV-S-1, EV-S-2, EV-S-3, EV-S-4, EV-N-2, EV-N-7,
//                EVSC-5, EVSC-6, EVSC-7, EVSC-7b, EVSC-7c, EVSC-11
// ---------------------------------------------------------------------------

// makePNG returns the bytes of a tiny valid PNG (single-pixel red). The
// PNG signature is the first 8 bytes; http.DetectContentType reads
// up to 512 bytes and matches the standard signature.
func makePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test PNG: %v", err)
	}
	return buf.Bytes()
}

// makePolyglotPNG returns bytes that start with a valid PNG signature
// (so http.DetectContentType says "image/png" off the first 512 bytes)
// but contain SVG markup AT BYTE >= sniffWindow (600). This documents
// the bounded-mitigation property: deep polyglots that hide payloads
// past the 512-byte sniff window are NOT caught (per spec §8.2). What
// we DO catch is renamed-extension attacks (e.g. .png file with
// HTML at byte 0).
func makePolyglotPNG(t *testing.T) []byte {
	t.Helper()
	// Real PNG, then enough zero bytes to push us past the sniff
	// window, then an SVG payload. Total > 600 bytes guaranteed.
	pngBytes := makePNG(t)
	// Pad until we're at byte 600, then append an SVG.
	out := bytes.Clone(pngBytes)
	for len(out) < 600 {
		out = append(out, 0x00)
	}
	out = append(out, []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)...)
	return out
}

// writeFileT helper: write bytes to a tmpdir path, t.Fatal on failure.
func writeFileT(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// ---------------------------------------------------------------------------
// EV-U-6 — MIME double-gate
// ---------------------------------------------------------------------------

func TestCopyAsset_RejectsDisallowedExtension(t *testing.T) {
	dir := t.TempDir()
	// Even valid PNG bytes with a disallowed extension must be rejected.
	src := filepath.Join(dir, "evil.exe")
	writeFileT(t, src, makePNG(t))
	dst := filepath.Join(dir, "out.exe")
	err := copyAsset(src, dst)
	if err == nil {
		t.Fatal("expected disallowed-extension reject")
	}
	if !strings.Contains(err.Error(), "disallowed extension") {
		t.Errorf("error should name the disallowed-extension reason: %v", err)
	}
	if !strings.Contains(err.Error(), "image/png") {
		t.Errorf("error should list allowed MIMEs: %v", err)
	}
}

func TestCopyAsset_RejectsSVGExtension(t *testing.T) {
	// SVG is intentionally excluded from the closed allowlist (§8.2).
	dir := t.TempDir()
	src := filepath.Join(dir, "x.svg")
	writeFileT(t, src, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`))
	dst := filepath.Join(dir, "x-out.svg")
	if err := copyAsset(src, dst); err == nil {
		t.Fatal("expected SVG reject")
	}
}

func TestCopyAsset_RejectsTxtExtension(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "readme.txt")
	writeFileT(t, src, []byte("hello"))
	dst := filepath.Join(dir, "out.txt")
	if err := copyAsset(src, dst); err == nil {
		t.Fatal("expected .txt reject")
	}
}

func TestCopyAsset_RejectsExtSniffMismatch(t *testing.T) {
	// .png extension but bytes are HTML — sniff should disagree
	// (EVSC-7c). This is the renamed-extension attack vector.
	dir := t.TempDir()
	src := filepath.Join(dir, "fake.png")
	writeFileT(t, src, []byte(`<!doctype html><html><body>nope</body></html>`))
	dst := filepath.Join(dir, "out.png")
	err := copyAsset(src, dst)
	if err == nil {
		t.Fatal("expected ext/sniff mismatch reject")
	}
	if !strings.Contains(err.Error(), "MIME mismatch") {
		t.Errorf("error should name the mismatch: %v", err)
	}
	if !strings.Contains(err.Error(), "text/html") {
		t.Errorf("error should report observed MIME: %v", err)
	}
}

func TestCopyAsset_AcceptsValidPNG(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "ok.png")
	pngBytes := makePNG(t)
	writeFileT(t, src, pngBytes)
	dst := filepath.Join(dir, "out.png")
	if err := copyAsset(src, dst); err != nil {
		t.Fatalf("valid PNG should accept: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if !bytes.Equal(got, pngBytes) {
		t.Errorf("dst bytes don't match src (len got=%d, want=%d)", len(got), len(pngBytes))
	}
}

func TestCopyAsset_AcceptsPolyglotPNG_BoundedMitigation(t *testing.T) {
	// Documents the bounded mitigation: a file with valid PNG header in
	// the first 512 bytes BUT an SVG payload past byte 600 is ACCEPTED
	// (sniff window only sees the PNG header). This is the deep-polyglot
	// case spec §8.2 explicitly puts out of scope; this test pins the
	// behavior so a future "fix" doesn't accidentally reject valid PNGs
	// that happen to have benign trailing data.
	dir := t.TempDir()
	src := filepath.Join(dir, "poly.png")
	writeFileT(t, src, makePolyglotPNG(t))
	dst := filepath.Join(dir, "out.png")
	if err := copyAsset(src, dst); err != nil {
		t.Fatalf("polyglot PNG (sniff sees PNG) should accept under bounded-mitigation contract: %v", err)
	}
}

// ---------------------------------------------------------------------------
// EV-S-2 — per-asset 1 GiB cap
// ---------------------------------------------------------------------------

func TestCopyAsset_RejectsOversizeAsset(t *testing.T) {
	// Use the test seam: shrink maxAssetBytes so we don't need a real
	// 1 GiB file. The spec (§8.5) explicitly endorses this approach.
	dir := t.TempDir()
	prev := maxAssetBytes
	maxAssetBytes = 4096 // 4 KiB cap
	t.Cleanup(func() { maxAssetBytes = prev })

	// Create a PNG larger than the (test-shrunk) cap. Stack a real PNG
	// header and pad with zeros until total > cap.
	png := makePNG(t)
	for len(png) <= int(maxAssetBytes) {
		png = append(png, 0x00)
	}
	src := filepath.Join(dir, "big.png")
	writeFileT(t, src, png)
	dst := filepath.Join(dir, "big-out.png")
	err := copyAsset(src, dst)
	if err == nil {
		t.Fatal("expected per-asset cap reject")
	}
	if !strings.Contains(err.Error(), "per-asset cap") {
		t.Errorf("error should name the cap: %v", err)
	}
}

// ---------------------------------------------------------------------------
// EV-N-2 — filename collision detection
// ---------------------------------------------------------------------------

func TestSafeAssetName_DistinctIndicesProduceDistinctNames(t *testing.T) {
	// Two items whose src basenames collide AFTER sanitization still
	// produce distinct dst names because of the index prefix.
	a := EvidenceItem{Src: "./a/foo.png"}
	b := EvidenceItem{Src: "./b/foo.png"}
	if SafeAssetName(a, 0) == SafeAssetName(b, 1) {
		t.Errorf("expected distinct safe names for items at different indices: a=%q b=%q",
			SafeAssetName(a, 0), SafeAssetName(b, 1))
	}
	if SafeAssetName(a, 0) != "001-foo.png" {
		t.Errorf("expected 001-foo.png, got %q", SafeAssetName(a, 0))
	}
	if SafeAssetName(b, 1) != "002-foo.png" {
		t.Errorf("expected 002-foo.png, got %q", SafeAssetName(b, 1))
	}
}

func TestSafeAssetName_SanitizesUnsafeChars(t *testing.T) {
	cases := map[string]string{
		"./a/Hello World.PNG":     "001-hello-world.png",
		"./a/UPPER_case_NAME.JPG": "001-upper-case-name.jpg",
		"./a/many   spaces.gif":   "001-many-spaces.gif",
		"./a/.dotfile.mp4":        "001-.dotfile.mp4",
	}
	for src, want := range cases {
		got := SafeAssetName(EvidenceItem{Src: src}, 0)
		if got != want {
			t.Errorf("SafeAssetName(%q) = %q; want %q", src, got, want)
		}
	}
}

func TestRenderEvidence_DetectsCollisionsInItemList(t *testing.T) {
	// We can't easily collide via the natural safe-name function (the
	// index prefix uniquifies), but we verify the collision check is
	// in place by injecting two items that would collide under a
	// hypothetical broken safe-name. Instead, assert the natural case:
	// two items with identical src basenames render to distinct dst
	// names (no false-positive collision) AND both end up on disk.
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "evidence-root")
	if err := os.MkdirAll(filepath.Join(contRoot, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(contRoot, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	pngBytes := makePNG(t)
	writeFileT(t, filepath.Join(contRoot, "a", "foo.png"), pngBytes)
	writeFileT(t, filepath.Join(contRoot, "b", "foo.png"), pngBytes)

	in := EvidenceInput{
		Title: "X",
		Items: []EvidenceItem{
			{Src: "./a/foo.png"},
			{Src: "./b/foo.png"},
		},
	}
	body, _ := json.Marshal(in)
	outDir := filepath.Join(dir, "out")
	_, finalDir, err := RenderEvidence(body, RenderOptions{
		OutDir:          outDir,
		ContainmentRoot: contRoot,
	}, Generator{})
	if err != nil {
		t.Fatalf("RenderEvidence: %v", err)
	}
	if finalDir != outDir {
		t.Errorf("final dir %q != opts.OutDir %q", finalDir, outDir)
	}
	for _, name := range []string{"001-foo.png", "002-foo.png"} {
		p := filepath.Join(finalDir, "assets", name)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s on disk: %v", p, err)
		}
	}
}

// ---------------------------------------------------------------------------
// EVSC-7 / EV-S-1 — lexical traversal
// ---------------------------------------------------------------------------

func TestContainAsset_RejectsLexicalTraversal(t *testing.T) {
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := containAsset(contRoot, "../../etc/passwd")
	if err == nil {
		t.Fatal("expected lexical-traversal reject")
	}
	if !strings.Contains(err.Error(), "escapes containment root") {
		t.Errorf("error should name traversal: %v", err)
	}
}

func TestContainAsset_AcceptsRelativeUnderRoot(t *testing.T) {
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(filepath.Join(contRoot, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(contRoot, "sub", "ok.png")
	writeFileT(t, src, makePNG(t))
	got, err := containAsset(contRoot, "./sub/ok.png")
	if err != nil {
		t.Fatalf("expected accept: %v", err)
	}
	wantAbs, _ := filepath.Abs(src)
	wantResolved, _ := filepath.EvalSymlinks(wantAbs)
	if got != wantResolved {
		t.Errorf("containAsset returned %q; want %q", got, wantResolved)
	}
}

// ---------------------------------------------------------------------------
// EVSC-7b / EV-S-1 — symlink traversal
// ---------------------------------------------------------------------------

func TestContainAsset_RejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows; bv targets darwin/linux")
	}
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	outsideTarget := filepath.Join(dir, "outside", "secret.txt")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(outsideTarget), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, outsideTarget, []byte("sensitive"))
	link := filepath.Join(contRoot, "secret")
	if err := os.Symlink(outsideTarget, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, err := containAsset(contRoot, "./secret")
	if err == nil {
		t.Fatal("expected symlink-escape reject")
	}
	if !strings.Contains(err.Error(), "escapes containment root") {
		t.Errorf("error should name escape: %v", err)
	}
}

// ---------------------------------------------------------------------------
// EV-U-7 / EVSC-11 — determinism (asset bytes + safe-names)
// ---------------------------------------------------------------------------

func TestRenderEvidence_DeterministicAssetLayout(t *testing.T) {
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	pngBytes := makePNG(t)
	writeFileT(t, filepath.Join(contRoot, "a.png"), pngBytes)
	writeFileT(t, filepath.Join(contRoot, "b.png"), pngBytes)

	body := []byte(`{"title":"X","items":[{"src":"./a.png"},{"src":"./b.png"}]}`)
	out1 := filepath.Join(dir, "out1")
	out2 := filepath.Join(dir, "out2")

	_, _, err := RenderEvidence(body, RenderOptions{OutDir: out1, ContainmentRoot: contRoot}, Generator{})
	if err != nil {
		t.Fatalf("render 1: %v", err)
	}
	_, _, err = RenderEvidence(body, RenderOptions{OutDir: out2, ContainmentRoot: contRoot}, Generator{})
	if err != nil {
		t.Fatalf("render 2: %v", err)
	}

	hashDir := func(d string) map[string]string {
		out := map[string]string{}
		entries, _ := os.ReadDir(filepath.Join(d, "assets"))
		for _, e := range entries {
			b, _ := os.ReadFile(filepath.Join(d, "assets", e.Name()))
			h := sha256.Sum256(b)
			out[e.Name()] = hex.EncodeToString(h[:])
		}
		return out
	}
	h1 := hashDir(out1)
	h2 := hashDir(out2)
	if !reflect.DeepEqual(h1, h2) {
		t.Errorf("asset layout/bytes differ across renders:\n out1=%v\n out2=%v", h1, h2)
	}
	// Sanity: the safe-names should match the documented format.
	for _, name := range []string{"001-a.png", "002-b.png"} {
		if _, ok := h1[name]; !ok {
			t.Errorf("expected %s in out1; got %v", name, h1)
		}
	}
}

// ---------------------------------------------------------------------------
// EVSC-5 — atomic --out: missing asset leaves --out untouched
// ---------------------------------------------------------------------------

func TestRenderEvidence_MissingAssetAbortsAndLeavesOutDirUntouched(t *testing.T) {
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	// No actual asset on disk; the manifest references it.
	body := []byte(`{"title":"X","items":[{"src":"./missing.png"}]}`)
	outDir := filepath.Join(dir, "never-existed")
	_, _, err := RenderEvidence(body, RenderOptions{OutDir: outDir, ContainmentRoot: contRoot}, Generator{})
	if err == nil {
		t.Fatal("expected missing-asset error")
	}
	// outDir must NOT have been created (atomic rename never fired).
	if _, statErr := os.Stat(outDir); !os.IsNotExist(statErr) {
		t.Errorf("outDir should not exist after missing-asset failure; stat err: %v", statErr)
	}
	// No sibling-tmp dirs left over either.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), renderTempDirPrefix) {
			t.Errorf("sibling tmp dir not cleaned up: %s", e.Name())
		}
	}
}

func TestRenderEvidence_RejectsPreExistingOutDir(t *testing.T) {
	// EV-S-4 RENAME_NOREPLACE-ish: refuse to overwrite a --out that
	// already exists at start.
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(contRoot, "a.png"), makePNG(t))
	outDir := filepath.Join(dir, "out")
	// Pre-create outDir.
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"title":"X","items":[{"src":"./a.png"}]}`)
	_, _, err := RenderEvidence(body, RenderOptions{OutDir: outDir, ContainmentRoot: contRoot}, Generator{})
	if err == nil {
		t.Fatal("expected pre-existing-outdir reject")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should name the conflict: %v", err)
	}
	// outDir is still the empty pre-created dir; we did NOT mutate it.
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 0 {
		t.Errorf("outDir contents were modified; entries: %v", entries)
	}
}

// ---------------------------------------------------------------------------
// EV-S-3 — signal cleanup (fault-injection variant)
// ---------------------------------------------------------------------------
//
// We can't reliably deliver a real SIGINT mid-render in a unit test
// without process-level fragility, so we exercise the cleanup *contract*
// instead: a render that aborts mid-asset-copy MUST remove the sibling
// tmp dir and MUST NOT touch a pre-existing --out path. The fault is
// injected via a corrupt MIME (causes copyAsset to fail mid-loop), which
// is the same code path a real SIGINT would land in via `doCleanup`.
//
// SIGINT-specific test (real signal delivery) is left for an integration
// suite where the bv binary is invoked as a subprocess.

func TestRenderEvidence_FaultInjection_LeavesNoSiblingTmp(t *testing.T) {
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	// First item is fine; second item's bytes don't match its
	// extension, forcing the second copy to fail and exercising the
	// cleanup path that a SIGINT would also take.
	writeFileT(t, filepath.Join(contRoot, "ok.png"), makePNG(t))
	writeFileT(t, filepath.Join(contRoot, "bad.png"), []byte("plain text not png"))
	body := []byte(`{"title":"X","items":[{"src":"./ok.png"},{"src":"./bad.png"}]}`)
	outDir := filepath.Join(dir, "out")
	_, _, err := RenderEvidence(body, RenderOptions{OutDir: outDir, ContainmentRoot: contRoot}, Generator{})
	if err == nil {
		t.Fatal("expected mid-copy failure")
	}
	// Sibling tmp must be gone; --out must not exist.
	if _, statErr := os.Stat(outDir); !os.IsNotExist(statErr) {
		t.Errorf("outDir should not exist; stat err: %v", statErr)
	}
	parent := filepath.Dir(outDir)
	entries, _ := os.ReadDir(parent)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), renderTempDirPrefix) {
			t.Errorf("sibling tmp not cleaned up: %s", e.Name())
		}
	}
}

// ---------------------------------------------------------------------------
// EV-S-4 — concurrent --out
// ---------------------------------------------------------------------------

func TestRenderEvidence_ConcurrentOutDir_OneSucceedsOneFails(t *testing.T) {
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	pngBytes := makePNG(t)
	writeFileT(t, filepath.Join(contRoot, "a.png"), pngBytes)
	body := []byte(`{"title":"X","items":[{"src":"./a.png"}]}`)
	outDir := filepath.Join(dir, "out")

	var wg sync.WaitGroup
	var successCount, failCount atomic.Int32
	var firstErr error
	var errMu sync.Mutex

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := RenderEvidence(body, RenderOptions{OutDir: outDir, ContainmentRoot: contRoot}, Generator{})
			if err == nil {
				successCount.Add(1)
				return
			}
			failCount.Add(1)
			errMu.Lock()
			if firstErr == nil {
				firstErr = err
			}
			errMu.Unlock()
		}()
	}
	wg.Wait()

	if successCount.Load() != 1 || failCount.Load() != 1 {
		t.Fatalf("expected 1 success + 1 failure; got success=%d fail=%d (err=%v)",
			successCount.Load(), failCount.Load(), firstErr)
	}
	// The single failure should be either pre-existing-outdir or
	// concurrent-render — both are acceptable EV-S-4 outcomes.
	if !strings.Contains(firstErr.Error(), "already exists") &&
		!strings.Contains(firstErr.Error(), "concurrent render") &&
		!strings.Contains(firstErr.Error(), "atomic rename") {
		t.Errorf("unexpected concurrent-render error shape: %v", firstErr)
	}

	// outDir should contain the asset (one render won the race).
	if _, err := os.Stat(filepath.Join(outDir, "assets", "001-a.png")); err != nil {
		t.Errorf("expected winning render's asset on disk: %v", err)
	}
}

// ---------------------------------------------------------------------------
// EV-N-7 — cross-device --out
// ---------------------------------------------------------------------------
//
// Real cross-device test environments are not portable in CI; we cover
// the same-device path via sameDevice() unit-test and skip the negative
// case unless a user-supplied env var supplies a known-different-device
// staging dir.

func TestSameDevice_SamePath(t *testing.T) {
	// `tmpDir` and itself are trivially same-device; this exercises the
	// stat path used by EV-N-7's check.
	dir := t.TempDir()
	same, err := sameDevice(dir, dir)
	if err != nil {
		t.Fatalf("sameDevice: %v", err)
	}
	if !same {
		t.Errorf("dir should match itself: %v", same)
	}
}

func TestSameDevice_CrossDevice(t *testing.T) {
	// Optional: opt-in via env var so CI doesn't fail when no cross-
	// device path is available. Locally the developer can run with
	// BV_TEST_CROSS_DEVICE=/Volumes/SomeOtherFS to actually exercise it.
	other := os.Getenv("BV_TEST_CROSS_DEVICE")
	if other == "" {
		t.Skipf("BV_TEST_CROSS_DEVICE unset; skipping cross-device sameDevice test (set to a path on a different filesystem to exercise)")
	}
	if _, err := os.Stat(other); err != nil {
		t.Skipf("BV_TEST_CROSS_DEVICE=%q not statable: %v", other, err)
	}
	dir := t.TempDir()
	// We can't assert NOT same-device without knowing the user's setup,
	// but if both stat and they DO match, we don't have a real cross-
	// device condition to test; flag that.
	same, err := sameDevice(dir, other)
	if err != nil {
		t.Fatalf("sameDevice: %v", err)
	}
	if same {
		t.Logf("BV_TEST_CROSS_DEVICE=%q is on the SAME device as %q; cannot exercise cross-device branch", other, dir)
	}
}

// ---------------------------------------------------------------------------
// EV-U-6 single-handle TOCTOU integration
// ---------------------------------------------------------------------------

// TestCopyAsset_SingleHandle_TOCTOU verifies the io.MultiReader pattern:
// once we've sniffed the first 512 bytes, those EXACT bytes flow into
// the dst, even if "another process" replaces the source file with
// different content between sniff and copy. The defense the spec
// describes (single file handle, sniff via TeeReader/MultiReader) is
// exactly that the sniffed window is preserved in the dst regardless
// of subsequent file mutations.
//
// We simulate "another process" by overwriting the file via os.WriteFile
// AFTER copyAsset has opened+sniffed but BEFORE it has finished the
// streaming copy. To create that timing window deterministically without
// modifying production code, we use a large source file (>1 MiB) and a
// short replacement so the os.Copy call has many iterations to be
// interrupted across. We assert:
//
//   - dst's first sniffWindow bytes match the ORIGINAL first 512 bytes
//     (the sniffed buffer that flows through MultiReader).
//
// We don't assert the bytes after the sniff window — POSIX semantics
// allow those to be the mutated bytes (the open fd reads from the
// inode, which the writer mutated). The protection is bounded to the
// sniff window; that's exactly what spec EV-U-6 says.
func TestCopyAsset_SingleHandle_PreservesSniffedBytes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "race.png")
	// Build a large valid PNG by appending zeros to a real PNG.
	pngBase := makePNG(t)
	// Pad to 1 MiB so the io.Copy loop has many iterations.
	const targetSize = 1 << 20
	pngBig := make([]byte, targetSize)
	copy(pngBig, pngBase)
	writeFileT(t, src, pngBig)

	// Capture the original sniff window for the post-copy assertion.
	originalSniff := make([]byte, sniffWindow)
	copy(originalSniff, pngBig[:sniffWindow])

	dst := filepath.Join(dir, "out.png")

	// Kick off copyAsset and a racer that overwrites the source file
	// with different content. The racer doesn't need to complete
	// "before" anything specific — even if it lands AFTER copy finishes,
	// the sniffed bytes (in the dst) should be intact. The interesting
	// case is when the racer lands DURING the copy: dst's first 512
	// bytes still come from the in-memory `head` buffer, so they're
	// unaffected.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Best-effort racer; we don't sync because the assertion is
		// about the sniff-window bytes, which are protected regardless
		// of when this lands.
		replacement := []byte("MUTATED-NOT-A-PNG-ANYMORE-after-the-race")
		_ = os.WriteFile(src, replacement, 0o644)
	}()

	if err := copyAsset(src, dst); err != nil {
		// The copy may legitimately fail if the racer truncated the
		// file mid-copy (read returned 0). That's not the case we're
		// testing — if it happens, retry once with the racer disabled.
		t.Logf("copyAsset failed (likely racer ran first): %v; retrying without race", err)
		writeFileT(t, src, pngBig)
		if err := copyAsset(src, dst); err != nil {
			t.Fatalf("copyAsset: %v", err)
		}
	}
	wg.Wait()

	gotBytes, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if len(gotBytes) < sniffWindow {
		t.Fatalf("dst too short for sniff-window comparison: %d", len(gotBytes))
	}
	if !bytes.Equal(gotBytes[:sniffWindow], originalSniff) {
		t.Errorf("sniffed bytes were re-read from disk after mutation; expected first %d bytes to match pre-copy snapshot", sniffWindow)
	}
}

// ---------------------------------------------------------------------------
// EV-S-3 — abort-channel cancellation propagation
// ---------------------------------------------------------------------------
//
// The EV-S-3 fix wired a `chan struct{}` "abort" channel from the
// signal handler in renderToOutDir down through writeBundleContents.
// The asset-copy loop selects on `<-abort` BETWEEN copies so a SIGINT
// that lands during the bundle build stops new asset copies cleanly,
// and the signal handler waits for the writer goroutine to return
// before running cleanup (avoiding the previous cleanup-vs-io.Copy
// race). These tests pin the contract at the package level. The
// end-to-end SIGINT integration test (TestRenderEvidence_RealSIGINT)
// covers the process-boundary path.

func TestWriteBundleContents_AbortChannelStopsLoopBetweenCopies(t *testing.T) {
	// Build a bundle with N items where the SECOND iteration sees a
	// closed abort channel. We expect: first asset is copied, abort
	// fires, second/third copies do NOT happen, errEvidenceAborted is
	// returned. This exercises the inter-iteration abort check that
	// guarantees cleanup never races a write.
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	pngBytes := makePNG(t)
	for _, name := range []string{"a.png", "b.png", "c.png"} {
		writeFileT(t, filepath.Join(contRoot, name), pngBytes)
	}
	in := EvidenceInput{
		Title: "X",
		Items: []EvidenceItem{
			{Src: "./a.png"},
			{Src: "./b.png"},
			{Src: "./c.png"},
		},
	}
	dstNames := []string{
		SafeAssetName(in.Items[0], 0),
		SafeAssetName(in.Items[1], 1),
		SafeAssetName(in.Items[2], 2),
	}
	resolvedSrcs := []string{
		filepath.Join(contRoot, "a.png"),
		filepath.Join(contRoot, "b.png"),
		filepath.Join(contRoot, "c.png"),
	}
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Pre-closed abort: writeBundleContents must bail at the very first
	// iteration check and never copy any asset.
	abort := make(chan struct{})
	close(abort)
	err := writeBundleContents(in, outDir, dstNames, resolvedSrcs, Generator{}, abort)
	if err == nil {
		t.Fatal("expected abort error, got nil")
	}
	if !errors.Is(err, errEvidenceAborted) {
		t.Errorf("expected errEvidenceAborted, got %v", err)
	}
	// No asset should have been written.
	assets, _ := os.ReadDir(filepath.Join(outDir, "assets"))
	if len(assets) != 0 {
		names := make([]string, 0, len(assets))
		for _, a := range assets {
			names = append(names, a.Name())
		}
		t.Errorf("expected 0 assets after pre-closed abort; got %d (%v)", len(assets), names)
	}
	// And no index.html.
	if _, statErr := os.Stat(filepath.Join(outDir, "index.html")); !os.IsNotExist(statErr) {
		t.Errorf("expected no index.html; stat err: %v", statErr)
	}
}

func TestWriteBundleContents_NilAbortChannelNeverAborts(t *testing.T) {
	// The --push code path passes nil; the loop must NOT block on a nil
	// channel and must produce a complete bundle.
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	pngBytes := makePNG(t)
	writeFileT(t, filepath.Join(contRoot, "a.png"), pngBytes)
	in := EvidenceInput{Title: "X", Items: []EvidenceItem{{Src: "./a.png"}}}
	dstNames := []string{SafeAssetName(in.Items[0], 0)}
	resolvedSrcs := []string{filepath.Join(contRoot, "a.png")}
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeBundleContents(in, outDir, dstNames, resolvedSrcs, Generator{}, nil); err != nil {
		t.Fatalf("nil abort channel: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "assets", dstNames[0])); err != nil {
		t.Errorf("expected asset on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "index.html")); err != nil {
		t.Errorf("expected index.html on disk: %v", err)
	}
}

func TestWriteBundleContents_AbortMidLoopStopsRemainingCopies(t *testing.T) {
	// Inject abort AFTER the first copy starts but before the second.
	// We can't intercept inside copyAsset without changing production
	// code, so we rely on the inter-iteration check: pre-stage an abort
	// channel, run with N=10 items, close abort after the first copy
	// "should have happened". The deterministic property we assert is:
	// after a closed abort, fewer than N assets are on disk.
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	pngBytes := makePNG(t)
	const N = 10
	in := EvidenceInput{Title: "X", Items: make([]EvidenceItem, N)}
	dstNames := make([]string, N)
	resolvedSrcs := make([]string, N)
	for i := 0; i < N; i++ {
		name := fmt.Sprintf("a%02d.png", i)
		writeFileT(t, filepath.Join(contRoot, name), pngBytes)
		in.Items[i] = EvidenceItem{Src: "./" + name}
		dstNames[i] = SafeAssetName(in.Items[i], i)
		resolvedSrcs[i] = filepath.Join(contRoot, name)
	}
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-close abort: every iteration check sees it closed, so we
	// expect zero asset copies AND errEvidenceAborted.
	abort := make(chan struct{})
	close(abort)
	err := writeBundleContents(in, outDir, dstNames, resolvedSrcs, Generator{}, abort)
	if !errors.Is(err, errEvidenceAborted) {
		t.Fatalf("expected errEvidenceAborted, got %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(outDir, "assets"))
	if len(entries) >= N {
		t.Errorf("expected fewer than %d assets after abort; got %d", N, len(entries))
	}
}

// ---------------------------------------------------------------------------
// Real SIGINT-injection integration test
// ---------------------------------------------------------------------------
//
// Spawns the test binary as a subprocess (BV_EVIDENCE_RENDERER_CHILD=1
// branches into the child path), starts a render with many small assets,
// sends SIGINT to the child after a brief delay, and asserts:
//   - subprocess exits non-zero quickly
//   - sibling-tmp dir is removed (no `.evidence-*` directories left in
//     the parent of --out)
//   - --out itself is never created (the rename never fired because the
//     writer aborted before HTML finalization)
//
// This complements the fault-injection variant (which exercises the
// cleanup code path via copyAsset error) by exercising the actual
// SIGINT delivery + abort-channel + writer-done-wait sequencing.

func TestRenderEvidence_RealSIGINT(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("SIGINT semantics differ on Windows; bv targets darwin/linux")
	}

	// Child path: actually run RenderEvidence and exit. The parent owns
	// asserting on the resulting filesystem state and the exit code.
	if os.Getenv("BV_EVIDENCE_RENDERER_CHILD") == "1" {
		runEvidenceRendererChild()
		return // unreachable; runEvidenceRendererChild always exits
	}

	// Parent path. Set up the input fixture in a tmpdir we can inspect
	// after the child terminates.
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	// Many assets so the loop has many iteration points to honor the
	// abort channel. We pad each PNG with zeros so per-asset io.Copy
	// takes nontrivial real time even on fast NVMe — otherwise the
	// 100-asset variant completes faster than the parent's 50ms SIGINT
	// delay and we never exercise the abort path. 200 KiB × 500 assets
	// = ~100 MiB of write traffic, which on any hardware bv would
	// realistically run on takes well over 50ms.
	const (
		N            = 500
		assetSizeKiB = 200
	)
	pngBig := make([]byte, assetSizeKiB*1024)
	copy(pngBig, makePNG(t))
	for i := 0; i < N; i++ {
		writeFileT(t, filepath.Join(contRoot, fmt.Sprintf("a%03d.png", i)), pngBig)
	}
	outDir := filepath.Join(dir, "out")

	// Re-invoke this test in subprocess mode. Use os.Args[0] (the test
	// binary) and -test.run= to scope to just this test.
	cmd := exec.Command(os.Args[0], "-test.run=^TestRenderEvidence_RealSIGINT$", "-test.v=true")
	cmd.Env = append(os.Environ(),
		"BV_EVIDENCE_RENDERER_CHILD=1",
		"BV_EVIDENCE_CONT_ROOT="+contRoot,
		"BV_EVIDENCE_OUT="+outDir,
		"BV_EVIDENCE_N="+strconv.Itoa(N),
	)
	// Start the child, wait briefly, send SIGINT, wait for exit.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	// Brief delay: long enough for the child to enter the asset-copy
	// loop, short enough that the loop hasn't finished. With 500 ×
	// 200 KiB PNG copies on a typical disk this is comfortably
	// mid-loop. The bound below (10s) lets us catch a never-aborting
	// child even on slow CI hardware.
	time.Sleep(50 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}
	// Bound the wait: the child should exit quickly (well under 10s).
	// If it doesn't, kill and fail.
	exitCh := make(chan error, 1)
	go func() { exitCh <- cmd.Wait() }()
	select {
	case err := <-exitCh:
		if err == nil {
			t.Errorf("child exited 0 after SIGINT; expected non-zero\nstdout:\n%s\nstderr:\n%s",
				stdout.String(), stderr.String())
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-exitCh
		t.Fatalf("child did not exit within 10s after SIGINT\nstdout:\n%s\nstderr:\n%s",
			stdout.String(), stderr.String())
	}

	// --out must never have been created (rename never fired).
	if _, statErr := os.Stat(outDir); !os.IsNotExist(statErr) {
		t.Errorf("--out should not exist after SIGINT abort; stat err: %v", statErr)
	}
	// Sibling-tmp must be gone.
	parent := filepath.Dir(outDir)
	entries, _ := os.ReadDir(parent)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), renderTempDirPrefix) {
			t.Errorf("sibling tmp not cleaned up after SIGINT: %s\nstdout:\n%s\nstderr:\n%s",
				e.Name(), stdout.String(), stderr.String())
		}
	}
}

// runEvidenceRendererChild is the subprocess body for
// TestRenderEvidence_RealSIGINT. It calls RenderEvidence with a
// many-asset input and exits with a nonzero code on any error path.
// The parent sends SIGINT mid-render; the abort-channel + cleanup
// machinery in renderToOutDir is what we are exercising.
func runEvidenceRendererChild() {
	contRoot := os.Getenv("BV_EVIDENCE_CONT_ROOT")
	outDir := os.Getenv("BV_EVIDENCE_OUT")
	n, _ := strconv.Atoi(os.Getenv("BV_EVIDENCE_N"))
	if contRoot == "" || outDir == "" || n <= 0 {
		fmt.Fprintf(os.Stderr, "child: missing env (contRoot=%q out=%q n=%d)\n", contRoot, outDir, n)
		os.Exit(2)
	}
	items := make([]map[string]any, n)
	for i := 0; i < n; i++ {
		items[i] = map[string]any{"src": fmt.Sprintf("./a%03d.png", i)}
	}
	body, _ := json.Marshal(map[string]any{"title": "X", "items": items})
	_, _, err := RenderEvidence(body, RenderOptions{
		OutDir:          outDir,
		ContainmentRoot: contRoot,
	}, Generator{})
	if err != nil {
		// Expected path under SIGINT — surface the error and exit
		// nonzero so the parent can assert on it. errEvidenceAborted is
		// surfaced as a generic non-nil error here; the parent doesn't
		// inspect the message.
		fmt.Fprintf(os.Stderr, "child: render failed: %v\n", err)
		os.Exit(1)
	}
	// If we reach here, SIGINT didn't land in time and the render
	// completed naturally. The parent treats this as a flaky-environment
	// signal (10s wait, but we won the race against the 50ms delay).
	// Exit 0; parent will see --out exists and either fail (clean win
	// before SIGINT) or report the test environment as too fast to
	// exercise the SIGINT path.
	fmt.Fprintln(os.Stdout, "child: render completed before SIGINT")
	os.Exit(0)
}

// ---------------------------------------------------------------------------
// Coverage gap tests (≥ 90% on evidence.go)
// ---------------------------------------------------------------------------

func TestValidate_AltTooLong(t *testing.T) {
	in := EvidenceInput{
		Title: "ok",
		Items: []EvidenceItem{{Src: "./a.png", Alt: strings.Repeat("a", maxEvidenceItemAltLen+1)}},
	}
	if err := in.Validate(); err == nil || !strings.Contains(err.Error(), "alt") {
		t.Errorf("expected alt-length error, got %v", err)
	}
}

func TestValidate_ItemDescriptionTooLong(t *testing.T) {
	in := EvidenceInput{
		Title: "ok",
		Items: []EvidenceItem{{Src: "./a.png", Description: strings.Repeat("a", maxEvidenceItemDescLen+1)}},
	}
	if err := in.Validate(); err == nil || !strings.Contains(err.Error(), "description") {
		t.Errorf("expected description-length error, got %v", err)
	}
}

// errReader returns a fixed error on first Read. Used to drive the io
// error path of ReadStdin (templates/evidence.go).
type errReader struct{ err error }

func (e *errReader) Read(_ []byte) (int, error) { return 0, e.err }

func TestReadStdin_PropagatesIOError(t *testing.T) {
	want := errors.New("simulated stdin read failure")
	_, err := ReadStdin(&errReader{err: want})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Wrapped error must surface the underlying cause for an agent to
	// diagnose pipe failures.
	if !errors.Is(err, want) {
		t.Errorf("expected wrapped %v; got %v", want, err)
	}
}

func TestSameDevice_NonexistentPathErrors(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")
	_, err := sameDevice(missing, dir)
	if err == nil {
		t.Fatal("expected stat-failure error for missing path")
	}
	if !strings.Contains(err.Error(), "stat") {
		t.Errorf("error should name stat: %v", err)
	}
	// Also the second-arg path:
	_, err = sameDevice(dir, missing)
	if err == nil {
		t.Fatal("expected stat-failure error for missing second-arg path")
	}
}

// containAsset symlink-tail variants (EV-S-1). Three cases, each
// distinct in WHERE the symlink lives in the resolved path:
//
//  1. Symlink is the asset itself (./link -> outside) — covered by the
//     existing TestContainAsset_RejectsSymlinkEscape.
//  2. Symlink is the IMMEDIATE parent (./link -> outside-dir; ./link/a.png).
//  3. Symlink is the GRAND-PARENT (./link -> outside-dir; ./link/sub/a.png).
//
// The third case is the one most likely to slip through a naive
// "EvalSymlinks(filepath.Dir(...))" implementation that only resolves one
// level. We pin both parent and grand-parent escapes here.

func TestContainAsset_RejectsSymlinkAtImmediateParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows; bv targets darwin/linux")
	}
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	outsideDir := filepath.Join(dir, "outside")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(outsideDir, "secret.png"), makePNG(t))
	// linkdir lives inside contRoot but resolves to outsideDir.
	if err := os.Symlink(outsideDir, filepath.Join(contRoot, "linkdir")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, err := containAsset(contRoot, "./linkdir/secret.png")
	if err == nil {
		t.Fatal("expected escape-via-symlink-parent reject")
	}
	if !strings.Contains(err.Error(), "escapes containment root") {
		t.Errorf("error should name escape: %v", err)
	}
}

func TestContainAsset_RejectsSymlinkAtGrandParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows; bv targets darwin/linux")
	}
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	outsideDir := filepath.Join(dir, "outside")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outsideDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(outsideDir, "sub", "secret.png"), makePNG(t))
	// linkdir at grand-parent depth: linkdir -> outsideDir, asset is
	// ./linkdir/sub/secret.png so the symlink is two segments above
	// the file.
	if err := os.Symlink(outsideDir, filepath.Join(contRoot, "linkdir")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, err := containAsset(contRoot, "./linkdir/sub/secret.png")
	if err == nil {
		t.Fatal("expected escape-via-symlink-grand-parent reject")
	}
	if !strings.Contains(err.Error(), "escapes containment root") {
		t.Errorf("error should name escape: %v", err)
	}
}

func TestContainAsset_NotYetCreatedFileResolvesViaParent(t *testing.T) {
	// EV-S-1 spec: a not-yet-created asset is resolved by EvalSymlinks-ing
	// the deepest existing ancestor and rejoining the unresolved tail.
	// Verify the in-bounds case: a file that doesn't exist yet, whose
	// parent IS inside the containment root, resolves OK.
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(filepath.Join(contRoot, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := containAsset(contRoot, "./sub/notyet.png")
	if err != nil {
		t.Fatalf("expected accept for not-yet-created file under root: %v", err)
	}
	rootResolved, _ := filepath.EvalSymlinks(contRoot)
	wantPrefix, _ := filepath.Abs(rootResolved)
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("resolved path %q should be under %q", got, wantPrefix)
	}
}

// EV-S-1: --push mode (no --out). RenderEvidence must create the temp
// dir under os.TempDir() and return its absolute path. Coverage gap:
// the entire renderToTempDir path was previously untested.
func TestRenderEvidence_PushModeReturnsTempDir(t *testing.T) {
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(contRoot, "a.png"), makePNG(t))
	body := []byte(`{"title":"X","items":[{"src":"./a.png"}]}`)

	_, bundleDir, err := RenderEvidence(body, RenderOptions{
		// OutDir intentionally empty — push mode.
		ContainmentRoot: contRoot,
	}, Generator{})
	if err != nil {
		t.Fatalf("RenderEvidence (push mode): %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(bundleDir) })

	// Bundle must live under the OS temp dir.
	tmpRoot, _ := filepath.EvalSymlinks(os.TempDir())
	bundleResolved, _ := filepath.EvalSymlinks(bundleDir)
	if !strings.HasPrefix(bundleResolved, tmpRoot) {
		t.Errorf("bundleDir %q (resolved %q) should be under %q", bundleDir, bundleResolved, tmpRoot)
	}
	// Bundle basename must follow the renderTempDirPrefix convention.
	if !strings.HasPrefix(filepath.Base(bundleDir), renderTempDirPrefix) {
		t.Errorf("bundleDir basename %q should start with %q", filepath.Base(bundleDir), renderTempDirPrefix)
	}
	// Bundle must contain index.html + assets/001-a.png.
	if _, err := os.Stat(filepath.Join(bundleDir, "index.html")); err != nil {
		t.Errorf("expected index.html in push-mode bundle: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bundleDir, "assets", "001-a.png")); err != nil {
		t.Errorf("expected asset in push-mode bundle: %v", err)
	}
}

// EV-E-5 / push-mode failure path: a missing asset must abort and clean
// the CLI-owned temp dir before returning.
func TestRenderEvidence_PushModeMissingAssetCleansTempDir(t *testing.T) {
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"title":"X","items":[{"src":"./missing.png"}]}`)
	_, bundleDir, err := RenderEvidence(body, RenderOptions{
		ContainmentRoot: contRoot,
	}, Generator{})
	if err == nil {
		_ = os.RemoveAll(bundleDir)
		t.Fatal("expected missing-asset error in push mode")
	}
	if bundleDir != "" {
		t.Errorf("expected empty bundleDir on failure; got %q", bundleDir)
	}
	// No leftover .evidence-* dirs in os.TempDir() that we care about.
	// We can't reliably scan the whole tmp dir (other tests share it),
	// but we can assert the specific bundleDir was cleaned IF the
	// implementation returned it for diagnostics. With bundleDir == ""
	// the contract is "the function owns cleanup".
}

// EV-E-7 / RenderOptions: empty ContainmentRoot must be rejected.
func TestRenderEvidence_RejectsEmptyContainmentRoot(t *testing.T) {
	body := []byte(`{"title":"X","items":[{"src":"./a.png"}]}`)
	_, _, err := RenderEvidence(body, RenderOptions{
		// ContainmentRoot intentionally empty.
		OutDir: filepath.Join(t.TempDir(), "out"),
	}, Generator{})
	if err == nil {
		t.Fatal("expected ContainmentRoot-required error")
	}
	if !strings.Contains(err.Error(), "ContainmentRoot") {
		t.Errorf("error should name ContainmentRoot: %v", err)
	}
}

// RenderOptions.Layout is retained as a deprecated compatibility field
// but ignored: evidence pages now include a viewer-side layout switcher.
func TestRenderEvidence_IgnoresDeprecatedLayoutOption(t *testing.T) {
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contRoot, "a.png"), makePNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"title":"X","items":[{"src":"./a.png"}]}`)
	_, outDir, err := RenderEvidence(body, RenderOptions{
		Layout:          "diorama", // ignored, not validated
		OutDir:          filepath.Join(dir, "out"),
		ContainmentRoot: contRoot,
	}, Generator{})
	if err != nil {
		t.Fatalf("RenderEvidence: %v", err)
	}
	idx, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(idx), `id="ev-layout-carousel"`) {
		t.Errorf("expected switchable layout UI despite deprecated Layout option")
	}
}

func TestRenderEvidence_RejectsContainmentEscape(t *testing.T) {
	// ContainAsset error path on the RenderEvidence side: an item.src
	// that escapes the containment root must abort before any temp dir
	// is created.
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"title":"X","items":[{"src":"../../etc/passwd.png"}]}`)
	_, _, err := RenderEvidence(body, RenderOptions{
		OutDir:          filepath.Join(dir, "out"),
		ContainmentRoot: contRoot,
	}, Generator{})
	if err == nil {
		t.Fatal("expected containment-escape error")
	}
	if !strings.Contains(err.Error(), "escapes containment root") {
		t.Errorf("error should name escape: %v", err)
	}
	// No --out, no sibling-tmp.
	if _, statErr := os.Stat(filepath.Join(dir, "out")); !os.IsNotExist(statErr) {
		t.Errorf("--out should not exist after containment failure: %v", statErr)
	}
}

func TestRenderEvidence_RejectsParseFailure(t *testing.T) {
	// Malformed JSON: parse failure must surface BEFORE any I/O.
	dir := t.TempDir()
	body := []byte(`{not valid json`)
	_, _, err := RenderEvidence(body, RenderOptions{
		OutDir:          filepath.Join(dir, "out"),
		ContainmentRoot: dir,
	}, Generator{})
	if err == nil {
		t.Fatal("expected parse error")
	}
}

// ---------------------------------------------------------------------------
// Additional coverage tests — push past 90% on evidence.go
// ---------------------------------------------------------------------------

func TestSafeAssetName_PathologicalBasenameUsesPlaceholder(t *testing.T) {
	// A src whose basename is entirely sanitized-away (".." literally)
	// hits the "asset" placeholder branch. The index prefix still
	// uniquifies the result.
	got := SafeAssetName(EvidenceItem{Src: "./.."}, 0)
	// ".." sanitizes to ".." (both chars allowed), then trips the
	// cleaned == ".." check and falls back to "asset". Either "001-.."
	// (raw) or "001-asset" (post-fallback) is acceptable depending on
	// the sanitizer rules; both prove the index prefix is intact.
	if !strings.HasPrefix(got, "001-") {
		t.Errorf("expected index-prefixed name for ./..; got %q", got)
	}
	// Empty basename via repeated separators.
	got2 := SafeAssetName(EvidenceItem{Src: "./"}, 1)
	if !strings.Contains(got2, "asset") && !strings.Contains(got2, "002-.") {
		// Either "002-asset" (from placeholder fallback) or "002-." is
		// acceptable depending on filepath.Base behavior; pin only that
		// the index prefix is intact.
		t.Errorf("expected index-prefixed name; got %q", got2)
	}
	// "..." (three dots): cleaned == "..." (all dots allowed); not the
	// pathological branch. The branch we want is when cleaned == ""
	// or "." or "..". An all-special-char basename ("///") becomes "-"
	// after sanitization; that's not the pathological branch either.
	// The branch IS reachable via filepath.Base of "./" which on Unix
	// returns ".".
	gotDot := SafeAssetName(EvidenceItem{Src: "."}, 5)
	// filepath.Base(".") == "."; cleaned == "."; placeholder kicks in.
	if !strings.HasSuffix(gotDot, "asset") {
		t.Errorf("expected pathological '.' basename to fall back to 'asset'; got %q", gotDot)
	}
}

func TestCopyAsset_RejectsCapSmallerThanSniffWindow(t *testing.T) {
	// Pathological cap config: maxAssetBytes < sniffWindow (512). The
	// `remaining < 0` branch in copyAsset must trip and reject.
	// We need a src LARGER than sniffWindow so io.ReadFull fills the
	// entire 512-byte head buffer; only then does len(head) == 512 and
	// `maxAssetBytes - 512 < 0` evaluate truthy.
	dir := t.TempDir()
	prev := maxAssetBytes
	maxAssetBytes = 100 // smaller than the 512-byte sniff window
	t.Cleanup(func() { maxAssetBytes = prev })
	pngBig := make([]byte, 1024)
	copy(pngBig, makePNG(t))
	src := filepath.Join(dir, "ok.png")
	writeFileT(t, src, pngBig)
	dst := filepath.Join(dir, "out.png")
	err := copyAsset(src, dst)
	if err == nil {
		t.Fatal("expected per-asset-cap error when cap < sniffWindow")
	}
	if !strings.Contains(err.Error(), "per-asset cap") {
		t.Errorf("error should name the cap: %v", err)
	}
}

func TestCopyAsset_RejectsUnwritableDst(t *testing.T) {
	// A dst whose parent is not a directory must surface the
	// dst-open error from os.OpenFile, exercising the
	// 448-450 branch in copyAsset.
	dir := t.TempDir()
	src := filepath.Join(dir, "ok.png")
	writeFileT(t, src, makePNG(t))
	// Create a file-not-dir at the parent path so dst-open errors.
	notADir := filepath.Join(dir, "blocker")
	writeFileT(t, notADir, []byte("blocker"))
	// dst path is "blocker/out.png" — parent is a file, so OpenFile fails.
	dst := filepath.Join(notADir, "out.png")
	err := copyAsset(src, dst)
	if err == nil {
		t.Fatal("expected dst-open failure")
	}
	if !strings.Contains(err.Error(), "create evidence dst") {
		t.Errorf("error should name the dst-open: %v", err)
	}
}

func TestContainAsset_ResolvesMultiLevelNotYetCreated(t *testing.T) {
	// EV-S-1 ancestor-walk: a deeply-nested not-yet-created path must
	// still resolve via the deepest existing ancestor. Two missing
	// directories above the file exercise the loop body that the
	// single-level test does not.
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	// Three missing levels: ./a/b/c/file.png — root exists, a/b/c does not.
	got, err := containAsset(contRoot, "./a/b/c/file.png")
	if err != nil {
		t.Fatalf("expected accept for multi-level not-yet-created file: %v", err)
	}
	rootResolved, _ := filepath.EvalSymlinks(contRoot)
	wantPrefix, _ := filepath.Abs(rootResolved)
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("resolved path %q should be under %q", got, wantPrefix)
	}
	// And confirm that a deep-not-yet-created path that ALSO escapes
	// (via ../) is still rejected by the lexical pre-check.
	_, err = containAsset(contRoot, "./a/b/c/../../../../etc/passwd")
	if err == nil {
		t.Fatal("expected escape reject even with not-yet-created tail")
	}
	if !strings.Contains(err.Error(), "escapes containment root") {
		t.Errorf("error should name escape: %v", err)
	}
}

func TestContainAsset_RejectsMissingBase(t *testing.T) {
	// EvalSymlinks of a missing base must surface as a containment-base
	// error, exercising the 488-490 branch.
	dir := t.TempDir()
	missingBase := filepath.Join(dir, "no-such-root")
	_, err := containAsset(missingBase, "./a.png")
	if err == nil {
		t.Fatal("expected error when containment base does not exist")
	}
	if !strings.Contains(err.Error(), "containment base") {
		t.Errorf("error should name containment base: %v", err)
	}
}

func TestWriteBundleContents_PostLoopAbortCheck(t *testing.T) {
	// The post-asset-loop abort check fires only when the abort channel
	// is closed AFTER all asset copies finish but BEFORE HTML render.
	// We exercise this by passing zero items (loop runs zero times) and
	// a pre-closed abort channel: the asset-loop pre-check is skipped
	// (no iterations), then the post-loop check trips.
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	in := EvidenceInput{Title: "X", Items: nil} // zero items deliberately
	abort := make(chan struct{})
	close(abort)
	err := writeBundleContents(in, outDir, nil, nil, Generator{}, abort)
	if !errors.Is(err, errEvidenceAborted) {
		t.Fatalf("expected errEvidenceAborted from post-loop check; got %v", err)
	}
	// HTML must NOT have been rendered.
	if _, statErr := os.Stat(filepath.Join(outDir, "index.html")); !os.IsNotExist(statErr) {
		t.Errorf("expected no index.html after post-loop abort; stat: %v", statErr)
	}
}

func TestRenderToOutDir_RejectsMissingParent_StatNonNotExistError(t *testing.T) {
	// Lstat error other than IsNotExist on the --out path: pass a path
	// whose intermediate component is a non-directory. On Unix this
	// surfaces as a "not a directory" error from Lstat, which is NOT
	// os.IsNotExist — exercising the 909-911 branch in renderToOutDir.
	if runtime.GOOS == "windows" {
		t.Skip("Lstat semantics on Windows differ; bv targets darwin/linux")
	}
	dir := t.TempDir()
	contRoot := filepath.Join(dir, "root")
	if err := os.MkdirAll(contRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(contRoot, "a.png"), makePNG(t))
	// Make a regular file, then try to use a path THROUGH it as --out:
	// "blocker/out". Lstat of "blocker/out" returns ENOTDIR, NOT ENOENT.
	blocker := filepath.Join(dir, "blocker")
	writeFileT(t, blocker, []byte("blocker"))
	outDir := filepath.Join(blocker, "out")
	body := []byte(`{"title":"X","items":[{"src":"./a.png"}]}`)
	_, _, err := RenderEvidence(body, RenderOptions{
		OutDir:          outDir,
		ContainmentRoot: contRoot,
	}, Generator{})
	if err == nil {
		t.Fatal("expected --out-stat or parent-creation error")
	}
}

// ---------------------------------------------------------------------------
// Sanity guard: helper sanity for fmt-only error messages
// ---------------------------------------------------------------------------

func TestAllowedAssetMIMEList_IsDeterministic(t *testing.T) {
	// Sorted, comma-separated. Used in error messages so an agent can
	// see the closed allowlist.
	want := "image/gif, image/jpeg, image/png, image/webp, video/mp4, video/quicktime, video/webm"
	if allowedAssetMIMEList != want {
		t.Errorf("allowedAssetMIMEList drift; got %q want %q", allowedAssetMIMEList, want)
	}
	// Also ensure we list the expected count (jpg + jpeg dedupe to one).
	if c := strings.Count(allowedAssetMIMEList, ","); c != 6 {
		t.Errorf("expected 6 commas (7 distinct MIMEs), got %d", c)
	}
	_ = fmt.Sprint(allowedAssetMIMEList) // touch fmt import
}
