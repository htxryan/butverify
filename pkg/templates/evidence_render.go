// Evidence template rendering: produces index.html + styles.css from a
// validated EvidenceInput, in either the default stacked layout or the
// CSS-only carousel variant.
//
// This file is the wave-1 minimal-shell renderer (E12-T4). Asset copy,
// MIME sniffing, path containment, atomic --out, and tmp-dir cleanup
// live in T3's `evidence.go` (sibling). The split is:
//
//   - T3 owns asset placement: assets land at outDir/assets/<safe-name>
//     before this code runs. T3's RenderEvidence calls
//     renderEvidenceHTML(in, layout, outDir, g) at the end. The
//     canonical safe-name algorithm — used both for on-disk filenames
//     AND for `<img src="assets/...">` hrefs in the HTML — lives in
//     evidence.go as the exported `SafeAssetName`. This file calls it
//     from `toEvidenceModel` so HTML and disk can never drift.
//   - This file owns HTML/CSS template execution.
//
// The /compound:build-great-things design pass is T7, NOT this task; the
// templates here are intentionally minimal-but-correct so EARS coverage
// (EV-U-2/4/9/10) lands without coupling to a design that's still
// in flight.

package templates

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"path/filepath"
	"strings"
)

//go:embed assets/evidence.html.tmpl assets/evidence-carousel.html.tmpl assets/evidence.css
var evidenceFS embed.FS

// evidenceFuncs are the template helpers we need for the carousel variant
// (1-based pager labels). Kept tiny on purpose — every helper is a chance
// for a typo to break the layout.
var evidenceFuncs = template.FuncMap{
	"add1": func(i int) int { return i + 1 },
}

// evidenceStackedTmpl and evidenceCarouselTmpl are parsed once at init.
// Parse failure is a programmer error (templates ship in the binary), so
// we panic — there's no runtime fallback.
var (
	evidenceStackedTmpl  = mustParseEvidence("assets/evidence.html.tmpl")
	evidenceCarouselTmpl = mustParseEvidence("assets/evidence-carousel.html.tmpl")
)

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
	GeneratorVersion string
	Items            []evidenceRenderItem
}

type evidenceRenderItem struct {
	SafeName    string
	Alt         string
	Title       string
	Description string
	IsImage     bool
	IsVideo     bool
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
		items = append(items, evidenceRenderItem{
			SafeName:    SafeAssetName(it, i),
			Alt:         alt,
			Title:       it.Title,
			Description: it.Description,
			IsImage:     imageExts[ext],
			IsVideo:     videoExts[ext],
		})
	}
	return evidenceRenderModel{
		Title:            in.Title,
		Subtitle:         in.Subtitle,
		Summary:          in.Summary,
		GeneratorVersion: g.version(),
		Items:            items,
	}
}

// renderEvidenceHTML writes index.html + styles.css into outDir.
//
// Caller (T3) is responsible for:
//   - validating `in` (via ParseEvidence)
//   - copying assets into outDir/assets/<safe-name> with the same
//     canonical `SafeAssetName` algorithm (evidence.go) — the same
//     function this file calls when building href attributes
//   - creating outDir
//
// Determinism: this function MUST NOT embed any wall-clock timestamp in
// the output (EV-U-7). Generator.Now is intentionally unused here; only
// Generator.Version is rendered (into the <meta name="generator"> tag).
//
// layout MUST be either "stacked" or "carousel". Any other value
// returns an error — defense in depth; the CLI (T5) is the primary
// gate per EV-E-7.
func renderEvidenceHTML(in EvidenceInput, layout string, outDir string, g Generator) error {
	var tmpl *template.Template
	switch layout {
	case "stacked":
		tmpl = evidenceStackedTmpl
	case "carousel":
		tmpl = evidenceCarouselTmpl
	case "":
		return errors.New("templates: renderEvidenceHTML: layout is required (\"stacked\" or \"carousel\")")
	default:
		return fmt.Errorf("templates: renderEvidenceHTML: unknown layout %q (must be \"stacked\" or \"carousel\")", layout)
	}

	sorted := SortItems(in.Items)
	model := toEvidenceModel(EvidenceInput{
		Title:    in.Title,
		Subtitle: in.Subtitle,
		Summary:  in.Summary,
		Items:    sorted,
	}, g)

	var html bytes.Buffer
	if err := tmpl.Execute(&html, model); err != nil {
		return fmt.Errorf("templates: render evidence (%s): %w", layout, err)
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
	return nil
}
