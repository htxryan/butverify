// Evidence template: parses a JSON manifest describing a sequence of
// screenshot/video assets and (in T3/T4) renders a static gallery
// microsite. This file is the FOUNDATION layer (E12-T2): types, strict
// parser, semantic validation, hand-authored JSON Schema, stable-sort
// ordering, and the stdin 4 MiB cap. Asset MIME sniffing, atomic
// `--out` rename, asset copy, and HTML rendering land in T3/T4 and
// MUST NOT appear here.
//
// Authoritative spec: docs/specs/evidence-template.md (rev 5).
//
// Conventions mirror report.go in this package:
//   - ParseEvidence([]byte) — strict parse via json.Decoder with
//     DisallowUnknownFields(); descriptive errors quote the offending
//     JSON field path.
//   - (in *EvidenceInput) Validate() — semantic checks (lengths, item
//     count, src-scheme rejection, alt rules).
//   - SortItems is exposed so the eventual renderer (T4) can call into
//     a single, tested sort implementation that matches EVSC-10.

package templates

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
)

// EvidenceInput is the top-level JSON contract for `bv evidence`. Keep
// this in sync with docs/specs/evidence-template.md §5 and with the
// hand-authored EvidenceSchema below — schema/parser parity is asserted
// by a unit test (EV-U-11).
type EvidenceInput struct {
	Title    string           `json:"title"`
	Subtitle string           `json:"subtitle,omitempty"`
	Summary  string           `json:"summary,omitempty"`
	Metadata EvidenceMetadata `json:"metadata,omitempty"`
	Items    []EvidenceItem   `json:"items"`
}

// EvidenceItem is one gallery entry. `Sequence` is `*int` (not `int`)
// because the spec distinguishes "no sequence provided" (slot in after
// all sequenced items, preserve JSON-array order) from an explicit
// `sequence: 0` (sort first). A non-pointer int would collapse those
// two cases.
type EvidenceItem struct {
	Src         string           `json:"src"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Sequence    *int             `json:"sequence,omitempty"`
	Alt         string           `json:"alt,omitempty"`
	Metadata    EvidenceMetadata `json:"metadata,omitempty"`
}

// EvidenceMetadata describes the work-management item being evidenced.
// Top-level metadata is for a gallery that proves one issue; per-item
// metadata is for galleries spanning multiple issues.
type EvidenceMetadata struct {
	IssueURL   string `json:"issue_url,omitempty"`
	IssueID    string `json:"issue_id,omitempty"`
	IssueTitle string `json:"issue_title,omitempty"`
}

// Bounds from §5 / EARS §4. These are parse-time caps; bundle-size
// enforcement happens server-side (HTTP 413) and is out of scope here.
const (
	maxEvidenceTitleLen      = 200
	maxEvidenceSubtitleLen   = 300
	maxEvidenceSummaryLen    = 2000
	maxEvidenceItemTitleLen  = 200
	maxEvidenceItemDescLen   = 2000
	maxEvidenceItemAltLen    = 1000 // not in spec table; bounded for safety
	maxEvidenceIssueURLLen   = 2048
	maxEvidenceIssueIDLen    = 200
	maxEvidenceIssueTitleLen = 300
	maxEvidenceItems         = 500     // EV-N-5
	evidenceStdinMaxBytes    = 4 << 20 // EV-N-6: 4 MiB
)

// rejectedSrcSchemes is the closed set of URL schemes that MUST NOT
// appear at the start of a `src` value (EV-U-5). Local relative paths
// are required; remote/data/file references are XSS / SSRF surfaces or
// containment escapes.
var rejectedSrcSchemes = []string{
	"http://",
	"https://",
	"data:",
	"file:",
}

// ParseEvidence strict-parses the JSON contract from input bytes,
// returning a fully validated EvidenceInput. Strict mode (unknown
// fields rejected) matches EV-U-3. Errors quote the offending JSON
// field path so an agent can fix the manifest without binary-searching.
func ParseEvidence(input []byte) (EvidenceInput, error) {
	var in EvidenceInput
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return EvidenceInput{}, fmt.Errorf("templates: parse evidence: %w", err)
	}
	// Reject trailing tokens so `{"a":1}{"b":2}` doesn't silently
	// parse as the first object — the spec is "one JSON document".
	if dec.More() {
		return EvidenceInput{}, errors.New("templates: parse evidence: unexpected trailing data after top-level JSON value")
	}
	if err := in.Validate(); err != nil {
		return EvidenceInput{}, err
	}
	return in, nil
}

// Validate enforces the §5 contract on a parsed EvidenceInput. This is
// intentionally a separate exposed method so a future programmatic
// caller (e.g. `RenderEvidence(EvidenceInput, ...)`) can reuse the
// rules without round-tripping through JSON.
func (in *EvidenceInput) Validate() error {
	if strings.TrimSpace(in.Title) == "" {
		return errors.New("templates: evidence.title is required")
	}
	if len(in.Title) > maxEvidenceTitleLen {
		return fmt.Errorf("templates: evidence.title exceeds %d chars (got %d)", maxEvidenceTitleLen, len(in.Title))
	}
	if len(in.Subtitle) > maxEvidenceSubtitleLen {
		return fmt.Errorf("templates: evidence.subtitle exceeds %d chars (got %d)", maxEvidenceSubtitleLen, len(in.Subtitle))
	}
	if len(in.Summary) > maxEvidenceSummaryLen {
		return fmt.Errorf("templates: evidence.summary exceeds %d chars (got %d)", maxEvidenceSummaryLen, len(in.Summary))
	}
	if err := validateMetadata("templates: evidence.metadata", in.Metadata); err != nil {
		return err
	}
	if len(in.Items) == 0 {
		// EV-N-1: an evidence site with zero items has no purpose.
		return errors.New("templates: evidence.items must contain at least one item")
	}
	if len(in.Items) > maxEvidenceItems {
		// EV-N-5: bound on render-time CPU/memory and HTML page weight.
		return fmt.Errorf("templates: evidence.items exceeds limit (%d > %d)", len(in.Items), maxEvidenceItems)
	}
	for i, it := range in.Items {
		if err := validateItem(i, it); err != nil {
			return err
		}
	}
	return nil
}

func validateItem(i int, it EvidenceItem) error {
	if strings.TrimSpace(it.Src) == "" {
		return fmt.Errorf("templates: evidence.items[%d].src is required", i)
	}
	// EV-U-5: reject URL-scheme refs at parse time. Match
	// case-insensitively; an attacker-controlled "HTTP://" should fail
	// the same way as "http://".
	low := strings.ToLower(it.Src)
	for _, s := range rejectedSrcSchemes {
		if strings.HasPrefix(low, s) {
			return fmt.Errorf(
				"templates: evidence.items[%d].src must be a local relative path; URL schemes (http, https, data, file) are rejected (got %q)",
				i, it.Src,
			)
		}
	}
	if len(it.Title) > maxEvidenceItemTitleLen {
		return fmt.Errorf("templates: evidence.items[%d].title exceeds %d chars (got %d)", i, maxEvidenceItemTitleLen, len(it.Title))
	}
	if len(it.Description) > maxEvidenceItemDescLen {
		return fmt.Errorf("templates: evidence.items[%d].description exceeds %d chars (got %d)", i, maxEvidenceItemDescLen, len(it.Description))
	}
	if len(it.Alt) > maxEvidenceItemAltLen {
		return fmt.Errorf("templates: evidence.items[%d].alt exceeds %d chars (got %d)", i, maxEvidenceItemAltLen, len(it.Alt))
	}
	if err := validateMetadata(fmt.Sprintf("templates: evidence.items[%d].metadata", i), it.Metadata); err != nil {
		return err
	}
	return nil
}

func validateMetadata(path string, meta EvidenceMetadata) error {
	if len(meta.IssueURL) > maxEvidenceIssueURLLen {
		return fmt.Errorf("%s.issue_url exceeds %d chars (got %d)", path, maxEvidenceIssueURLLen, len(meta.IssueURL))
	}
	if len(meta.IssueID) > maxEvidenceIssueIDLen {
		return fmt.Errorf("%s.issue_id exceeds %d chars (got %d)", path, maxEvidenceIssueIDLen, len(meta.IssueID))
	}
	if len(meta.IssueTitle) > maxEvidenceIssueTitleLen {
		return fmt.Errorf("%s.issue_title exceeds %d chars (got %d)", path, maxEvidenceIssueTitleLen, len(meta.IssueTitle))
	}
	if meta.IssueURL == "" {
		return nil
	}
	u, err := url.Parse(meta.IssueURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%s.issue_url must be an absolute http(s) URL (got %q)", path, meta.IssueURL)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%s.issue_url must be an absolute http(s) URL (got %q)", path, meta.IssueURL)
	}
	return nil
}

// SortItems returns a stable-sorted copy of items per EVSC-10:
//
//   - Items with sequence set come first, ascending by sequence.
//   - Items without sequence preserve their original JSON-array order
//     and slot in AFTER all sequenced items.
//   - Equal sequences preserve JSON-array order (stable).
//
// The renderer (T4) calls this; tests assert the ordering directly so
// the contract can't drift if the renderer's loop is rewritten.
func SortItems(items []EvidenceItem) []EvidenceItem {
	// Tag each input position to make the no-sequence tail
	// "JSON-array-order" and the sequence-tied "JSON-array-order" both
	// fall out of a single stable sort.
	type tagged struct {
		idx  int
		item EvidenceItem
	}
	tagged_ := make([]tagged, len(items))
	for i, it := range items {
		tagged_[i] = tagged{idx: i, item: it}
	}
	sort.SliceStable(tagged_, func(a, b int) bool {
		ai := tagged_[a].item.Sequence
		bi := tagged_[b].item.Sequence
		// Sequenced beats unsequenced.
		switch {
		case ai != nil && bi == nil:
			return true
		case ai == nil && bi != nil:
			return false
		case ai == nil && bi == nil:
			// Both unsequenced: preserve JSON-array order.
			return tagged_[a].idx < tagged_[b].idx
		default:
			// Both sequenced: ascending by sequence.
			if *ai != *bi {
				return *ai < *bi
			}
			// Tie: preserve JSON-array order.
			return tagged_[a].idx < tagged_[b].idx
		}
	})
	out := make([]EvidenceItem, len(items))
	for i, t := range tagged_ {
		out[i] = t.item
	}
	return out
}

// ReadStdin pulls the JSON manifest off an io.Reader (typically
// os.Stdin) with the EV-N-6 4 MiB cap. We read up to 4 MiB + 1 byte;
// if we got more than 4 MiB the producer is over the limit and we
// reject with a usage-style error before the parser ever sees the
// payload.
//
// This stays separate from ParseEvidence so callers using `--from
// <path>` (which has its own filesystem-side bound from the OS / the
// 1 GiB asset cap) don't pay the artificial cap.
func ReadStdin(r io.Reader) ([]byte, error) {
	limited := io.LimitReader(r, evidenceStdinMaxBytes+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("templates: read evidence stdin: %w", err)
	}
	if len(buf) > evidenceStdinMaxBytes {
		return nil, fmt.Errorf(
			"templates: evidence stdin payload exceeds %d bytes (4 MiB cap); pass JSON via --from <path> for larger manifests",
			evidenceStdinMaxBytes,
		)
	}
	return buf, nil
}

// EvidenceSchema is the v1.0 hand-authored Draft 2020-12 JSON Schema
// for `bv evidence` input (EV-U-11). The CLI prints this verbatim from
// `bv evidence --schema`. A unit test validates every spec example
// against this schema AND asserts the same payload parses cleanly via
// ParseEvidence; a curated negative-payload set fails both.
//
// v1.x followup: auto-derive from struct tags so the schema and parser
// can't drift. For v1.0 we accept the drift risk and pin parity in the
// test suite.
//
// Notes on choices below:
//   - `additionalProperties: false` on both top-level and item objects
//     mirrors EV-U-3 strict-parse.
//   - `src` uses `pattern` to reject http/https/data/file schemes at
//     the top of the string. We don't try to fully validate the path
//     here — containment is enforced lexically + via EvalSymlinks at
//     render time (EV-S-1, T3 territory).
//   - `src` uses an extension allowlist via a regex anchored at end of
//     string so the schema documents the closed MIME set even though
//     the runtime sniff (EV-U-6) is the actual gate.
//   - `sequence` is `integer` with no min/max — JSON's `integer` is
//     bounded by the consumer; agents producing values outside int32
//     are pathological and the Go parser will fail at decode.
const EvidenceSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://butverify.dev/schemas/evidence/v1.json",
  "title": "butverify.dev evidence manifest",
  "description": "Input contract for ` + "`bv evidence`" + `. See docs/specs/evidence-template.md §5.",
  "type": "object",
  "additionalProperties": false,
  "required": ["title", "items"],
  "properties": {
    "title": {
      "type": "string",
      "minLength": 1,
      "maxLength": 200,
      "description": "Page <title> and the H1 above the gallery."
    },
    "subtitle": {
      "type": "string",
      "maxLength": 300,
      "description": "Single secondary line below the title."
    },
    "summary": {
      "type": "string",
      "maxLength": 2000,
      "description": "Short paragraph above the gallery; rendered with white-space: pre-wrap."
    },
    "metadata": {
      "type": "object",
      "additionalProperties": false,
      "description": "Optional work-management item this evidence site proves. Use this when all evidence items relate to one Jira/Linear/GitHub issue; use item.metadata when individual captures map to different work items.",
      "properties": {
        "issue_url": {
          "type": "string",
          "maxLength": 2048,
          "anyOf": [
            {"maxLength": 0},
            {"pattern": "^[Hh][Tt][Tt][Pp][Ss]?://[^\\s/?#][^\\s]*$"}
          ],
          "description": "Absolute http(s) URL for the work item, such as a Jira issue URL."
        },
        "issue_id": {
          "type": "string",
          "maxLength": 200,
          "description": "Work item identifier or key, such as JIRA-123, ENG-456, or #789."
        },
        "issue_title": {
          "type": "string",
          "maxLength": 300,
          "description": "Work item title/summary from the source system."
        }
      }
    },
    "items": {
      "type": "array",
      "minItems": 1,
      "maxItems": 500,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["src"],
        "properties": {
          "src": {
            "type": "string",
            "minLength": 1,
            "description": "Local relative path. URL schemes (http/https/data/file) are rejected. Extension must be one of png/jpg/jpeg/webp/gif/mp4/webm/mov.",
            "not": {
              "anyOf": [
                {"pattern": "^[Hh][Tt][Tt][Pp]://"},
                {"pattern": "^[Hh][Tt][Tt][Pp][Ss]://"},
                {"pattern": "^[Dd][Aa][Tt][Aa]:"},
                {"pattern": "^[Ff][Ii][Ll][Ee]:"}
              ]
            },
            "pattern": "\\.(?:[Pp][Nn][Gg]|[Jj][Pp][Gg]|[Jj][Pp][Ee][Gg]|[Ww][Ee][Bb][Pp]|[Gg][Ii][Ff]|[Mm][Pp]4|[Ww][Ee][Bb][Mm]|[Mm][Oo][Vv])$"
          },
          "title": {
            "type": "string",
            "maxLength": 200,
            "description": "Heading shown above the asset."
          },
          "description": {
            "type": "string",
            "maxLength": 2000,
            "description": "Body text shown beneath the asset; rendered with white-space: pre-wrap."
          },
          "sequence": {
            "type": "integer",
            "description": "Explicit ordering. Items with sequence set sort ascending; items without sequence preserve JSON-array order and slot in after all sequenced items."
          },
          "alt": {
            "type": "string",
            "maxLength": 1000,
            "description": "Alt text for images. Defaults to the item's title if unset."
          },
          "metadata": {
            "type": "object",
            "additionalProperties": false,
            "description": "Optional work-management item represented by this individual evidence item. Use this when the gallery spans multiple Jira/Linear/GitHub issues or a capture proves a more specific issue than the page-level metadata.",
            "properties": {
              "issue_url": {
                "type": "string",
                "maxLength": 2048,
                "anyOf": [
                  {"maxLength": 0},
                  {"pattern": "^[Hh][Tt][Tt][Pp][Ss]?://[^\\s/?#][^\\s]*$"}
                ],
                "description": "Absolute http(s) URL for the work item, such as a Jira issue URL."
              },
              "issue_id": {
                "type": "string",
                "maxLength": 200,
                "description": "Work item identifier or key, such as JIRA-123, ENG-456, or #789."
              },
              "issue_title": {
                "type": "string",
                "maxLength": 300,
                "description": "Work item title/summary from the source system."
              }
            }
          }
        }
      }
    }
  }
}
`

// ===========================================================================
// T3 — security-critical filesystem layer (MIME double-gate, containment,
// atomic --out, signal cleanup). Authoritative spec: §4.2 (EARS-S),
// §4.4 (EARS-N), §8.2 (security).
// ===========================================================================

// allowedAssetMIMEs is the closed extension→expected-MIME allowlist for
// EV-U-6. Keys are LOWERCASE; matching is done via strings.ToLower on
// the file extension. SVG is intentionally excluded (§8.2 — SVG is an
// executable XML format and would create an XSS surface). `.jpg` and
// `.jpeg` BOTH map to `image/jpeg` (the same MIME the http.DetectContentType
// sniffer returns for either JPEG variant).
var allowedAssetMIMEs = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mov":  "video/quicktime",
}

// allowedAssetMIMEList is the rendered ", "-joined sorted allowlist used
// in error messages so an agent reading a render failure sees the exact
// permitted set. Built once at init.
var allowedAssetMIMEList = func() string {
	seen := map[string]bool{}
	mimes := make([]string, 0, len(allowedAssetMIMEs))
	for _, m := range allowedAssetMIMEs {
		if seen[m] {
			continue
		}
		seen[m] = true
		mimes = append(mimes, m)
	}
	sort.Strings(mimes)
	return strings.Join(mimes, ", ")
}()

// maxAssetBytes is the per-asset size cap (EV-S-2). It is a `var` not a
// `const` so tests can shrink it to drive the 1-GiB-cap rejection path
// without writing a real 1-GiB fixture.
var maxAssetBytes int64 = 1 << 30 // 1 GiB

// asset-sniff window: http.DetectContentType uses up to 512 bytes per the
// MIME-sniff spec. Pulled out as a const for clarity at the read site.
const sniffWindow = 512

// copyAsset opens srcPath ONCE, sniffs the MIME on the first 512 bytes,
// validates the extension/sniff agreement against allowedAssetMIMEs, and
// streams the rest of the file to dstPath without reopening the source
// (EV-U-6 TOCTOU mitigation: one file handle, sniffed bytes flow through
// io.MultiReader so the sniffed window is never re-read from disk).
//
// Per-asset 1-GiB cap (EV-S-2) is enforced by reading from
// io.LimitReader(srcFile, maxAssetBytes - sniffWindow + 1). On overflow,
// the partial dst is left for the caller to clean up.
//
// dst file mode is 0644.
func copyAsset(srcPath, dstPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("templates: open evidence asset %q: %w", srcPath, err)
	}
	defer srcFile.Close()

	// Sniff first sniffWindow bytes from the open handle. We read into
	// a stack buffer; ReadFull tolerates a short file (treats EOF as
	// "we read what we could").
	head := make([]byte, sniffWindow)
	n, err := io.ReadFull(srcFile, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return fmt.Errorf("templates: read evidence asset %q: %w", srcPath, err)
	}
	head = head[:n]

	// Two-gate MIME check (EV-U-6).
	ext := strings.ToLower(filepath.Ext(srcPath))
	expected, ok := allowedAssetMIMEs[ext]
	if !ok {
		return fmt.Errorf(
			"templates: evidence asset %q has disallowed extension %q; allowed MIMEs: %s",
			srcPath, ext, allowedAssetMIMEList,
		)
	}
	sniffed := http.DetectContentType(head)
	// http.DetectContentType returns "video/mp4" for both .mp4 and the
	// related ISO-BMFF container types we accept; for .mov (QuickTime)
	// the sniffer returns "video/quicktime". We compare the leading
	// "type/subtype" segment so a "; charset=..." suffix (which the
	// sniffer never adds for binary types but might in future Go
	// versions) doesn't break the equality check.
	sniffedHead := strings.SplitN(sniffed, ";", 2)[0]
	if sniffedHead != expected {
		return fmt.Errorf(
			"templates: evidence asset %q MIME mismatch: extension=%s expected=%s, content sniff=%s; allowed MIMEs: %s",
			srcPath, ext, expected, sniffedHead, allowedAssetMIMEList,
		)
	}

	// Open dst, then stream sniffed-bytes-then-rest-of-file via
	// MultiReader. The "rest of file" is bounded by LimitReader to
	// catch the EV-S-2 1 GiB cap. We add 1 to the limit so we can
	// distinguish "exactly at cap" from "over cap" by looking at
	// the trailing read.
	dstFile, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("templates: create evidence dst %q: %w", dstPath, err)
	}
	defer dstFile.Close()

	remaining := maxAssetBytes - int64(len(head))
	if remaining < 0 {
		// pathological: cap is smaller than sniff window. Treat as cap-exceeded.
		return fmt.Errorf("templates: evidence asset %q exceeds per-asset cap of %d bytes", srcPath, maxAssetBytes)
	}
	limited := io.LimitReader(srcFile, remaining+1)
	mr := io.MultiReader(bytes.NewReader(head), limited)

	written, err := io.Copy(dstFile, mr)
	if err != nil {
		return fmt.Errorf("templates: copy evidence asset %q: %w", srcPath, err)
	}
	if written > maxAssetBytes {
		return fmt.Errorf(
			"templates: evidence asset %q exceeds per-asset cap of %d bytes (read %d)",
			srcPath, maxAssetBytes, written,
		)
	}
	return nil
}

// containAsset implements the EV-S-1 path-containment algorithm:
//
//  1. base   = filepath.EvalSymlinks(filepath.Clean(base))
//  2. target = filepath.EvalSymlinks(filepath.Join(base, candidate))
//     (if target doesn't exist, EvalSymlinks the parent and rejoin the
//     basename — a not-yet-created file can't itself contain a symlink,
//     but its parent might)
//  3. rel    = filepath.Rel(base, target); MUST NOT start with `..` and
//     MUST NOT be absolute.
//
// Returns the resolved absolute target path on success. NEVER returns a
// path outside base.
func containAsset(base, candidate string) (string, error) {
	cleanBase, err := filepath.EvalSymlinks(filepath.Clean(base))
	if err != nil {
		return "", fmt.Errorf("templates: evidence containment base %q: %w", base, err)
	}
	cleanBase, err = filepath.Abs(cleanBase)
	if err != nil {
		return "", fmt.Errorf("templates: evidence containment base abs %q: %w", base, err)
	}

	joined := filepath.Join(cleanBase, candidate)

	// Lexical pre-check: if the joined+cleaned path is already outside
	// cleanBase BEFORE any filesystem resolution, reject immediately.
	// This catches `../../...` cases where the parent directory doesn't
	// even exist (so EvalSymlinks would error with "no such file" rather
	// than "escapes root", which is a less useful error).
	if rel, relErr := filepath.Rel(cleanBase, joined); relErr == nil {
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return "", fmt.Errorf(
				"templates: evidence asset %q escapes containment root (lexical resolution %q outside %q)",
				candidate, joined, cleanBase,
			)
		}
	}

	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		// Asset doesn't exist yet — resolve the deepest existing
		// ancestor and rejoin the unresolved tail so we still catch a
		// parent-directory symlink that escapes base. (The asset-
		// existence check fires later when we open it.)
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("templates: evidence resolve %q: %w", candidate, err)
		}
		ancestor := joined
		var tail string
		for {
			parent := filepath.Dir(ancestor)
			if parent == ancestor {
				// reached filesystem root without finding an extant
				// ancestor — fall back to the lexical join.
				resolved = joined
				break
			}
			if r, perr := filepath.EvalSymlinks(parent); perr == nil {
				if tail == "" {
					tail = filepath.Base(ancestor)
				} else {
					tail = filepath.Join(filepath.Base(ancestor), tail)
				}
				resolved = filepath.Join(r, tail)
				break
			} else if !os.IsNotExist(perr) {
				return "", fmt.Errorf("templates: evidence resolve parent of %q: %w", candidate, perr)
			}
			if tail == "" {
				tail = filepath.Base(ancestor)
			} else {
				tail = filepath.Join(filepath.Base(ancestor), tail)
			}
			ancestor = parent
		}
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("templates: evidence resolve abs %q: %w", candidate, err)
	}

	rel, err := filepath.Rel(cleanBase, resolved)
	if err != nil {
		return "", fmt.Errorf("templates: evidence rel %q: %w", candidate, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf(
			"templates: evidence asset %q escapes containment root (resolved to %q, outside %q)",
			candidate, resolved, cleanBase,
		)
	}
	return resolved, nil
}

// SafeAssetName builds the destination basename for an asset inside the
// rendered bundle. The format is `<NNN>-<sanitized-basename>` where:
//
//   - NNN is the 1-based item index padded to 3 digits, so the order in
//     the bundle directory matches the post-sort item array (helpful when
//     listing the assets directory) and adjacent indices can never
//     collide.
//   - sanitized-basename lowercases the filename, replaces any character
//     outside [a-z0-9.-] with '-', and collapses runs of '-'.
//
// The deterministic prefix means two items whose post-sanitization
// basenames match (e.g. ./a/foo.png and ./b/foo.png) still produce
// distinct dst names (`001-foo.png` vs `002-foo.png`). The collision
// check in RenderEvidence is a defensive guard for the case where a
// future change to this function weakens the prefix.
//
// Exported because evidence_render.go's HTML template needs to compute
// the same name for `<img src="assets/...">` hrefs. There is exactly
// one safe-name algorithm in this package; see the call site in
// `toEvidenceModel` (evidence_render.go) for the HTML side.
func SafeAssetName(item EvidenceItem, idx int) string {
	base := filepath.Base(item.Src)
	low := strings.ToLower(base)
	var b strings.Builder
	b.Grow(len(low))
	prevDash := false
	for _, r := range low {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			prevDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '.' || r == '-':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	cleaned := b.String()
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		// pathological basename; fall back to a stable placeholder so
		// the index prefix still uniquifies.
		cleaned = "asset"
	}
	return fmt.Sprintf("%03d-%s", idx+1, cleaned)
}

// RenderOptions wraps the mode-specific knobs T5 (cmd_evidence.go) needs
// to pass into RenderEvidence. Keeping this as a struct (rather than a
// long positional arg list) keeps the call site readable as new fields
// land in v1.x.
type RenderOptions struct {
	// Deprecated: ignored. Evidence pages include a viewer-side layout
	// switcher, so layout is no longer chosen at render/publish time.
	Layout string
	// OutDir is the user-supplied --out target. Empty when --push only:
	// in that case RenderEvidence creates a CLI-owned temp dir, returns
	// its path, and leaves cleanup to the caller.
	OutDir string
	// ContainmentRoot is the base directory all item.src paths must
	// resolve inside (EV-S-1). Per spec §4.2 EV-E-7: the directory
	// containing the --from file, or CWD when --from -.
	ContainmentRoot string
}

// renderTempDirPrefix is used by both --push (CLI-owned temp dir) and
// --out (sibling tmp). Kept as a named const so the signal handler can
// pattern-match if needed for diagnostics.
const renderTempDirPrefix = ".evidence-"

// renderTempName builds the sibling-tmp / CLI-owned-tmp basename. 8 hex
// chars of entropy is plenty to make accidental collision in the same
// parent directory effectively impossible (2^32 namespace, parent dir
// would never see millions of concurrent renders).
func renderTempName() (string, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return renderTempDirPrefix + hex.EncodeToString(buf[:]), nil
}

// RenderEvidence parses+validates the JSON input, sorts items per
// EVSC-10, copies referenced assets into the bundle (with MIME
// double-gate, containment, and 1-GiB cap), and calls T4's
// renderEvidenceHTML to emit index.html + styles.css + evidence.js.
//
// Returns the parsed EvidenceInput (so the caller can log
// title/itemcount) and the absolute path of the final bundle directory:
//
//   - When opts.OutDir is set: the bundle is written into a sibling tmp
//     dir (`<parent-of-outdir>/.evidence-<8hex>`), then atomically
//     renamed to opts.OutDir on success. On any failure, the sibling
//     tmp is removed; opts.OutDir is NEVER touched. Returns
//     opts.OutDir.
//   - When opts.OutDir is empty (--push only): a CLI-owned tmp dir is
//     created and returned to the caller. The caller (T5) installs its
//     own signal cleanup and removes the dir when the push completes
//     (success or failure).
//
// Concurrency (EV-S-4): two parallel runs against the same opts.OutDir
// produce one success + one clean failure. The guard is a stat-then-
// rename: we re-stat opts.OutDir immediately before the rename and
// refuse if it now exists. The residual race window (sibling-tmp built,
// but a concurrent run renamed first) is bounded by the per-tenant
// /v1/sites rate limit (E8 §8.2), consistent with the read-then-INSERT
// fairness-counter inheritance documented in spec §8.2.
func RenderEvidence(input []byte, opts RenderOptions, g Generator) (EvidenceInput, string, error) {
	in, err := ParseEvidence(input)
	if err != nil {
		return EvidenceInput{}, "", err
	}
	in.Items = SortItems(in.Items)

	if opts.ContainmentRoot == "" {
		return in, "", errors.New("templates: evidence RenderOptions.ContainmentRoot is required")
	}

	// Pre-flight: build dst names for every (sorted) item and detect
	// collisions BEFORE any I/O (EV-N-2). With the current safe-name
	// function this guard is defensive — the index prefix uniquifies
	// adjacent indices — but a future change to SafeAssetName could
	// re-introduce collisions and we want a fail-fast assertion.
	dstNames := make([]string, len(in.Items))
	seen := make(map[string]int, len(in.Items))
	for i, it := range in.Items {
		name := SafeAssetName(it, i)
		if prev, dup := seen[name]; dup {
			return in, "", fmt.Errorf(
				"templates: evidence asset name collision: items[%d] (src=%q) and items[%d] (src=%q) both resolve to %q",
				prev, in.Items[prev].Src, i, it.Src, name,
			)
		}
		seen[name] = i
		dstNames[i] = name
	}

	// Resolve every item.src under the containment root BEFORE creating
	// any output dirs — if containment fails, no temp dir was made and
	// nothing needs cleanup.
	resolvedSrcs := make([]string, len(in.Items))
	for i, it := range in.Items {
		resolved, cerr := containAsset(opts.ContainmentRoot, it.Src)
		if cerr != nil {
			return in, "", cerr
		}
		resolvedSrcs[i] = resolved
	}

	// Branch on --out vs --push-only.
	if opts.OutDir != "" {
		return renderToOutDir(in, opts, dstNames, resolvedSrcs, g)
	}
	return renderToTempDir(in, dstNames, resolvedSrcs, g)
}

// errEvidenceAborted signals that the asset-copy loop bailed because
// the abort channel was closed by the signal handler. It is wrapped in
// a templates-namespaced error string so the failure is recognizable in
// logs, but it is intentionally NOT exported: callers above
// renderToOutDir treat this the same as any other render failure (they
// see a non-nil error and surface it). The signal handler itself does
// not inspect this error — it only needs to know that the writer
// goroutine has returned, regardless of whether it returned with this
// abort error or completed naturally.
var errEvidenceAborted = errors.New("templates: evidence render aborted by signal")

// renderToTempDir handles the --push-only path: create a CLI-owned temp
// dir and return its absolute path. The caller (T5) owns cleanup and
// signal handling; we install nothing here.
func renderToTempDir(in EvidenceInput, dstNames, resolvedSrcs []string, g Generator) (EvidenceInput, string, error) {
	tmpName, err := renderTempName()
	if err != nil {
		return in, "", fmt.Errorf("templates: evidence tmp name: %w", err)
	}
	// os.TempDir() is the canonical CLI-owned location; the caller
	// removes the dir whether the push succeeds or fails (EV-E-5).
	tmpDir := filepath.Join(os.TempDir(), tmpName)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return in, "", fmt.Errorf("templates: evidence tmpdir: %w", err)
	}
	// --push mode: no signal handler at this layer (the caller, T5, owns
	// process-level signals and tmp cleanup per EV-E-5). Pass a nil
	// abort channel; writeBundleContents treats nil as "never aborts".
	if err := writeBundleContents(in, tmpDir, dstNames, resolvedSrcs, g, nil); err != nil {
		// Best-effort cleanup on failure; caller would clean up too,
		// but we own this dir until we return.
		_ = os.RemoveAll(tmpDir)
		return in, "", err
	}
	return in, tmpDir, nil
}

// renderToOutDir handles the --out path with EV-S-1/3/4 guarantees:
// sibling-tmp build, signal-handler cleanup (sibling-tmp only — never
// touches opts.OutDir), atomic rename, RENAME_NOREPLACE-ish concurrent
// guard.
func renderToOutDir(in EvidenceInput, opts RenderOptions, dstNames, resolvedSrcs []string, g Generator) (EvidenceInput, string, error) {
	absOut, err := filepath.Abs(opts.OutDir)
	if err != nil {
		return in, "", fmt.Errorf("templates: evidence --out abs %q: %w", opts.OutDir, err)
	}
	parent := filepath.Dir(absOut)

	// EV-S-4 pre-guard: refuse if --out already exists at start. The
	// post-build re-stat below catches the concurrent-render race.
	if _, statErr := os.Lstat(absOut); statErr == nil {
		return in, "", fmt.Errorf(
			"templates: evidence --out %q already exists; refusing to overwrite",
			absOut,
		)
	} else if !os.IsNotExist(statErr) {
		return in, "", fmt.Errorf("templates: evidence --out stat %q: %w", absOut, statErr)
	}

	// EV-N-7 note: the sibling-tmp is created INSIDE `parent` (same
	// directory as opts.OutDir), so the final atomic rename is
	// guaranteed to be intra-device — no cross-device check is
	// needed in this path. The package-private `sameDevice` helper
	// exists for the EV-N-7 unit test (TestSameDevice_*) and any
	// future render-internal check that needs to compare an
	// externally-staged location against --out before rename. It is
	// intentionally not exported: every external caller (the CLI in
	// T5) goes through RenderEvidence, which already enforces the
	// EV-N-7 invariant by construction.
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return in, "", fmt.Errorf("templates: evidence --out parent %q: %w", parent, err)
	}

	tmpName, err := renderTempName()
	if err != nil {
		return in, "", fmt.Errorf("templates: evidence sibling tmp name: %w", err)
	}
	siblingTmp := filepath.Join(parent, tmpName)
	if err := os.MkdirAll(siblingTmp, 0o755); err != nil {
		return in, "", fmt.Errorf("templates: evidence sibling tmpdir: %w", err)
	}

	// Install signal cleanup: only the sibling-tmp is removed; --out is
	// NEVER touched (EV-S-3). signal.Stop on the success path.
	//
	// Cancellation propagation: the previous implementation
	// called `os.RemoveAll(siblingTmp)` IMMEDIATELY on signal delivery,
	// concurrently with an in-flight `io.Copy` writing into siblingTmp.
	// That was a real race: cleanup could unlink files that copyAsset
	// was still streaming into. The fix is two-step:
	//
	//   1. Signal handler closes `abort`, which writeBundleContents
	//      checks BETWEEN asset copies. The current asset's io.Copy is
	//      bounded by maxAssetBytes (1 GiB cap, EV-S-2) so it finishes
	//      in seconds even on the worst case. NEW asset copies do not
	//      start after abort.
	//   2. Signal handler waits on `writerDone` (closed by the main
	//      goroutine after writeBundleContents returns), THEN runs
	//      cleanup. Cleanup never races a write because the writer has
	//      provably returned.
	//
	// Residual: if SIGINT lands during an asset's io.Copy, that asset
	// finishes (bounded by the 1 GiB cap = a few seconds on a fast disk)
	// before cleanup runs. New asset copies do not start. Cleanup never
	// races a write because cleanup waits for the asset-copy goroutine
	// to return.
	var cleanupOnce sync.Once
	cleanup := func() { _ = os.RemoveAll(siblingTmp) }
	doCleanup := func() { cleanupOnce.Do(cleanup) }

	abort := make(chan struct{})
	writerDone := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case <-sigCh:
			// Step 1: tell the writer to stop starting new asset copies.
			close(abort)
			// Step 2: wait for the writer to return (cleanly or with
			// the abort error). This is the critical synchronization
			// that fixes the cleanup-vs-io.Copy race.
			<-writerDone
			doCleanup()
			// Do NOT exit the process — the caller's signal handler
			// (or default Go behavior on a re-raised signal) decides
			// the exit code. We just guarantee the sibling-tmp is gone.
		case <-done:
		}
	}()
	defer func() {
		signal.Stop(sigCh)
		close(done)
	}()

	werr := writeBundleContents(in, siblingTmp, dstNames, resolvedSrcs, g, abort)
	close(writerDone)
	if werr != nil {
		// The signal handler may have already initiated cleanup. doCleanup
		// is once-guarded, so the second call is a no-op.
		doCleanup()
		return in, "", werr
	}

	// EV-S-4 re-stat-then-rename concurrent guard. Residual race window
	// is bounded by the per-tenant rate limit (spec §8.2).
	if _, statErr := os.Lstat(absOut); statErr == nil {
		doCleanup()
		return in, "", fmt.Errorf(
			"templates: evidence --out %q was created by a concurrent render; refusing to overwrite",
			absOut,
		)
	} else if !os.IsNotExist(statErr) {
		doCleanup()
		return in, "", fmt.Errorf("templates: evidence --out re-stat %q: %w", absOut, statErr)
	}

	if err := os.Rename(siblingTmp, absOut); err != nil {
		doCleanup()
		return in, "", fmt.Errorf("templates: evidence atomic rename %q -> %q: %w", siblingTmp, absOut, err)
	}
	// sibling-tmp is now gone (renamed); no cleanup needed.
	return in, absOut, nil
}

// writeBundleContents lays out the bundle inside outDir:
//
//	outDir/
//	  index.html
//	  styles.css        (T4 owns)
//	  evidence.js       (T4 owns)
//	  assets/<safe-name>...
//
// Asset copies happen first (so a missing asset / MIME failure aborts
// before HTML is written, keeping the partial dir empty of HTML).
//
// `abort`, when non-nil, is the signal-handler abort channel
// (EV-S-3). It is checked BEFORE each asset copy so a
// SIGINT/SIGTERM mid-bundle stops new asset copies cleanly. The
// in-flight asset (if any) finishes its io.Copy bounded by the 1 GiB
// per-asset cap (EV-S-2) — see renderToOutDir's signal-handler comment
// for the residual race analysis. A nil abort channel means "never
// aborts" (the --push mode in renderToTempDir uses this).
func writeBundleContents(in EvidenceInput, outDir string, dstNames, resolvedSrcs []string, g Generator, abort <-chan struct{}) error {
	assetsDir := filepath.Join(outDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return fmt.Errorf("templates: evidence assets dir: %w", err)
	}
	for i := range in.Items {
		// Pre-iteration abort check. A nil channel never selects, so
		// the --push path falls through cost-free.
		if abort != nil {
			select {
			case <-abort:
				return errEvidenceAborted
			default:
			}
		}
		dst := filepath.Join(assetsDir, dstNames[i])
		if err := copyAsset(resolvedSrcs[i], dst); err != nil {
			return err
		}
	}
	// One more abort check before HTML render: the HTML write is fast
	// (a few KB) so this is mostly defensive, but it keeps the loop
	// invariant tidy — "if abort fires, we never write more than the
	// already-finished asset".
	if abort != nil {
		select {
		case <-abort:
			return errEvidenceAborted
		default:
		}
	}
	// T4 owns the actual template execution. We pass items already
	// sorted (per EVSC-10) and dstNames so the template can build the
	// `assets/<name>` href without re-deriving the safe-name.
	return renderEvidenceHTML(in, outDir, g)
}
