// Report template: renders a structured "agent ran a job, here's what
// happened" site from a JSON contract. The schema is documented in
// docs/specs/templates.md; this file is the executable definition.
//
// Section types at v1:
//   * headline    — a tone-coloured banner ("succeeded", "warning", "failed")
//   * kv          — a two-column key/value summary
//   * table       — a tabular view (string cells; rendering does NOT
//                   re-format numbers)
//   * code        — a code snippet (language label optional; no syntax
//                   highlighting at v1 — adds JS runtime cost we don't
//                   want yet)
//   * diff        — a unified diff with +/- line styling. We accept either a
//                   pre-formatted unified-diff string OR a {before, after}
//                   pair (we run a minimal LCS-free diff for the latter — see
//                   diff.go). For v1 the {before, after} path is intentionally
//                   simple: we line-up the two strings and tag every line
//                   that differs, which is good enough for "render this code
//                   change" without pulling in a full diff library.
//   * text        — a plain-text block (preserves whitespace via white-space:
//                   pre-wrap)

package templates

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ReportInput is the top-level JSON contract for `bv report`. Keep this in
// sync with docs/specs/templates.md.
type ReportInput struct {
	Title    string          `json:"title"`
	Subtitle string          `json:"subtitle,omitempty"`
	Sections []ReportSection `json:"sections"`
}

// ReportSection is one rendered block. The Type field selects the renderer;
// the other fields are populated according to Type.
//
// We keep all fields on one struct (rather than a discriminated-union via
// json.RawMessage) because the shape is small and a flat struct keeps the
// JSON contract self-documenting for agent authors. The trade is that
// downstream code reads only the fields that match Type and ignores the rest.
type ReportSection struct {
	Type     string     `json:"type"`
	Title    string     `json:"title,omitempty"`
	Tone     string     `json:"tone,omitempty"`     // headline only
	Text     string     `json:"text,omitempty"`     // headline only
	Items    []ReportKV `json:"items,omitempty"`    // kv only
	Columns  []string   `json:"columns,omitempty"`  // table only
	Rows     [][]string `json:"rows,omitempty"`     // table only
	Language string     `json:"language,omitempty"` // code only
	Code     string     `json:"code,omitempty"`     // code only
	Before   string     `json:"before,omitempty"`   // diff only
	After    string     `json:"after,omitempty"`    // diff only
	Unified  string     `json:"unified,omitempty"`  // diff only (pre-formatted)
	Body     string     `json:"body,omitempty"`     // text only
}

// ReportKV is one row in a kv block.
type ReportKV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Render-time caps. The tar-bundle Content-Length cap (E3) gates oversized
// uploads but applies AFTER rendering — a malicious / runaway agent submitting
// a JSON with millions of sections or rows would OOM the renderer before the
// bundle cap could stop it. These caps bound peak memory at parse time so the
// renderer is bounded irrespective of upload size.
const (
	maxReportSections  = 500
	maxReportItems     = 10_000
	maxReportTableRows = 10_000
)

// validReportSectionTypes is the closed set of section types accepted at v1.
// Adding a new type is a contract change — bump the docs and add the
// renderer's branch in the template.
var validReportSectionTypes = map[string]bool{
	"headline": true,
	"kv":       true,
	"table":    true,
	"code":     true,
	"diff":     true,
	"text":     true,
}

// validReportTones is the closed set of headline tones; mismatches default to
// "info" in the template's `{{or .Tone "info"}}` clause but we still validate
// here so an agent's typo is reported up-front.
var validReportTones = map[string]bool{
	"info":    true,
	"success": true,
	"warn":    true,
	"error":   true,
}

// ParseReport reads the JSON contract from r, validates it, and returns the
// parsed input ready for rendering. Validation is strict: unknown section
// types, missing required fields, and shape mismatches return an error
// describing the offending section index.
func ParseReport(r []byte) (ReportInput, error) {
	var in ReportInput
	dec := json.NewDecoder(bytes.NewReader(r))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return ReportInput{}, fmt.Errorf("templates: parse report: %w", err)
	}
	if err := in.Validate(); err != nil {
		return ReportInput{}, err
	}
	return in, nil
}

// Validate enforces the report contract. Errors carry the offending section
// index so an agent can fix a malformed input without binary-searching.
func (in *ReportInput) Validate() error {
	if strings.TrimSpace(in.Title) == "" {
		return errors.New("templates: report.title is required")
	}
	if len(in.Sections) == 0 {
		return errors.New("templates: report.sections must contain at least one section")
	}
	if len(in.Sections) > maxReportSections {
		return fmt.Errorf("templates: report.sections exceeds limit (%d > %d)", len(in.Sections), maxReportSections)
	}
	for i, s := range in.Sections {
		if !validReportSectionTypes[s.Type] {
			return fmt.Errorf("templates: report.sections[%d].type=%q is not a known type", i, s.Type)
		}
		switch s.Type {
		case "headline":
			if strings.TrimSpace(s.Text) == "" {
				return fmt.Errorf("templates: report.sections[%d] (headline): text is required", i)
			}
			if s.Tone != "" && !validReportTones[s.Tone] {
				return fmt.Errorf("templates: report.sections[%d] (headline): tone=%q is not one of info/success/warn/error", i, s.Tone)
			}
		case "kv":
			if len(s.Items) == 0 {
				return fmt.Errorf("templates: report.sections[%d] (kv): items is required", i)
			}
			if len(s.Items) > maxReportItems {
				return fmt.Errorf("templates: report.sections[%d] (kv): items exceeds limit (%d > %d)", i, len(s.Items), maxReportItems)
			}
		case "table":
			if len(s.Columns) == 0 {
				return fmt.Errorf("templates: report.sections[%d] (table): columns is required", i)
			}
			if len(s.Rows) > maxReportTableRows {
				return fmt.Errorf("templates: report.sections[%d] (table): rows exceeds limit (%d > %d)", i, len(s.Rows), maxReportTableRows)
			}
			for j, row := range s.Rows {
				if len(row) != len(s.Columns) {
					return fmt.Errorf(
						"templates: report.sections[%d] (table): row %d has %d cells; expected %d",
						i, j, len(row), len(s.Columns),
					)
				}
			}
		case "code":
			if strings.TrimSpace(s.Code) == "" {
				return fmt.Errorf("templates: report.sections[%d] (code): code is required", i)
			}
		case "diff":
			if strings.TrimSpace(s.Unified) == "" && s.Before == "" && s.After == "" {
				return fmt.Errorf("templates: report.sections[%d] (diff): either unified or before/after is required", i)
			}
		case "text":
			if strings.TrimSpace(s.Body) == "" {
				return fmt.Errorf("templates: report.sections[%d] (text): body is required", i)
			}
		}
	}
	return nil
}

// reportRenderModel is the shape passed to the template. We pre-compute the
// per-section diff lines here so the template stays a flat range over the
// sections list (avoids a custom template func registration just for diff
// rendering).
type reportRenderModel struct {
	Title            string
	Subtitle         string
	Sections         []reportRenderSection
	GeneratorVersion string
	GeneratedAt      string
}

type reportRenderSection struct {
	Type      string
	Title     string
	Tone      string
	Text      string
	Items     []ReportKV
	Columns   []string
	Rows      [][]string
	Language  string
	Code      string
	DiffLines []diffLine
	Body      string
}

type diffLine struct {
	Cls  string // "add", "rem", or empty (context)
	Text string
}

func toReportModel(in ReportInput, g Generator) reportRenderModel {
	out := reportRenderModel{
		Title:            in.Title,
		Subtitle:         in.Subtitle,
		Sections:         make([]reportRenderSection, 0, len(in.Sections)),
		GeneratorVersion: g.version(),
		GeneratedAt:      g.generatedAt(),
	}
	for _, s := range in.Sections {
		rs := reportRenderSection{
			Type:     s.Type,
			Title:    s.Title,
			Tone:     s.Tone,
			Text:     s.Text,
			Items:    s.Items,
			Columns:  s.Columns,
			Rows:     s.Rows,
			Language: s.Language,
			Code:     s.Code,
			Body:     s.Body,
		}
		if s.Type == "diff" {
			rs.DiffLines = renderDiffLines(s)
		}
		out.Sections = append(out.Sections, rs)
	}
	return out
}

// RenderReport reads the JSON input bytes and writes the rendered site files
// (index.html + assets/styles.css) into outDir. The directory is created if
// it doesn't exist; existing files are overwritten.
//
// The output bundle has the shape:
//
//	outDir/
//	  index.html
//	  assets/
//	    styles.css
//
// which is exactly what the standard `bv push` path expects. Note: we do
// NOT write a manifest.json — the server computes that on finalize.
func RenderReport(input []byte, outDir string, g Generator) (ReportInput, error) {
	in, err := ParseReport(input)
	if err != nil {
		return ReportInput{}, err
	}
	model := toReportModel(in, g)
	var html bytes.Buffer
	if err := reportTmpl.Execute(&html, model); err != nil {
		return ReportInput{}, fmt.Errorf("templates: render report: %w", err)
	}
	if err := writeFile(outDir, "index.html", html.Bytes()); err != nil {
		return ReportInput{}, err
	}
	if err := writeAsset(outDir, "assets/styles.css"); err != nil {
		return ReportInput{}, err
	}
	return in, nil
}
