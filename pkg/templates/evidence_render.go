// Evidence template rendering: produces index.html + styles.css +
// evidence.js from a validated EvidenceInput. The rendered page includes a
// static-bundle controller for layout, metadata, outline, carousel
// navigation, and image lightbox controls.
//
// This file is the wave-1 minimal-shell renderer (E12-T4). Asset copy,
// MIME sniffing, path containment, atomic --out, and tmp-dir cleanup
// live in T3's `evidence.go` (sibling). The split is:
//
//   - T3 owns asset placement: assets land at outDir/assets/<safe-name>
//     before this code runs. T3's RenderEvidence calls
//     renderEvidenceHTML(in, outDir, g) at the end. The
//     canonical safe-name algorithm — used both for on-disk filenames
//     AND for `<img src="assets/...">` hrefs in the HTML — lives in
//     evidence.go as the exported `SafeAssetName`. This file calls it
//     from `toEvidenceModel` so HTML and disk can never drift.
//   - This file owns HTML/CSS/JS template execution.
//
// The /compound:build-great-things design pass is T7, NOT this task; the
// templates here are intentionally minimal-but-correct so EARS coverage
// (EV-U-2/4/9/10) lands without coupling to a design that's still
// in flight.

package templates

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed assets/evidence.html.tmpl assets/evidence.css assets/evidence.js
var evidenceFS embed.FS

// evidenceFuncs are the template helpers we need for 1-based item labels.
// Kept tiny on purpose — every helper is a chance for a typo to break the
// layout.
var evidenceFuncs = template.FuncMap{
	"add1": func(i int) int { return i + 1 },
}

// evidenceTmpl is parsed once at init. Parse failure is a programmer
// error (templates ship in the binary), so we panic — there's no runtime
// fallback.
var evidenceTmpl = mustParseEvidence("assets/evidence.html.tmpl")

func mustParseEvidence(name string) *template.Template {
	b, err := evidenceFS.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("templates: read embedded %s: %v", name, err))
	}
	t, err := template.New(filepath.Base(name)).Funcs(evidenceFuncs).Parse(string(b))
	if err != nil {
		panic(fmt.Sprintf("templates: parse %s: %v", name, err))
	}
	return t
}

// evidenceRenderModel is the shape passed to the template. The per-item
// SafeName/Alt/IsImage/IsVideo are pre-computed so the template stays a
// flat range over Items — keeps the .tmpl readable and keeps the
// branching logic in Go where it's testable.
type evidenceRenderModel struct {
	Title            string
	Subtitle         string
	Summary          string
	Metadata         evidenceRenderMetadata
	GeneratorVersion string
	PublishedAt      string
	Items            []evidenceRenderItem
}

type evidenceRenderMetadata struct {
	IssueURL   string
	IssueID    string
	IssueTitle string
	IssueLabel string
	HasIssue   bool
}

type evidenceRenderItem struct {
	SafeName    string
	Alt         string
	Title       string
	Description string
	Metadata    evidenceRenderMetadata
	Properties  []evidenceRenderProperty
	HasDetails  bool
	IsImage     bool
	IsVideo     bool
}

type evidenceRenderProperty struct {
	Label  string
	Value  string
	JSON   template.HTML
	IsJSON bool
}

// imageExts and videoExts mirror the closed allowlist documented in
// docs/specs/evidence-template.md §5 / EV-U-6. The MIME sniff (T3) is
// the runtime gate; we use the extension here only to pick the right
// HTML element (<img> vs <video>) — bad extensions are rejected
// upstream by T2/T3 before this code runs.
var imageExts = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".webp": true,
	".gif":  true,
}

var videoExts = map[string]bool{
	".mp4":  true,
	".webm": true,
	".mov":  true,
}

func toRenderMetadata(meta EvidenceMetadata) evidenceRenderMetadata {
	label := meta.IssueID
	if label == "" {
		label = meta.IssueTitle
	}
	if label == "" {
		label = meta.IssueURL
	}
	return evidenceRenderMetadata{
		IssueURL:   meta.IssueURL,
		IssueID:    meta.IssueID,
		IssueTitle: meta.IssueTitle,
		IssueLabel: label,
		HasIssue:   label != "",
	}
}

func toRenderProperties(props map[string]any) []evidenceRenderProperty {
	if len(props) == 0 {
		return nil
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]evidenceRenderProperty, 0, len(keys))
	for _, k := range keys {
		v := props[k]
		prop := evidenceRenderProperty{Label: k}
		switch val := v.(type) {
		case string:
			prop.Value = val
		case json.Number:
			prop.Value = val.String()
		case float64:
			prop.Value = fmt.Sprintf("%g", val)
		case map[string]any:
			prop.IsJSON = true
			prop.JSON = highlightJSON(val)
		default:
			prop.Value = fmt.Sprint(val)
		}
		out = append(out, prop)
	}
	return out
}

func highlightJSON(v any) template.HTML {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return template.HTML(html.EscapeString(fmt.Sprint(v)))
	}
	return template.HTML(highlightJSONText(string(b)))
}

func highlightJSONText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '"':
			j := i + 1
			for j < len(s) {
				if s[j] == '\\' {
					j += 2
					continue
				}
				if s[j] == '"' {
					j++
					break
				}
				j++
			}
			className := "ev-json-string"
			for k := j; k < len(s); k++ {
				if s[k] == ':' {
					className = "ev-json-key"
					break
				}
				if s[k] != ' ' && s[k] != '\t' && s[k] != '\n' && s[k] != '\r' {
					break
				}
			}
			writeJSONSpan(&b, className, s[i:j])
			i = j
		case c == '-' || (c >= '0' && c <= '9'):
			j := i + 1
			for j < len(s) && strings.ContainsRune("0123456789.eE+-", rune(s[j])) {
				j++
			}
			writeJSONSpan(&b, "ev-json-number", s[i:j])
			i = j
		case strings.HasPrefix(s[i:], "true"):
			writeJSONSpan(&b, "ev-json-literal", "true")
			i += 4
		case strings.HasPrefix(s[i:], "false"):
			writeJSONSpan(&b, "ev-json-literal", "false")
			i += 5
		case strings.HasPrefix(s[i:], "null"):
			writeJSONSpan(&b, "ev-json-literal", "null")
			i += 4
		default:
			b.WriteString(html.EscapeString(s[i : i+1]))
			i++
		}
	}
	return b.String()
}

func writeJSONSpan(b *strings.Builder, className, text string) {
	b.WriteString(`<span class="`)
	b.WriteString(className)
	b.WriteString(`">`)
	b.WriteString(html.EscapeString(text))
	b.WriteString(`</span>`)
}

// toEvidenceModel maps a validated EvidenceInput (already sorted via
// SortItems) to the template's render model. Alt-text default rule
// (per spec §5): item.Alt → fall back to item.Title (NEVER to
// description, which is body copy and would be a poor screen-reader
// experience).
func toEvidenceModel(in EvidenceInput, g Generator) evidenceRenderModel {
	items := make([]evidenceRenderItem, 0, len(in.Items))
	for i, it := range in.Items {
		ext := strings.ToLower(filepath.Ext(it.Src))
		alt := it.Alt
		if alt == "" {
			alt = it.Title
		}
		metadata := toRenderMetadata(it.Metadata)
		properties := toRenderProperties(it.Properties)
		items = append(items, evidenceRenderItem{
			SafeName:    SafeAssetName(it, i),
			Alt:         alt,
			Title:       it.Title,
			Description: it.Description,
			Metadata:    metadata,
			Properties:  properties,
			HasDetails:  metadata.HasIssue || len(properties) > 0,
			IsImage:     imageExts[ext],
			IsVideo:     videoExts[ext],
		})
	}
	return evidenceRenderModel{
		Title:            in.Title,
		Subtitle:         in.Subtitle,
		Summary:          in.Summary,
		Metadata:         toRenderMetadata(in.Metadata),
		GeneratorVersion: g.version(),
		PublishedAt:      g.generatedAt(),
		Items:            items,
	}
}

// renderEvidenceHTML writes index.html + styles.css + evidence.js into outDir.
//
// Caller (T3) is responsible for:
//   - validating `in` (via ParseEvidence)
//   - copying assets into outDir/assets/<safe-name> with the same
//     canonical `SafeAssetName` algorithm (evidence.go) — the same
//     function this file calls when building href attributes
//   - creating outDir
//
// Tests pin Generator.Now when byte-identical output matters; production
// renders use it for publication metadata shown in the evidence top bar.
func renderEvidenceHTML(in EvidenceInput, outDir string, g Generator) error {
	sorted := SortItems(in.Items)
	model := toEvidenceModel(EvidenceInput{
		Title:    in.Title,
		Subtitle: in.Subtitle,
		Summary:  in.Summary,
		Metadata: in.Metadata,
		Items:    sorted,
	}, g)

	var html bytes.Buffer
	if err := evidenceTmpl.Execute(&html, model); err != nil {
		return fmt.Errorf("templates: render evidence: %w", err)
	}
	if err := writeFile(outDir, "index.html", html.Bytes()); err != nil {
		return err
	}

	cssBytes, err := evidenceFS.ReadFile("assets/evidence.css")
	if err != nil {
		return fmt.Errorf("templates: read embedded evidence.css: %w", err)
	}
	if err := writeFile(outDir, "styles.css", cssBytes); err != nil {
		return err
	}
	jsBytes, err := evidenceFS.ReadFile("assets/evidence.js")
	if err != nil {
		return fmt.Errorf("templates: read embedded evidence.js: %w", err)
	}
	if err := writeFile(outDir, "evidence.js", jsBytes); err != nil {
		return err
	}
	return nil
}
