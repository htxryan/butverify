// Package templates renders structured agent data into ready-to-publish
// "evidence sites" — closes the loop on butverify.dev's "agent shows their
// work" value prop. Two templates are exposed at v1: report (from JSON) and
// dashboard (from CSV). Both produce a directory of static HTML/CSS that the
// CLI's standard push flow uploads as a normal site.
//
// Design principles, in order:
//
//  1. **Stdlib only** — no third-party deps. The HTML is rendered via Go's
//     `html/template` (autoescaping built-in), CSV parsing via `encoding/csv`,
//     charts as inline SVG generated from a few hand-rolled primitives. The
//     resulting bundle has no JS runtime; everything renders on first paint.
//  2. **Schemas validate at parse time** — bad input fails before any HTML
//     is written. The data contracts are documented in
//     docs/specs/templates.md and tested as the source of truth.
//  3. **Deterministic output** — a given input file always produces a
//     byte-identical bundle. The deterministic-mtime + sorted-walk discipline
//     in go/pkg/tarbundle relies on this so a templated `bv push` produces
//     the same site_id each time (deterministic_site_id is derived from
//     (tenant_id, upload_id), but the file bytes are also stable).
//  4. **Bundle size budget** — typical input produces <2MB output (fitness
//     function from the epic). Charts are SVG (KB-scale, not MB) and CSS is
//     a single file embedded once.
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

// Generator is the version string injected into the rendered HTML's
// <meta name="generator"> tag and the manifest. Wire this from the CLI's
// build-time Version variable so audit logs can correlate a rendered site
// to a specific CLI release.
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
