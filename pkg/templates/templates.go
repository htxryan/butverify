// Package templates renders structured agent data into ready-to-publish
// "evidence sites" — closes the loop on butverify.dev's "agent shows their
// work" value prop. The CLI exposes report (from JSON), dashboard (from CSV),
// and evidence (from JSON manifests plus local media assets). Each produces a
// directory of static files that the CLI's standard push flow uploads as a
// normal site.
//
// Design principles, in order:
//
//  1. **Stdlib only** — no third-party deps. The HTML is rendered via Go's
//     `html/template` (autoescaping built-in), CSV parsing via `encoding/csv`,
//     charts as inline SVG generated from a few hand-rolled primitives. Report
//     and dashboard render without JavaScript; evidence bundles a small local
//     script for viewer controls.
//  2. **Schemas validate at parse time** — bad input fails before any HTML
//     is written. The data contracts are documented in
//     docs/specs/templates.md and tested as the source of truth.
//  3. **Deterministic when generation metadata is pinned** — a given input
//     file plus Generator produces a byte-identical bundle. Evidence renders
//     include publication metadata, so callers/tests that require byte identity
//     must pass Generator.Now.
//  4. **Bundle size budget** — each template has a shell-size fitness test
//     appropriate to its rendered surface. Media assets remain governed by
//     upload-tier caps and evidence's per-asset limit.
package templates

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Generator carries render-time metadata injected into generated HTML. Wire
// Version from the CLI's build-time Version variable so audit logs can
// correlate a rendered site to a specific CLI release.
type Generator struct {
	Version string
	// Now overrides the timestamp written into the rendered footer; tests
	// pin this to a fixed value to keep snapshots stable. Zero value uses
	// time.Now().UTC().
	Now time.Time
}

func (g Generator) version() string {
	if g.Version == "" {
		return "dev"
	}
	return g.Version
}

func (g Generator) generatedAt() string {
	t := g.Now
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return t.UTC().Format(time.RFC3339)
}

//go:embed assets/styles.css assets/report.html.tmpl assets/dashboard.html.tmpl
var assetsFS embed.FS

// reportTmpl and dashboardTmpl are parsed once at init. Parse failure here is
// a programmer error (the templates ship in the binary) so panicking is
// appropriate — there is no runtime fallback path.
var (
	reportTmpl    = mustParse("assets/report.html.tmpl")
	dashboardTmpl = mustParse("assets/dashboard.html.tmpl")
)

func mustParse(name string) *template.Template {
	b, err := assetsFS.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("templates: read embedded %s: %v", name, err))
	}
	t, err := template.New(filepath.Base(name)).Parse(string(b))
	if err != nil {
		panic(fmt.Sprintf("templates: parse %s: %v", name, err))
	}
	return t
}

// writeAsset copies an embedded asset into outDir. Used to drop styles.css
// alongside the rendered HTML.
func writeAsset(outDir, name string) error {
	src, err := assetsFS.Open(name)
	if err != nil {
		return fmt.Errorf("templates: open asset %s: %w", name, err)
	}
	defer src.Close()
	dstPath := filepath.Join(outDir, "assets", filepath.Base(name))
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return nil
}

// writeFile creates outDir as needed and writes name with the given bytes.
func writeFile(outDir, name string, contents []byte) error {
	full := filepath.Join(outDir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, contents, 0o644)
}
